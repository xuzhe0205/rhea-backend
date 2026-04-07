package agent

import (
	"context"
	"errors"
	"log"
	"math"
	"strings"
	"time"

	ctxbuilder "rhea-backend/internal/context"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/model"
	"rhea-backend/internal/pkg/netutil"
	"rhea-backend/internal/retrieval"

	"github.com/google/uuid"
)

type StreamCallbacks struct {
	OnDelta func(delta string) error
	OnMeta  func(payload map[string]any) error
	OnModel func(model string) error
	OnRag   func(payload map[string]any) error
}

func (s *Service) ChatStream(
	ctx context.Context,
	conversationID string,
	userText string,
	cb StreamCallbacks,
) (string, error) {
	if s.Store == nil || s.Builder == nil || s.Router == nil {
		return "", errors.New("agent service not configured")
	}
	if cb.OnDelta == nil {
		return "", errors.New("OnDelta callback is required")
	}

	// 0. 获取真实身份
	userID, err := s.getUserID(ctx)
	if err != nil {
		return "", err
	}

	// 1) 如果是新对话，先创建 conversation
	isNewConversation := conversationID == ""
	if isNewConversation {
		newID := uuid.New()
		_, err := s.Store.CreateConversation(ctx, &model.Conversation{
			ID:      newID,
			UserID:  userID,
			Title:   "New Chat: " + truncate(userText, 20),
			Summary: "",
		})
		if err != nil {
			return "", err
		}
		conversationID = newID.String()
		s.handleTitleGeneration(conversationID, userText)
	}

	// 2) 获取当前对话状态（拿到最新的 LastMsgID 作为父节点）
	conv, err := s.Store.GetConversation(ctx, conversationID)
	if err != nil {
		return "", err
	}
	if conv.UserID != userID {
		return "", errors.New("forbidden: access to conversation denied")
	}

	var parentID *string
	if conv.LastMsgID != nil {
		p := conv.LastMsgID.String()
		parentID = &p
	}

	// 3) 先落库 user message，拿到真实 ID
	newUserMsgID, err := s.Store.AppendMessage(ctx, conversationID, parentID, model.Message{
		Role:    model.RoleUser,
		Content: userText,
	}, nil)
	if err != nil {
		return "", err
	}

	// 4) 立刻把真实 conversation_id + user_message_id 发回前端
	if cb.OnMeta != nil {
		if err := cb.OnMeta(map[string]any{
			"conversation_id": conversationID,
			"user_message_id": newUserMsgID,
		}); err != nil {
			return conversationID, err
		}
	}

	// 5) 构建上下文（含 RAG 检索）
	buildResult, err := s.Builder.Build(ctx, ctxbuilder.BuildInput{
		ConversationID: conversationID,
		UserMsg:        userText,
	})
	if err != nil {
		return "", err
	}
	msgs := buildResult.Messages

	// 5a) 把 RAG 检索结果发给前端
	if cb.OnRag != nil {
		if err := cb.OnRag(computeRagStats(buildResult.RetrievedContext, string(buildResult.Scope))); err != nil {
			return conversationID, err
		}
	}

	// 6) 按你现有 Router 的流式 provider 链选择逻辑来跑
	var sb strings.Builder
	var finalUsage *llm.Usage
	var hasPickedProvider bool
	var lastErr error

	emit := func(delta string) error {
		sb.WriteString(delta)
		return cb.OnDelta(delta)
	}

	providerPriorityChain := s.Router.ChooseChain(ctx, userText)
	if len(providerPriorityChain) == 0 {
		return "", ErrNoProvider
	}

	for _, p := range providerPriorityChain {
		if p == nil {
			continue
		}
		hasPickedProvider = true

		log.Printf("[ChatStream] trying provider=%s", p.Name())

		// 这里假设你的 Provider.Stream 签名是：
		// Stream(ctx, msgs, func(delta string, usage *llm.Usage) error) error
		lastErr = p.Stream(ctx, msgs, func(delta string, usage *llm.Usage) error {
			if usage != nil {
				finalUsage = usage
			}
			if delta == "" {
				return nil
			}
			return emit(delta)
		})

		if lastErr == nil {
			break
		}
		if cb.OnModel != nil {
			_ = cb.OnModel(string(p.Name()) + ":" + p.ModelName())
		}

		// 如果已经吐出过内容，就不要再切 provider，直接返回错误
		if sb.Len() > 0 {
			return conversationID, lastErr
		}

		if netutil.IsRateLimitError(lastErr) || strings.Contains(lastErr.Error(), "503") {
			log.Printf("[ChatStream] provider=%s throttled: %v; trying next", p.Name(), lastErr)
			continue
		}

		log.Printf("[ChatStream] provider=%s failed: %v", p.Name(), lastErr)
		return conversationID, lastErr
	}

	if !hasPickedProvider {
		return "", ErrNoProvider
	}
	if lastErr != nil {
		return conversationID, lastErr
	}

	// 7) 新对话异步标题生成
	// if isNewConversation {
	// 	s.handleTitleGeneration(conversationID, userText)
	// }

	// 8) assistant 回复落库
	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fullReply := sb.String()
	inputT, outputT := 0, 0
	if finalUsage != nil {
		inputT = finalUsage.InputTokens
		outputT = finalUsage.OutputTokens
	}

	aiMsgID, err := s.Store.AppendMessage(persistCtx, conversationID, &newUserMsgID, model.Message{
		Role:         model.RoleAssistant,
		Content:      fullReply,
		InputTokens:  inputT,
		OutputTokens: outputT,
	}, nil)
	if err != nil {
		log.Printf("[ChatStream] Error saving AI message: %v", err)
		return conversationID, err
	}

	// 9) 更新 Conversation 状态并累加 token
	totalDelta := inputT + outputT
	updatedTotal, err := s.Store.UpdateConversationStatus(persistCtx, conversationID, aiMsgID, parentID, totalDelta)
	if err != nil {
		log.Printf("[ChatStream] Error updating pointer and tokens: %v", err)
		return conversationID, err
	}

	if s.Ingestor != nil {
		ingestCtx, cancelIngest := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancelIngest()

		if err := s.Ingestor.RebuildConversationSnapshot(ingestCtx, conversationID); err != nil {
			log.Printf("[ChatStream] Warning: failed to rebuild memory snapshot for conv=%s: %v", conversationID, err)
		} else {
			log.Printf("[ChatStream] Memory snapshot rebuilt for conv=%s", conversationID)
		}
	}

	if updatedTotal > 1000000 {
		log.Printf("⚠️ [Quota] Conv %s has reached %d tokens!", conversationID, updatedTotal)
	}

	// 10) assistant message 落库后，再把真实 assistant id 发给前端
	if cb.OnMeta != nil {
		payload := map[string]any{
			"conversation_id":      conversationID,
			"assistant_message_id": aiMsgID,
		}

		// 如果此时 title 已经生成好了，一并带回去；没有也没关系
		latestConv, err := s.Store.GetConversation(persistCtx, conversationID)
		if err == nil && strings.TrimSpace(latestConv.Title) != "" {
			payload["title"] = latestConv.Title
		}

		if err := cb.OnMeta(payload); err != nil {
			return conversationID, err
		}
	}

	return conversationID, nil
}

func computeRagStats(rc *retrieval.RetrievedContext, scope string) map[string]any {
	if rc == nil || len(rc.Chunks) == 0 {
		return map[string]any{
			"chunks_used":  0,
			"top_score":    0.0,
			"avg_score":    0.0,
			"vector_hits":  0,
			"keyword_hits": 0,
			"mode":         "none",
			"scope":        scope,
		}
	}

	topScore := 0.0
	totalScore := 0.0
	vectorHits := 0
	keywordHits := 0

	for _, ch := range rc.Chunks {
		if ch.FinalScore > topScore {
			topScore = ch.FinalScore
		}
		totalScore += ch.FinalScore
		if ch.VectorScore > 0.05 {
			vectorHits++
		}
		if ch.KeywordScore > 0.05 {
			keywordHits++
		}
	}

	avgScore := totalScore / float64(len(rc.Chunks))

	mode := "hybrid"
	if vectorHits == 0 {
		mode = "keyword"
	} else if keywordHits == 0 {
		mode = "vector"
	}

	round3 := func(v float64) float64 {
		return math.Round(v*1000) / 1000
	}

	return map[string]any{
		"chunks_used":  len(rc.Chunks),
		"top_score":    round3(topScore),
		"avg_score":    round3(avgScore),
		"vector_hits":  vectorHits,
		"keyword_hits": keywordHits,
		"mode":         mode,
		"scope":        scope,
	}
}
