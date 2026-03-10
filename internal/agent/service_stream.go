package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"rhea-backend/internal/model"

	"github.com/google/uuid"
)

func (s *Service) ChatStream(
	ctx context.Context,
	conversationID string,
	userText string,
	emit func(delta string) error,
) (string, error) {
	if s.Store == nil || s.Builder == nil || s.Router == nil {
		return "", errors.New("agent service not configured")
	}
	if emit == nil {
		return "", errors.New("emit function is required")
	}

	// 0. 获取真实身份 🚀
	userID, err := s.getUserID(ctx)
	if err != nil {
		return "", err
	}

	// 1) Persist user message even if provider fails later
	isNewConversation := conversationID == ""
	if isNewConversation {
		newID := uuid.New()
		_, err := s.Store.CreateConversation(ctx, &model.Conversation{
			ID:     newID,
			UserID: userID,

			// TODO: 后期接入 Title Generator，根据 userText 生成标题
			// 目前先简单截取前 20 个字符或硬编码
			Title: "New Chat: " + truncate(userText, 20),

			Summary: "", // 初始摘要为空
		})
		if err != nil {
			return "", err
		}
		conversationID = newID.String()
	}

	// 2) 获取当前 parentID (用于存储用户消息和最后的更新)
	conv, err := s.Store.GetConversation(ctx, conversationID)
	if err != nil {
		return "", err
	}

	// 🛡️ 增加所有权检查：确保这个对话真的属于当前登录用户
	if conv.UserID != userID {
		return "", errors.New("forbidden: access to conversation denied")
	}

	var parentID *string
	if conv.LastMsgID != nil {
		p := conv.LastMsgID.String()
		parentID = &p
	}

	// 3) 存储用户消息 (拿到 newUserMsgID)
	newUserMsgID, err := s.Store.AppendMessage(ctx, conversationID, parentID, model.Message{
		Role:    model.RoleUser,
		Content: userText,
	}, nil)
	if err != nil {
		return conversationID, err
	}

	// 4) 异步处理title的生成，运用LLM，并且在这个异步方法内把title更新到conversation_entities
	if isNewConversation {
		s.handleTitleGeneration(conversationID, userText)
	}

	// 5) Build Context (此时包含了刚才存的消息)
	msgs, err := s.Builder.Build(ctx, conversationID, "")
	if err != nil {
		return conversationID, err
	}

	// 6) Choose and call provider
	// 传入 ctx 触发智能路由
	providerPriorityChain := s.Router.ChooseChain(ctx, userText)
	if len(providerPriorityChain) == 0 {
		return "", ErrNoProvider
	}

	var lastErr error

	hasPickedProvider := false
	var sb strings.Builder
	for _, p := range providerPriorityChain {
		if p == nil {
			continue
		}
		hasPickedProvider = true
		// 🚀 关键：每次尝试新的 Provider 前，重置 Builder
		sb.Reset()
		// 🚀 核心改动：在发送正式内容前，先 emit 一个标识包
		// 使用特殊的识别前缀，方便前端拦截
		modelInfo := fmt.Sprintf("::__metadata__:model:%s:%s::", p.Name(), p.ModelName())
		_ = emit(modelInfo)
		// 7) Stream from provider, while accumulating final reply for persistence
		lastErr = p.Stream(ctx, msgs, func(delta string) error {
			sb.WriteString(delta)
			return emit(delta)
		})
		if lastErr == nil {
			break
		}
		// 💡 进阶逻辑判断：
		// 如果 sb.Len() > 0，说明已经有部分内容发给前端了。
		// 这种情况下，切换到下一个 Provider 会导致前端内容重复。
		// 建议：此时直接返回错误，不再重试。
		if sb.Len() > 0 {
			log.Printf("[ChatStream] Stream interrupted mid-way for provider %s: %v", p.Name(), lastErr)
			return conversationID, lastErr
		}

		log.Printf("[ChatStream] AI reply failed to start: %v. Trying next agent: %v", lastErr, p.Name())
	}
	if !hasPickedProvider {
		return "", ErrNoProvider
	}
	if lastErr != nil {
		log.Printf("[ChatStream] Provider stream error: %v | ConvID: %s", lastErr, conversationID)
		return conversationID, lastErr
	}

	// 7.5
	// 🚀 优化：创建一个“不会随请求断开而取消”的 Context 用于持久化
	// 这样即便用户在 AI 说话时关掉浏览器，我们也能把已生成的内容存好
	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 8) 【关键】流成功结束后，存储全量 AI 回复
	fullReply := sb.String()
	aiMsgID, err := s.Store.AppendMessage(persistCtx, conversationID, &newUserMsgID, model.Message{
		Role:    model.RoleAssistant,
		Content: fullReply,
	}, nil)
	if err != nil {
		log.Printf("[ChatStream] Error saving AI message: %v", err)
		return conversationID, err
	}

	// 9) 【关键】更新 Conversation 指针 (从 parentID 更新到 aiMsgID)
	if err := s.Store.UpdateConversationStatus(persistCtx, conversationID, aiMsgID, parentID, 0); err != nil {
		log.Printf("[ChatStream] Error updating pointer: %v", err)
		return conversationID, err // 同样报错，因为这会导致后续对话逻辑错误
	}

	return conversationID, nil
}
