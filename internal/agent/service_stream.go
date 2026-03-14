package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"rhea-backend/internal/llm"
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
	var sb strings.Builder
	var finalUsage *llm.Usage // 🚀 新增：用于捕获流结束时的 Token 统计

	hasPickedProvider := false
	for _, p := range providerPriorityChain {
		if p == nil {
			continue
		}
		hasPickedProvider = true
		sb.Reset()
		finalUsage = nil // 重置统计

		modelInfo := fmt.Sprintf("::__metadata__:model:%s:%s::", p.Name(), p.ModelName())
		_ = emit(modelInfo)

		// 7) Stream 核心改动：适配新的 emit 签名 (delta, usage)
		lastErr = p.Stream(ctx, msgs, func(delta string, usage *llm.Usage) error {
			if usage != nil {
				finalUsage = usage // 🚀 捕获到了最后的账单
			}
			if delta != "" {
				sb.WriteString(delta)
				return emit(delta)
			}
			return nil
		})

		if lastErr == nil {
			break
		}
		if sb.Len() > 0 {
			return conversationID, lastErr
		}
	}

	if !hasPickedProvider {
		return "", ErrNoProvider
	}
	if lastErr != nil {
		return conversationID, lastErr
	}

	// 7.5 持久化 Context
	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 8) 存储 AI 回复 (带上 Token 消耗)
	fullReply := sb.String()
	inputT, outputT := 0, 0
	if finalUsage != nil {
		inputT = finalUsage.InputTokens
		outputT = finalUsage.OutputTokens
	}

	aiMsgID, err := s.Store.AppendMessage(persistCtx, conversationID, &newUserMsgID, model.Message{
		Role:         model.RoleAssistant,
		Content:      fullReply,
		InputTokens:  inputT,  // 🚀 存入消息表
		OutputTokens: outputT, // 🚀 存入消息表
	}, nil)
	if err != nil {
		log.Printf("[ChatStream] Error saving AI message: %v", err)
		return conversationID, err
	}

	// 9) 【核心原子更新】更新 Conversation 指针 + 累加 Token
	totalDelta := inputT + outputT
	// 注意：这里调用的是你刚才重构的返回 int 的 UpdateConversationStatus
	updatedTotal, err := s.Store.UpdateConversationStatus(persistCtx, conversationID, aiMsgID, parentID, totalDelta)
	if err != nil {
		log.Printf("[ChatStream] Error updating pointer and tokens: %v", err)
		return conversationID, err
	}

	// 10) 阈值检查 (100万 Token 预警)
	if updatedTotal > 1000000 {
		log.Printf("⚠️ [Quota] Conv %s has reached %d tokens!", conversationID, updatedTotal)
		// 这里的逻辑可以根据你的业务需求扩展，比如发一个特殊的 emit 给前端
	}

	return conversationID, nil
}
