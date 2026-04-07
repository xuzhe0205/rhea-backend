package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"rhea-backend/internal/auth"
	ctxbuilder "rhea-backend/internal/context"
	"rhea-backend/internal/ingest"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/model"
	"rhea-backend/internal/router"
	"rhea-backend/internal/store"

	"github.com/google/uuid"
)

var ErrNoProvider = errors.New("no provider available")

// var tempUserID = uuid.MustParse("04260bc3-0f0b-4268-a079-21375a2340ea")

type Service struct {
	Store    store.Store
	Builder  *ctxbuilder.Builder
	Router   *router.Router
	Ingestor *ingest.ConversationIngestor
}

func (s *Service) Chat(ctx context.Context, conversationID string, userText string) (string, string, error) {
	if s.Store == nil || s.Builder == nil || s.Router == nil {
		return "", "", errors.New("agent service not configured")
	}

	// 0. 获取真实身份 🚀
	userID, err := s.getUserID(ctx)
	if err != nil {
		return "", "", err
	}

	// 1) Persist user message even if provider fails later
	if conversationID == "" {
		newID := uuid.New()
		_, err := s.Store.CreateConversation(ctx, &model.Conversation{
			ID: newID,
			// TODO: 后期接入 Auth 模块，从 context 获取真实的 UserID
			UserID: userID,

			// TODO: 后期接入 Title Generator，根据 userText 生成标题
			// 目前先简单截取前 20 个字符或硬编码
			Title: "New Chat: " + truncate(userText, 20),

			Summary: "", // 初始摘要为空
		})
		if err != nil {
			return "", "", err
		}
		conversationID = newID.String()
	}
	log.Printf("✅ Conversation ID created: %s", conversationID)

	// 2) 获取当前对话状态（拿到最新的 LastMsgID 作为父节点）
	conv, err := s.Store.GetConversation(ctx, conversationID)
	if err != nil {
		return "", "", err
	}
	// 🛡️ 增加所有权检查：确保这个对话真的属于当前登录用户
	if conv.UserID != userID {
		return "", "", errors.New("forbidden: access to conversation denied")
	}
	var parentID *string
	if conv.LastMsgID != nil {
		p := conv.LastMsgID.String()
		parentID = &p
	}

	// 3) 存储用户消息 (它的父节点是对话现有的 LastMsgID)
	newUserMsgID, err := s.Store.AppendMessage(ctx, conversationID, parentID, model.Message{
		Role:    model.RoleUser,
		Content: userText,
	}, nil)
	if err != nil {
		return "", "", err
	}

	// 4) Build context (includes the new user message)
	// 构建上下文
	// 此时 GetRecentMessages(conversationID) 理论上能查到刚才存的那条 userMsg 了
	buildResult, err := s.Builder.Build(ctx, ctxbuilder.BuildInput{
		ConversationID: conversationID,
		UserMsg:        "",
	})
	if err != nil {
		return "", "", err
	}
	msgs := buildResult.Messages

	// 5) Choose provider
	providerPriorityChain := s.Router.ChooseChain(ctx, userText)
	if len(providerPriorityChain) == 0 {
		return "", "", ErrNoProvider
	}

	// 6) Call provider
	var lastReply llm.ChatResponse
	var lastErr error

	hasPickedProvider := false
	for _, p := range providerPriorityChain {
		if p == nil {
			continue
		}
		hasPickedProvider = true
		lastReply, lastErr = p.Chat(ctx, msgs)
		if lastErr == nil {
			break
		}
		log.Printf("[Chat Service] AI reply failed: %v. Agent: %v", lastErr, p.Name())
	}
	if !hasPickedProvider {
		return "", "", ErrNoProvider
	}
	if lastErr != nil {
		return "", "", lastErr
	}

	// 7) 存储 AI 回复
	// 🚀 我们需要修改 AppendMessage 的签名，支持传入 Input/Output Tokens
	aiMsgID, err := s.Store.AppendMessage(ctx, conversationID, &newUserMsgID, model.Message{
		Role:         model.RoleAssistant,
		Content:      lastReply.Content,
		InputTokens:  lastReply.Usage.InputTokens,  // 🚀 记录消耗
		OutputTokens: lastReply.Usage.OutputTokens, // 🚀 记录消耗
	}, nil)
	if err != nil {
		return "", "", err
	}

	// 8) 更新 Conversation 状态并原子累加 Token
	// 🚀 我们在这里传入总消耗：Input + Output
	totalTokens := lastReply.Usage.InputTokens + lastReply.Usage.OutputTokens
	updatedTotal, err := s.Store.UpdateConversationStatus(ctx, conversationID, aiMsgID, parentID, totalTokens)
	if err != nil {
		return "", "", err
	}

	// 9) 检查是否触发 Summary 或 100w 提醒 (建议在 UpdateConversationStatus 内部逻辑或此处判断)
	if updatedTotal > 1000000 {
		log.Printf("⚠️ [Quota] Conv %s has reached %d tokens!", conversationID, updatedTotal)
		// 这里的逻辑可以根据你的业务需求扩展，比如发一个特殊的 emit 给前端
	}

	return lastReply.Content, conversationID, nil
}

func (s *Service) handleTitleGeneration(conversationID string, firstMsg string) {
	go func() {
		// 🚀 1. 终极防御：捕获 Panic，防止毁掉整个进程
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[TitleGen] RECOVERED from panic: %v", r)
			}
		}()

		// 🚀 2. 严格的 Nil 检查
		if s.Router == nil || s.Store == nil {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		p := s.Router.Choose("internal_task")
		if p == nil {
			return
		}

		msgs := []model.Message{
			{Role: model.RoleSystem, Content: TitleGeneratorPrompt},
			{Role: model.RoleUser, Content: "Target Message: " + firstMsg},
		}

		titleResponse, err := p.Chat(ctx, msgs)
		if err != nil {
			return
		}

		title := strings.Trim(titleResponse.Content, "\"\n ")
		titleDelta := titleResponse.Usage.InputTokens + titleResponse.Usage.OutputTokens

		_ = s.Store.IncrementConversationTokenUsage(ctx, conversationID, titleDelta)
		_ = s.Store.UpdateConversationTitle(ctx, conversationID, title)
	}()
}

// GetConversation 封装了对 Store 的查询，方便 Handler 调用
func (s *Service) GetConversation(ctx context.Context, convID string) (*model.Conversation, error) {
	// 假设你的 Store.GetConversation 返回的是 model.Conversation 实体
	conv, err := s.Store.GetConversation(ctx, convID)
	if err != nil {
		return nil, err
	}
	return conv, nil
}

// truncate 是一个简单的辅助函数，用于生成对话的初始标题
func truncate(s string, maxLen int) string {
	// 将字符串转为 rune 切片，确保按字符（而非字节）截断
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func (s *Service) getUserID(ctx context.Context) (uuid.UUID, error) {
	uid, ok := auth.GetUserID(ctx)
	if !ok {
		return uuid.Nil, errors.New("unauthorized: user identity missing from context")
	}
	return uid, nil
}

// ListUserConversations 获取属于该用户的所有对话
func (s *Service) ListUserConversations(ctx context.Context, userID uuid.UUID) ([]*model.Conversation, error) {
	// 这里直接调用我们刚刚在 PostgresStore 里写好的 DAO
	return s.Store.ListConversationsByUserID(ctx, userID)
}

// ListConversationMessages 返回值对象切片，以获得更好的 GC 性能和缓存局部性
// ListConversationMessages 现在支持分页参数 limit 和 beforeID
func (s *Service) ListConversationMessages(ctx context.Context, userID uuid.UUID, convID uuid.UUID, limit int, beforeID string) ([]model.Message, error) {
	// 1. 权限校验：确保用户只能查看自己的对话记录
	conv, err := s.Store.GetConversation(ctx, convID.String())
	if err != nil {
		return nil, fmt.Errorf("conversation not found: %w", err)
	}

	// 校验所有权
	if conv.UserID != userID {
		return nil, fmt.Errorf("access denied: user %s does not own conversation %s", userID, convID)
	}

	// 2. 调用 Store 获取消息
	// limit: 由 Handler 传入，决定一页加载多少条
	// order: 固定为 "asc"，因为 UI 渲染逻辑总是从旧到新
	// beforeID: 游标，由 Handler 从 URL Query 中提取
	return s.Store.GetMessagesByConvID(ctx, convID.String(), limit, "asc", beforeID)
}

// ListFavoriteMessages returns the current user's favorited messages.
func (s *Service) ListFavoriteMessages(
	ctx context.Context,
	userID uuid.UUID,
	limit int,
	offset int,
) ([]model.FavoriteMessageRow, error) {
	return s.Store.ListFavoriteMessages(ctx, userID.String(), limit, offset)
}

// ListMessagesForFavoriteJump returns:
// 1) up to olderBuffer messages before the favorite message
// 2) the favorite message itself
// 3) all messages after it up to the latest message in the conversation
func (s *Service) ListMessagesForFavoriteJump(
	ctx context.Context,
	userID uuid.UUID,
	convID uuid.UUID,
	messageID uuid.UUID,
	olderBuffer int,
) ([]model.Message, error) {
	conv, err := s.Store.GetConversation(ctx, convID.String())
	if err != nil {
		return nil, fmt.Errorf("conversation not found: %w", err)
	}

	if conv.UserID != userID {
		return nil, fmt.Errorf("access denied: user %s does not own conversation %s", userID, convID)
	}

	return s.Store.GetMessagesForFavoriteJump(ctx, convID.String(), messageID.String(), olderBuffer)
}

// SetMessageFavorite updates the favorite state of a message after ownership check.
func (s *Service) SetMessageFavorite(
	ctx context.Context,
	userID uuid.UUID,
	messageID uuid.UUID,
	isFavorite bool,
) error {
	msg, err := s.Store.GetMessageByID(ctx, messageID.String())
	if err != nil {
		return fmt.Errorf("message not found: %w", err)
	}

	conv, err := s.Store.GetConversation(ctx, msg.ConvID.String())
	if err != nil {
		return fmt.Errorf("conversation not found for message: %w", err)
	}

	if conv.UserID != userID {
		return fmt.Errorf("access denied: user %s does not own message %s", userID, messageID)
	}

	return s.Store.SetMessageFavorite(ctx, messageID.String(), isFavorite)
}

func (s *Service) SetMessageFavoriteLabel(
	ctx context.Context,
	userID uuid.UUID,
	messageID uuid.UUID,
	label *string,
) error {
	msg, err := s.Store.GetMessageByID(ctx, messageID.String())
	if err != nil {
		return fmt.Errorf("message not found: %w", err)
	}

	conv, err := s.Store.GetConversation(ctx, msg.ConvID.String())
	if err != nil {
		return fmt.Errorf("conversation not found for message: %w", err)
	}

	if conv.UserID != userID {
		return fmt.Errorf("access denied: user %s does not own message %s", userID, messageID)
	}

	return s.Store.SetMessageFavoriteLabel(ctx, messageID.String(), label)
}

func (s *Service) SetConversationPinned(
	ctx context.Context,
	userID uuid.UUID,
	convID uuid.UUID,
	isPinned bool,
) error {
	conv, err := s.Store.GetConversation(ctx, convID.String())
	if err != nil {
		return fmt.Errorf("conversation not found: %w", err)
	}

	if conv.UserID != userID {
		return fmt.Errorf("access denied: user %s does not own conversation %s", userID, convID)
	}

	if err := s.Store.SetConversationPinned(ctx, convID.String(), isPinned); err != nil {
		return fmt.Errorf("failed to update conversation pinned state: %w", err)
	}

	return nil
}

func (s *Service) ListPinnedConversations(
	ctx context.Context,
	userID uuid.UUID,
) ([]*model.Conversation, error) {
	convs, err := s.Store.ListPinnedConversationsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list pinned conversations: %w", err)
	}
	return convs, nil
}
