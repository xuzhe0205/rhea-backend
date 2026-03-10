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
	"rhea-backend/internal/model"
	"rhea-backend/internal/router"
	"rhea-backend/internal/store"

	"github.com/google/uuid"
)

var ErrNoProvider = errors.New("no provider available")

// var tempUserID = uuid.MustParse("04260bc3-0f0b-4268-a079-21375a2340ea")

type Service struct {
	Store   store.Store
	Builder *ctxbuilder.Builder
	Router  *router.Router
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
	msgs, err := s.Builder.Build(ctx, conversationID, userText)
	if err != nil {
		return "", "", err
	}

	// 5) Choose provider
	providerPriorityChain := s.Router.ChooseChain(ctx, userText)
	if len(providerPriorityChain) == 0 {
		return "", "", ErrNoProvider
	}

	// 6) Call provider
	var lastReply string
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
		} else {
			log.Printf("[Chat Service] AI reply failed: %v. Agent: %v", lastErr, p.Name())
		}
	}
	if !hasPickedProvider {
		return "", "", ErrNoProvider
	}
	if lastErr != nil {
		return "", "", lastErr
	}

	// 7) Persist assistant reply only on success
	aiMsgID, err := s.Store.AppendMessage(ctx, conversationID, &newUserMsgID, model.Message{
		Role:    model.RoleAssistant,
		Content: lastReply,
	}, nil)
	if err != nil {
		return "", "", err
	}

	// 8) 别忘了更新 Conversation 的 LastMsgID
	// 这样下次 Chat 进来时，parentID 就能拿到这条 AI 消息的 ID
	err = s.Store.UpdateConversationStatus(ctx, conversationID, aiMsgID, parentID, 0)
	if err != nil {
		return "", "", err
	}

	return lastReply, conversationID, nil
}

func (s *Service) handleTitleGeneration(conversationID string, firstMsg string) {
	// 启动协程
	go func() {
		// 1. 创建一个独立的、不受主请求生命周期影响的 Context
		// 使用 context.Background() 确保请求结束后它还能跑
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		fmt.Printf("[TitleGen] Starting for conv: %s\n", conversationID)

		// 2. 构造 Prompt
		msgs := []model.Message{
			{Role: model.RoleSystem, Content: TitleGeneratorPrompt},
			{Role: model.RoleUser, Content: "Target Message: " + firstMsg},
		}

		// 3. 调用 Provider (建议选一个响应快的模型)
		p := s.Router.Choose("internal_task")
		title, err := p.Chat(ctx, msgs)
		if err != nil {
			fmt.Printf("[TitleGen] Error generating title: %v\n", err)
			return
		}

		// 4. 清洗数据并存入数据库
		title = strings.Trim(title, "\"\n ")
		if err := s.Store.UpdateConversationTitle(ctx, conversationID, title); err != nil {
			fmt.Printf("[TitleGen] Error saving title: %v\n", err)
			return
		}

		fmt.Printf("[TitleGen] Success: %s\n", title)
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
