/*
Class memory_store's responsibilities:
- Actually storing data
- Preventing concurrent race conditions
- Returning cloned slices to prevent mutation bugs
- Generating a message ID (string) to match PostgresStore behavior
*/

package store

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"rhea-backend/internal/model"
)

type memoryMsg struct {
	ID       string
	ParentID *string
	Msg      model.Message
	Metadata map[string]interface{}
}

type memoryConv struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Title     string
	LastMsgID *uuid.UUID
	TokenSum  int    // 👈 存放在这里，因为 domain model 里没有
	Summary   string // 👈 存放在这里
	UpdatedAt time.Time
}

// 1. 在 MemoryStore 结构体中增加 annotations map
type MemoryStore struct {
	mu            sync.RWMutex
	messages      map[string][]memoryMsg
	conversations map[string]*memoryConv
	summary       map[string]string
	users         map[string]*model.User
	// 新增：key 为 annotation 的 ID 字符串
	annotations map[string]*model.Annotation
}

// 2. 在 NewMemoryStore 中初始化 map
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		messages:      make(map[string][]memoryMsg),
		conversations: make(map[string]*memoryConv),
		summary:       make(map[string]string),
		users:         make(map[string]*model.User),
		annotations:   make(map[string]*model.Annotation), // 👈 初始化
	}
}

// AppendMessage implements store.Store.
// parentID can be nil to represent a root message.
// metadata can be nil.
func (s *MemoryStore) AppendMessage(
	ctx context.Context,
	conversationID string,
	parentID *string,
	msg model.Message,
	metadata map[string]interface{},
) (string, error) {
	_ = ctx // currently unused, kept to match interface

	s.mu.Lock()
	defer s.mu.Unlock()

	id := uuid.NewString()

	var parentCopy *string
	if parentID != nil {
		p := *parentID // copy value
		parentCopy = &p
	}

	var metaCopy map[string]interface{}
	if metadata != nil {
		metaCopy = cloneMetadata(metadata)
	}

	s.messages[conversationID] = append(s.messages[conversationID], memoryMsg{
		ID:       id,
		ParentID: parentCopy,
		Msg:      msg,
		Metadata: metaCopy,
	})

	return id, nil
}

// func (s *MemoryStore) GetRecentMessages(ctx context.Context, conversationID string, limit int) ([]model.Message, error) {
// 	_ = ctx

// 	s.mu.RLock()
// 	defer s.mu.RUnlock()

// 	all := s.messages[conversationID]
// 	if limit <= 0 || len(all) <= limit {
// 		return cloneMsgs(extractMsgs(all)), nil
// 	}

// 	return cloneMsgs(extractMsgs(all[len(all)-limit:])), nil
// }

// GetMessagesByConvID 统一了 LLM 取上下文和 UI 取历史记录的逻辑
func (s *MemoryStore) GetMessagesByConvID(ctx context.Context, conversationID string, limit int, order string, beforeID string) ([]model.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. 获取该对话的所有原始消息
	rawMsgs, ok := s.messages[conversationID]
	if !ok {
		return []model.Message{}, nil
	}

	// 2. 拷贝一份用于处理，避免并发读写冲突
	temp := make([]memoryMsg, len(rawMsgs))
	copy(temp, rawMsgs)

	// 3. 模拟数据库的“初始状态”：由于 Append 是顺序的，temp 现在是 ASC
	// 但为了模拟 Postgres 的逻辑（最新的在前才能 Limit），我们先统一转成 DESC
	for i, j := 0, len(temp)-1; i < j; i, j = i+1, j-1 {
		temp[i], temp[j] = temp[j], temp[i]
	}

	// 4. 处理 beforeID 游标过滤 (Cursor)
	var filtered []memoryMsg
	if beforeID != "" {
		foundCursor := false
		for i, m := range temp {
			if m.ID == beforeID {
				// 找到了作为游标的消息，取它之后的所有消息（即比它更旧的消息）
				if i+1 < len(temp) {
					filtered = temp[i+1:]
				}
				foundCursor = true
				break
			}
		}
		if !foundCursor {
			return nil, fmt.Errorf("before_id not found in memory: %s", beforeID)
		}
	} else {
		filtered = temp
	}

	// 5. 处理 Limit
	var limited []memoryMsg
	if limit > 0 && len(filtered) > limit {
		limited = filtered[:limit]
	} else {
		limited = filtered
	}

	// 6. 转换回 Domain Model
	msgs := extractMsgs(limited)

	// 7. 处理最终排序：如果要求 asc (UI/LLM 需要)，则再次反转
	// 因为 limited 现在是 [新...旧]
	if order == "asc" {
		for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
			msgs[i], msgs[j] = msgs[j], msgs[i]
		}
	}

	return msgs, nil
}

func (s *MemoryStore) CreateConversation(ctx context.Context, conv *model.Conversation) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 如果 Domain 对象里没有 ID，我们生成一个
	if conv.ID == uuid.Nil {
		conv.ID = uuid.New()
	}

	// 2. 将 Domain 转换并存入内存实体 (memoryConv)
	// 这样即便之后 conv 指针在外部被修改，也不会影响我们存好的数据
	s.conversations[conv.ID.String()] = &memoryConv{
		ID:        conv.ID,
		UserID:    conv.UserID,
		Title:     conv.Title,
		LastMsgID: conv.LastMsgID,
		Summary:   conv.Summary,
		TokenSum:  0,          // 初始 Token 为 0
		UpdatedAt: time.Now(), // 记录创建/更新时间
	}

	return conv.ID.String(), nil
}

func (s *MemoryStore) GetConversation(ctx context.Context, id string) (*model.Conversation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	mc, ok := s.conversations[id]
	if !ok {
		return nil, fmt.Errorf("conversation not found: %s", id)
	}

	return &model.Conversation{
		ID:               mc.ID,
		UserID:           mc.UserID,
		Title:            mc.Title,
		LastMsgID:        mc.LastMsgID,
		Summary:          mc.Summary,
		CumulativeTokens: mc.TokenSum, // 🚀 映射回 model
	}, nil
}

// UpdateConversationStatus 实现：包含指针更新、Token 累加和乐观锁校验
func (s *MemoryStore) UpdateConversationStatus(ctx context.Context, convID string, newLastMsgID string, expectedOldMsgID *string, tokenDelta int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.conversations[convID]
	if !ok {
		return 0, fmt.Errorf("conversation not found: %s", convID)
	}

	// 乐观锁校验
	if expectedOldMsgID != nil && *expectedOldMsgID != "" {
		currentLastID := ""
		if conv.LastMsgID != nil {
			currentLastID = conv.LastMsgID.String()
		}
		if currentLastID != *expectedOldMsgID {
			return 0, fmt.Errorf("concurrent_conflict") // 模拟 Postgres 冲突
		}
	} else if conv.LastMsgID != nil {
		// 如果期望是 NULL 但实际不是，也算冲突
		return 0, fmt.Errorf("concurrent_conflict")
	}

	uNewID, _ := uuid.Parse(newLastMsgID)
	conv.LastMsgID = &uNewID
	conv.TokenSum += tokenDelta // 🚀 原子累加
	conv.UpdatedAt = time.Now()

	return conv.TokenSum, nil // 🚀 返回最新总额
}

func (s *MemoryStore) IncrementConversationTokenUsage(ctx context.Context, convID string, delta int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if conv, ok := s.conversations[convID]; ok {
		conv.TokenSum += delta
		conv.UpdatedAt = time.Now()
		return nil
	}
	return fmt.Errorf("not found")
}

func (s *MemoryStore) GetSummary(ctx context.Context, conversationID string) (string, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.summary[conversationID], nil
}

func (s *MemoryStore) SetSummary(ctx context.Context, conversationID string, summary string) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()
	s.summary[conversationID] = summary
	return nil
}

func (s *MemoryStore) UpdateConversationTitle(ctx context.Context, convID string, title string) error {
	_ = ctx // 模拟接口

	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.conversations[convID]
	if !ok {
		return fmt.Errorf("conversation not found: %s", convID)
	}

	// 更新标题和时间戳
	conv.Title = title
	conv.UpdatedAt = time.Now()

	return nil
}

// CreateUser 模拟数据库插入用户
func (s *MemoryStore) CreateUser(ctx context.Context, user *model.User) (*model.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 模拟唯一索引校验
	if _, exists := s.users[user.Email]; exists {
		return nil, fmt.Errorf("user already exists: %s", user.Email)
	}

	// 生成 ID (如果还没有)
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}

	// 存入内存
	// 注意：这里建议存一份克隆，防止外部修改指针影响 Store
	userCopy := *user
	s.users[user.Email] = &userCopy

	return user, nil
}

// GetUserByEmail 模拟数据库查询用户
func (s *MemoryStore) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[email]
	if !ok {
		return nil, fmt.Errorf("user not found: %s", email)
	}

	// 返回克隆对象，防止外部直接修改哈希值等敏感数据
	userCopy := *user
	return &userCopy, nil
}

// GetUserByID 模拟数据库通过 ID 查询用户
func (s *MemoryStore) GetUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 遍历 map 查找 ID 匹配的用户 (内存实现稍微笨一点，但也最直观)
	for _, user := range s.users {
		if user.ID == id {
			userCopy := *user
			return &userCopy, nil
		}
	}

	return nil, fmt.Errorf("user not found with id: %s", id)
}

// ListConversationsByUserID 实现接口：获取特定用户的所有对话，按更新时间降序排列
func (s *MemoryStore) ListConversationsByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Conversation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*memoryConv

	// 1. 过滤属于该用户的对话
	for _, mc := range s.conversations {
		if mc.UserID == userID {
			results = append(results, mc)
		}
	}

	// 2. 模拟数据库的 Order By updated_at DESC
	sort.Slice(results, func(i, j int) bool {
		return results[i].UpdatedAt.After(results[j].UpdatedAt)
	})

	// 3. 转换为 Domain Model 数组
	finalResult := make([]*model.Conversation, len(results))
	for i, mc := range results {
		finalResult[i] = &model.Conversation{
			ID:        mc.ID,
			UserID:    mc.UserID,
			Title:     mc.Title,
			LastMsgID: mc.LastMsgID,
			Summary:   mc.Summary,
		}
	}

	return finalResult, nil
}

// --- 实现 Annotation (Rich Text) 接口 ---

// SaveAnnotation 实现存储逻辑
func (s *MemoryStore) SaveAnnotation(ctx context.Context, ann *model.Annotation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ann.ID == uuid.Nil {
		ann.ID = uuid.New()
	}

	// 存储一份克隆，模拟数据库的持久化行为，防止外部指针修改
	annCopy := *ann
	s.annotations[ann.ID.String()] = &annCopy
	return nil
}

// GetAnnotationByFeature 模拟精确特征查找
func (s *MemoryStore) GetAnnotationByFeature(ctx context.Context, msgID uuid.UUID, start, end int, annType model.AnnotationType) (*model.Annotation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, a := range s.annotations {
		if a.MessageID == msgID && a.RangeStart == start && a.RangeEnd == end && a.Type == annType {
			res := *a
			return &res, nil
		}
	}
	return nil, fmt.Errorf("annotation not found") // 模拟 GORM 的 RecordNotFound
}

// DeleteAnnotationsByRangeAndTypes 模拟范围和类型的批量删除
func (s *MemoryStore) DeleteAnnotationsByRangeAndTypes(ctx context.Context, msgID uuid.UUID, start, end int, types []model.AnnotationType) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	typeMap := make(map[model.AnnotationType]bool)
	for _, t := range types {
		typeMap[t] = true
	}

	for id, a := range s.annotations {
		// 匹配逻辑：同一条消息 + 类型在列表中 + 范围完全一致 (或按需改为重叠)
		// 这里采用完全一致逻辑，与 Service 层调用匹配
		if a.MessageID == msgID && a.RangeStart == start && a.RangeEnd == end && typeMap[a.Type] {
			delete(s.annotations, id)
		}
	}
	return nil
}

// DeleteAnnotation 模拟带权限校验的删除
func (s *MemoryStore) DeleteAnnotation(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sid := id.String()
	ann, ok := s.annotations[sid]
	if !ok {
		return nil // 模拟 Delete Ignore
	}

	// 模拟越权校验
	if ann.UserID != userID {
		return fmt.Errorf("permission denied")
	}

	delete(s.annotations, sid)
	return nil
}

// ListAnnotationsByMessageID 模拟按消息拉取并排序
func (s *MemoryStore) ListAnnotationsByMessageID(ctx context.Context, msgID uuid.UUID, userID uuid.UUID) ([]*model.Annotation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*model.Annotation
	for _, a := range s.annotations {
		// 增加 userID 过滤，确保只能看到自己的标注
		if a.MessageID == msgID && a.UserID == userID {
			copyAnn := *a
			list = append(list, &copyAnn)
		}
	}

	// 模拟数据库的 CreatedAt ASC 排序 (假设 ID 生成顺序或手动排序)
	// 简单起见，这里按 RangeStart 排序，实际生产中应记录 CreatedAt
	sort.Slice(list, func(i, j int) bool {
		return list[i].RangeStart < list[j].RangeStart
	})

	return list, nil
}

// --- helpers ---

func extractMsgs(in []memoryMsg) []model.Message {
	out := make([]model.Message, len(in))
	for i := range in {
		out[i] = in[i].Msg
	}
	return out
}

func cloneMsgs(in []model.Message) []model.Message {
	out := make([]model.Message, len(in))
	copy(out, in)
	return out
}

func cloneMetadata(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
