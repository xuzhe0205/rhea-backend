/*
Class memory_store's responsibilities:
- Actually storing data
- Preventing concurrent race conditions
- Returning cloned slices to prevent mutation bugs
- Generating IDs to match PostgresStore behavior
*/

package store

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"rhea-backend/internal/model"
	"rhea-backend/internal/rag"
)

type memoryMsg struct {
	ID             string
	ConversationID string
	ParentID       *string
	Msg            model.Message
	Metadata       map[string]interface{}
	FavoritedAt    *time.Time
}

type memoryConv struct {
	ID                    uuid.UUID
	UserID                uuid.UUID
	ProjectID             *uuid.UUID
	Title                 string
	LastMsgID             *uuid.UUID
	TokenSum              int
	Summary               string
	UpdatedAt             time.Time
	IsPinned              bool
	PinnedAt              *time.Time
	SummaryUpdatedAt      *time.Time
	MemoryCheckpointMsgID *uuid.UUID
}

type MemoryStore struct {
	mu sync.RWMutex

	messages      map[string][]memoryMsg // convID -> msgs (ASC by append order)
	conversations map[string]*memoryConv // convID -> conv
	summary       map[string]string      // convID -> summary
	users         map[string]*model.User // email -> user

	annotations    map[string]*model.Annotation    // annID -> ann
	commentThreads map[string]*model.CommentThread // threadID -> thread
	comments       map[string]*model.Comment       // commentID -> comment

	memoryDocuments  map[uuid.UUID]*model.MemoryDocumentEntity  // docID -> doc
	memoryChunks     map[uuid.UUID]*model.MemoryChunkEntity     // chunkID -> chunk
	memoryEmbeddings map[uuid.UUID]*model.MemoryEmbeddingEntity // chunkID -> embedding
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		messages:         make(map[string][]memoryMsg),
		conversations:    make(map[string]*memoryConv),
		summary:          make(map[string]string),
		users:            make(map[string]*model.User),
		annotations:      make(map[string]*model.Annotation),
		commentThreads:   make(map[string]*model.CommentThread),
		comments:         make(map[string]*model.Comment),
		memoryDocuments:  make(map[uuid.UUID]*model.MemoryDocumentEntity),
		memoryChunks:     make(map[uuid.UUID]*model.MemoryChunkEntity),
		memoryEmbeddings: make(map[uuid.UUID]*model.MemoryEmbeddingEntity),
	}
}

func (s *MemoryStore) AppendMessage(
	ctx context.Context,
	conversationID string,
	parentID *string,
	msg model.Message,
	metadata map[string]interface{},
) (string, error) {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.conversations[conversationID]; !ok {
		return "", fmt.Errorf("conversation not found")
	}

	id := uuid.NewString()
	now := time.Now()

	var parentCopy *string
	if parentID != nil && *parentID != "" {
		p := *parentID
		parentCopy = &p
	}

	var metaCopy map[string]interface{}
	if metadata != nil {
		metaCopy = cloneMetadata(metadata)
	}

	msgCopy := cloneMessage(msg)
	parsedID, _ := uuid.Parse(id)
	convUUID, _ := uuid.Parse(conversationID)

	msgCopy.ID = parsedID
	msgCopy.ConvID = convUUID
	msgCopy.CreatedAt = now
	msgCopy.Metadata = metaCopy

	s.messages[conversationID] = append(s.messages[conversationID], memoryMsg{
		ID:             id,
		ConversationID: conversationID,
		ParentID:       parentCopy,
		Msg:            msgCopy,
		Metadata:       metaCopy,
	})

	return id, nil
}

func (s *MemoryStore) GetMessagesByConvID(
	ctx context.Context,
	conversationID string,
	limit int,
	order string,
	beforeID string,
) ([]model.Message, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	rawMsgs, ok := s.messages[conversationID]
	if !ok {
		return []model.Message{}, nil
	}

	temp := make([]memoryMsg, len(rawMsgs))
	copy(temp, rawMsgs)

	// Match Postgres default query ordering: newest first
	reverseMemoryMsgs(temp)

	var filtered []memoryMsg
	if beforeID != "" {
		foundCursor := false
		for i, m := range temp {
			if m.ID == beforeID {
				if i+1 < len(temp) {
					filtered = temp[i+1:]
				} else {
					filtered = []memoryMsg{}
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

	var limited []memoryMsg
	if limit > 0 && len(filtered) > limit {
		limited = filtered[:limit]
	} else {
		limited = filtered
	}

	msgs := extractDomainMsgs(limited)

	if order == "asc" {
		for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
			msgs[i], msgs[j] = msgs[j], msgs[i]
		}
	}

	return msgs, nil
}

func (s *MemoryStore) GetMessagesForFavoriteJump(
	ctx context.Context,
	conversationID string,
	messageID string,
	olderBuffer int,
) ([]model.Message, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	rawMsgs, ok := s.messages[conversationID]
	if !ok {
		return nil, fmt.Errorf("conversation not found")
	}

	anchorIdx := -1
	for i, m := range rawMsgs {
		if m.ID == messageID {
			anchorIdx = i
			break
		}
	}
	if anchorIdx == -1 {
		return nil, fmt.Errorf("favorite message not found in conversation")
	}

	start := anchorIdx - olderBuffer
	if start < 0 {
		start = 0
	}

	combined := rawMsgs[start:]
	return extractDomainMsgs(combined), nil
}

func (s *MemoryStore) SetMessageFavorite(
	ctx context.Context,
	messageID string,
	isFavorite bool,
) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	for convID, msgs := range s.messages {
		for i := range msgs {
			if msgs[i].ID == messageID {
				msgs[i].Msg.IsFavorite = isFavorite
				if isFavorite {
					now := time.Now()
					msgs[i].FavoritedAt = &now
				} else {
					msgs[i].FavoritedAt = nil
					msgs[i].Msg.FavoriteLabel = nil
				}
				s.messages[convID][i] = msgs[i]
				return nil
			}
		}
	}

	return fmt.Errorf("message not found")
}

func (s *MemoryStore) ListFavoriteMessages(
	ctx context.Context,
	userID string,
	limit int,
	offset int,
) ([]model.FavoriteMessageRow, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user uuid: %w", err)
	}

	rows := make([]model.FavoriteMessageRow, 0)

	for convID, conv := range s.conversations {
		if conv.UserID != userUUID {
			continue
		}
		msgs := s.messages[convID]
		for _, mm := range msgs {
			if mm.Msg.IsFavorite {
				rows = append(rows, model.FavoriteMessageRow{
					ID:            mm.Msg.ID,
					ConvID:        mm.Msg.ConvID,
					Role:          mm.Msg.Role,
					Content:       mm.Msg.Content,
					CreatedAt:     mm.Msg.CreatedAt,
					FavoritedAt:   mm.FavoritedAt,
					FavoriteLabel: mm.Msg.FavoriteLabel,
				})
			}
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		var ti, tj time.Time
		if rows[i].FavoritedAt != nil {
			ti = *rows[i].FavoritedAt
		}
		if rows[j].FavoritedAt != nil {
			tj = *rows[j].FavoritedAt
		}
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.After(rows[j].CreatedAt)
		}
		return rows[i].ID.String() > rows[j].ID.String()
	})

	if offset > 0 {
		if offset >= len(rows) {
			return []model.FavoriteMessageRow{}, nil
		}
		rows = rows[offset:]
	}
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}

	return rows, nil
}

func (s *MemoryStore) GetMessageByID(ctx context.Context, messageID string) (*model.Message, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, msgs := range s.messages {
		for _, mm := range msgs {
			if mm.ID == messageID {
				msg := cloneMessage(mm.Msg)
				return &msg, nil
			}
		}
	}

	return nil, fmt.Errorf("message not found")
}

func (s *MemoryStore) SetMessageFavoriteLabel(
	ctx context.Context,
	messageID string,
	label *string,
) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	for convID, msgs := range s.messages {
		for i := range msgs {
			if msgs[i].ID == messageID {
				msgs[i].Msg.FavoriteLabel = cloneStringPtr(label)
				s.messages[convID][i] = msgs[i]
				return nil
			}
		}
	}

	return fmt.Errorf("message not found")
}

func (s *MemoryStore) CreateConversation(ctx context.Context, conv *model.Conversation) (string, error) {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	if conv.ID == uuid.Nil {
		conv.ID = uuid.New()
	}

	s.conversations[conv.ID.String()] = &memoryConv{
		ID:                    conv.ID,
		UserID:                conv.UserID,
		ProjectID:             cloneUUIDPtr(conv.ProjectID),
		Title:                 conv.Title,
		LastMsgID:             cloneUUIDPtr(conv.LastMsgID),
		Summary:               conv.Summary,
		TokenSum:              conv.CumulativeTokens,
		UpdatedAt:             time.Now(),
		IsPinned:              conv.IsPinned,
		PinnedAt:              cloneTimePtr(conv.PinnedAt),
		SummaryUpdatedAt:      cloneTimePtr(conv.SummaryUpdatedAt),
		MemoryCheckpointMsgID: cloneUUIDPtr(conv.MemoryCheckpointMsgID),
	}

	return conv.ID.String(), nil
}

func (s *MemoryStore) GetConversation(ctx context.Context, id string) (*model.Conversation, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	mc, ok := s.conversations[id]
	if !ok {
		return nil, fmt.Errorf("conversation not found: %s", id)
	}

	return &model.Conversation{
		ID:                    mc.ID,
		UserID:                mc.UserID,
		ProjectID:             cloneUUIDPtr(mc.ProjectID),
		Title:                 mc.Title,
		LastMsgID:             cloneUUIDPtr(mc.LastMsgID),
		Summary:               mc.Summary,
		IsPinned:              mc.IsPinned,
		PinnedAt:              cloneTimePtr(mc.PinnedAt),
		CumulativeTokens:      mc.TokenSum,
		SummaryUpdatedAt:      cloneTimePtr(mc.SummaryUpdatedAt),
		MemoryCheckpointMsgID: cloneUUIDPtr(mc.MemoryCheckpointMsgID),
	}, nil
}

// Match Postgres behavior: no optimistic-lock enforcement anymore.
func (s *MemoryStore) UpdateConversationStatus(
	ctx context.Context,
	convID string,
	newLastMsgID string,
	_ *string,
	tokenDelta int,
) (int, error) {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.conversations[convID]
	if !ok {
		return 0, fmt.Errorf("conversation not found: %s", convID)
	}

	uNewID, err := uuid.Parse(newLastMsgID)
	if err != nil {
		return 0, fmt.Errorf("invalid message id: %w", err)
	}

	conv.LastMsgID = &uNewID
	conv.TokenSum += tokenDelta
	conv.UpdatedAt = time.Now()

	return conv.TokenSum, nil
}

func (s *MemoryStore) UpdateConversationTitle(ctx context.Context, convID string, title string) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.conversations[convID]
	if !ok {
		return fmt.Errorf("conversation not found: %s", convID)
	}

	conv.Title = title
	conv.UpdatedAt = time.Now()
	return nil
}

func (s *MemoryStore) ListConversationsByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Conversation, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*memoryConv
	for _, mc := range s.conversations {
		if mc.UserID == userID {
			results = append(results, mc)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].IsPinned != results[j].IsPinned {
			return results[i].IsPinned && !results[j].IsPinned
		}

		if results[i].PinnedAt != nil || results[j].PinnedAt != nil {
			if results[i].PinnedAt == nil {
				return false
			}
			if results[j].PinnedAt == nil {
				return true
			}
			if !results[i].PinnedAt.Equal(*results[j].PinnedAt) {
				return results[i].PinnedAt.After(*results[j].PinnedAt)
			}
		}

		return results[i].UpdatedAt.After(results[j].UpdatedAt)
	})

	out := make([]*model.Conversation, len(results))
	for i, mc := range results {
		out[i] = &model.Conversation{
			ID:                    mc.ID,
			UserID:                mc.UserID,
			ProjectID:             cloneUUIDPtr(mc.ProjectID),
			Title:                 mc.Title,
			LastMsgID:             cloneUUIDPtr(mc.LastMsgID),
			Summary:               mc.Summary,
			IsPinned:              mc.IsPinned,
			PinnedAt:              cloneTimePtr(mc.PinnedAt),
			CumulativeTokens:      mc.TokenSum,
			SummaryUpdatedAt:      cloneTimePtr(mc.SummaryUpdatedAt),
			MemoryCheckpointMsgID: cloneUUIDPtr(mc.MemoryCheckpointMsgID),
		}
	}
	return out, nil
}

func (s *MemoryStore) IncrementConversationTokenUsage(ctx context.Context, convID string, delta int) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	if conv, ok := s.conversations[convID]; ok {
		conv.TokenSum += delta
		conv.UpdatedAt = time.Now()
		return nil
	}
	return fmt.Errorf("not found")
}

func (s *MemoryStore) ListPinnedConversationsByUserID(ctx context.Context, userID uuid.UUID) ([]*model.Conversation, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*memoryConv
	for _, mc := range s.conversations {
		if mc.UserID == userID && mc.IsPinned {
			results = append(results, mc)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].PinnedAt == nil {
			return false
		}
		if results[j].PinnedAt == nil {
			return true
		}
		return results[i].PinnedAt.After(*results[j].PinnedAt)
	})

	out := make([]*model.Conversation, len(results))
	for i, mc := range results {
		out[i] = &model.Conversation{
			ID:                    mc.ID,
			UserID:                mc.UserID,
			ProjectID:             cloneUUIDPtr(mc.ProjectID),
			Title:                 mc.Title,
			LastMsgID:             cloneUUIDPtr(mc.LastMsgID),
			Summary:               mc.Summary,
			IsPinned:              mc.IsPinned,
			PinnedAt:              cloneTimePtr(mc.PinnedAt),
			CumulativeTokens:      mc.TokenSum,
			SummaryUpdatedAt:      cloneTimePtr(mc.SummaryUpdatedAt),
			MemoryCheckpointMsgID: cloneUUIDPtr(mc.MemoryCheckpointMsgID),
		}
	}
	return out, nil
}

func (s *MemoryStore) SetConversationPinned(ctx context.Context, convID string, isPinned bool) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.conversations[convID]
	if !ok {
		return fmt.Errorf("conversation not found")
	}

	conv.IsPinned = isPinned
	if isPinned {
		now := time.Now()
		conv.PinnedAt = &now
	} else {
		conv.PinnedAt = nil
	}
	return nil
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
	if conv, ok := s.conversations[conversationID]; ok {
		now := time.Now()
		conv.Summary = summary
		conv.SummaryUpdatedAt = &now
	}
	return nil
}

func (s *MemoryStore) CreateUser(ctx context.Context, user *model.User) (*model.User, error) {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[user.Email]; exists {
		return nil, fmt.Errorf("user already exists: %s", user.Email)
	}
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}

	userCopy := *user
	s.users[user.Email] = &userCopy

	out := userCopy
	return &out, nil
}

func (s *MemoryStore) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[email]
	if !ok {
		return nil, fmt.Errorf("user not found: %s", email)
	}

	userCopy := *user
	return &userCopy, nil
}

func (s *MemoryStore) GetUserByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, user := range s.users {
		if user.ID == id {
			userCopy := *user
			return &userCopy, nil
		}
	}

	return nil, fmt.Errorf("user not found with id: %s", id)
}

// --------------------
// Annotation methods
// --------------------

func (s *MemoryStore) SaveAnnotation(ctx context.Context, ann *model.Annotation) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	if ann.ID == uuid.Nil {
		ann.ID = uuid.New()
	}
	annCopy := cloneAnnotation(ann)
	s.annotations[ann.ID.String()] = annCopy
	return nil
}

func (s *MemoryStore) GetAnnotationByFeature(
	ctx context.Context,
	msgID uuid.UUID,
	start, end int,
	annType model.AnnotationType,
) (*model.Annotation, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, a := range s.annotations {
		if a.MessageID == msgID && a.RangeStart == start && a.RangeEnd == end && a.Type == annType {
			return cloneAnnotation(a), nil
		}
	}

	return nil, nil
}

func (s *MemoryStore) DeleteAnnotationsByRangeAndTypes(
	ctx context.Context,
	msgID uuid.UUID,
	start, end int,
	types []model.AnnotationType,
) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	typeSet := make(map[model.AnnotationType]struct{}, len(types))
	for _, t := range types {
		typeSet[t] = struct{}{}
	}

	for id, a := range s.annotations {
		if a.MessageID == msgID && a.RangeStart == start && a.RangeEnd == end {
			if _, ok := typeSet[a.Type]; ok {
				delete(s.annotations, id)
			}
		}
	}
	return nil
}

func (s *MemoryStore) DeleteAnnotation(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	ann, ok := s.annotations[id.String()]
	if !ok {
		return nil
	}
	if ann.UserID != userID {
		return fmt.Errorf("permission denied")
	}

	delete(s.annotations, id.String())
	return nil
}

func (s *MemoryStore) ListAnnotationsByMessageID(
	ctx context.Context,
	msgID uuid.UUID,
	userID uuid.UUID,
) ([]*model.Annotation, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*model.Annotation
	for _, a := range s.annotations {
		if a.MessageID == msgID && a.UserID == userID {
			list = append(list, cloneAnnotation(a))
		}
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].RangeStart != list[j].RangeStart {
			return list[i].RangeStart < list[j].RangeStart
		}
		if list[i].RangeEnd != list[j].RangeEnd {
			return list[i].RangeEnd < list[j].RangeEnd
		}
		return list[i].ID.String() < list[j].ID.String()
	})

	return list, nil
}

func (s *MemoryStore) ListAnnotationsByConversationAndMessageIDs(
	ctx context.Context,
	convID uuid.UUID,
	userID uuid.UUID,
	messageIDs []uuid.UUID,
) ([]*model.Annotation, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	msgSet := make(map[uuid.UUID]struct{}, len(messageIDs))
	for _, id := range messageIDs {
		msgSet[id] = struct{}{}
	}

	out := make([]*model.Annotation, 0)
	for _, a := range s.annotations {
		if a.ConvID != convID || a.UserID != userID {
			continue
		}
		if len(messageIDs) > 0 {
			if _, ok := msgSet[a.MessageID]; !ok {
				continue
			}
		}
		out = append(out, cloneAnnotation(a))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID.String() < out[j].ID.String()
	})

	return out, nil
}

func (s *MemoryStore) ListAnnotationsByMessageIDAndType(
	ctx context.Context,
	msgID uuid.UUID,
	userID uuid.UUID,
	annType model.AnnotationType,
) ([]*model.Annotation, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*model.Annotation, 0)
	for _, a := range s.annotations {
		if a.MessageID == msgID && a.UserID == userID && a.Type == annType {
			out = append(out, cloneAnnotation(a))
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].RangeStart != out[j].RangeStart {
			return out[i].RangeStart < out[j].RangeStart
		}
		return out[i].ID.String() < out[j].ID.String()
	})

	return out, nil
}

func (s *MemoryStore) DeleteAnnotationsByIDs(
	ctx context.Context,
	ids []uuid.UUID,
	userID uuid.UUID,
) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range ids {
		if ann, ok := s.annotations[id.String()]; ok && ann.UserID == userID {
			delete(s.annotations, id.String())
		}
	}
	return nil
}

// --------------------
// Comment thread methods
// --------------------

func (s *MemoryStore) CreateCommentThread(ctx context.Context, thread *model.CommentThread) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	if thread.ID == uuid.Nil {
		thread.ID = uuid.New()
	}
	now := time.Now()
	if thread.CreatedAt.IsZero() {
		thread.CreatedAt = now
	}
	if thread.UpdatedAt.IsZero() {
		thread.UpdatedAt = now
	}

	threadCopy := cloneCommentThread(thread)
	s.commentThreads[thread.ID.String()] = threadCopy
	return nil
}

func (s *MemoryStore) GetCommentThreadByRange(
	ctx context.Context,
	msgID uuid.UUID,
	userID uuid.UUID,
	start, end int,
) (*model.CommentThread, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, t := range s.commentThreads {
		if t.MessageID == msgID && t.UserID == userID && t.RangeStart == start && t.RangeEnd == end {
			out := cloneCommentThread(t)
			out.Comments = s.listCommentsByThreadIDLocked(t.ID, userID)
			return out, nil
		}
	}

	return nil, nil
}

func (s *MemoryStore) GetCommentThreadByID(
	ctx context.Context,
	threadID uuid.UUID,
	userID uuid.UUID,
) (*model.CommentThread, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.commentThreads[threadID.String()]
	if !ok || t.UserID != userID {
		return nil, nil
	}

	out := cloneCommentThread(t)
	out.Comments = s.listCommentsByThreadIDLocked(threadID, userID)
	return out, nil
}

func (s *MemoryStore) DeleteCommentThread(ctx context.Context, threadID uuid.UUID, userID uuid.UUID) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.commentThreads[threadID.String()]
	if !ok {
		return nil
	}
	if t.UserID != userID {
		return fmt.Errorf("permission denied")
	}

	delete(s.commentThreads, threadID.String())

	for id, c := range s.comments {
		if c.ThreadID == threadID && c.UserID == userID {
			delete(s.comments, id)
		}
	}

	return nil
}

func (s *MemoryStore) CreateComment(ctx context.Context, comment *model.Comment) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	if comment.ID == uuid.Nil {
		comment.ID = uuid.New()
	}
	now := time.Now()
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = now
	}
	if comment.UpdatedAt.IsZero() {
		comment.UpdatedAt = now
	}

	commentCopy := cloneComment(comment)
	s.comments[comment.ID.String()] = commentCopy

	if thread, ok := s.commentThreads[comment.ThreadID.String()]; ok {
		thread.UpdatedAt = now
	}
	return nil
}

func (s *MemoryStore) GetCommentByID(
	ctx context.Context,
	commentID uuid.UUID,
	userID uuid.UUID,
) (*model.Comment, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	c, ok := s.comments[commentID.String()]
	if !ok || c.UserID != userID {
		return nil, nil
	}

	return cloneComment(c), nil
}

func (s *MemoryStore) ListCommentsByThreadID(
	ctx context.Context,
	threadID uuid.UUID,
	userID uuid.UUID,
) ([]*model.Comment, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listCommentsByThreadIDLocked(threadID, userID), nil
}

func (s *MemoryStore) DeleteComment(ctx context.Context, commentID uuid.UUID, userID uuid.UUID) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.comments[commentID.String()]
	if !ok {
		return nil
	}
	if c.UserID != userID {
		return fmt.Errorf("permission denied")
	}

	delete(s.comments, commentID.String())
	return nil
}

func (s *MemoryStore) CountCommentsByThreadID(
	ctx context.Context,
	threadID uuid.UUID,
	userID uuid.UUID,
) (int64, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int64
	for _, c := range s.comments {
		if c.ThreadID == threadID && c.UserID == userID {
			count++
		}
	}
	return count, nil
}

func (s *MemoryStore) ListCommentThreadsByMessageIDs(
	ctx context.Context,
	userID uuid.UUID,
	messageIDs []uuid.UUID,
) ([]*model.CommentThread, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(messageIDs) == 0 {
		return []*model.CommentThread{}, nil
	}

	msgSet := make(map[uuid.UUID]struct{}, len(messageIDs))
	for _, id := range messageIDs {
		msgSet[id] = struct{}{}
	}

	out := make([]*model.CommentThread, 0)
	for _, t := range s.commentThreads {
		if t.UserID != userID {
			continue
		}
		if _, ok := msgSet[t.MessageID]; !ok {
			continue
		}

		threadCopy := cloneCommentThread(t)
		threadCopy.Comments = s.listCommentsByThreadIDLocked(t.ID, userID)
		out = append(out, threadCopy)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].MessageID != out[j].MessageID {
			return out[i].MessageID.String() < out[j].MessageID.String()
		}
		if out[i].RangeStart != out[j].RangeStart {
			return out[i].RangeStart < out[j].RangeStart
		}
		if out[i].RangeEnd != out[j].RangeEnd {
			return out[i].RangeEnd < out[j].RangeEnd
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})

	return out, nil
}

// --------------------
// RAG / Memory methods
// --------------------

func (s *MemoryStore) CreateMemoryDocument(ctx context.Context, doc *model.MemoryDocumentEntity) error {
	_ = ctx

	if doc == nil {
		return fmt.Errorf("memory document is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	docCopy := cloneMemoryDocument(doc)
	if docCopy.ID == uuid.Nil {
		docCopy.ID = uuid.New()
	}
	now := time.Now()
	if docCopy.CreatedAt.IsZero() {
		docCopy.CreatedAt = now
	}
	if docCopy.UpdatedAt.IsZero() {
		docCopy.UpdatedAt = now
	}

	s.memoryDocuments[docCopy.ID] = docCopy
	return nil
}

func (s *MemoryStore) BulkCreateMemoryChunks(ctx context.Context, chunks []model.MemoryChunkEntity) error {
	_ = ctx

	if len(chunks) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, chunk := range chunks {
		chunkCopy := cloneMemoryChunk(&chunk)
		if chunkCopy.ID == uuid.Nil {
			chunkCopy.ID = uuid.New()
		}
		if chunkCopy.CreatedAt.IsZero() {
			chunkCopy.CreatedAt = now
		}
		if chunkCopy.UpdatedAt.IsZero() {
			chunkCopy.UpdatedAt = now
		}
		s.memoryChunks[chunkCopy.ID] = chunkCopy
	}

	return nil
}

func (s *MemoryStore) BulkCreateMemoryEmbeddings(ctx context.Context, embeddings []model.MemoryEmbeddingEntity) error {
	_ = ctx

	if len(embeddings) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, emb := range embeddings {
		embCopy := cloneMemoryEmbedding(&emb)
		if embCopy.ChunkID == uuid.Nil {
			return fmt.Errorf("memory embedding chunk_id is required")
		}
		if embCopy.CreatedAt.IsZero() {
			embCopy.CreatedAt = now
		}
		s.memoryEmbeddings[embCopy.ChunkID] = embCopy
	}

	return nil
}

func (s *MemoryStore) VectorSearchMemoryChunks(
	ctx context.Context,
	userID uuid.UUID,
	conversationID uuid.UUID,
	projectID *uuid.UUID,
	scope rag.Scope,
	queryEmbedding []float32,
	limit int,
) ([]MemoryChunkSearchResult, error) {
	_ = ctx

	if len(queryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding is empty")
	}
	if limit <= 0 {
		limit = 8
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]MemoryChunkSearchResult, 0)

	for chunkID, chunk := range s.memoryChunks {
		doc, ok := s.memoryDocuments[chunk.DocumentID]
		if !ok {
			continue
		}
		emb, ok := s.memoryEmbeddings[chunkID]
		if !ok {
			continue
		}
		if chunk.UserID != userID || doc.UserID != userID {
			continue
		}
		if !doc.Active || doc.Status != model.MemoryDocIndexed {
			continue
		}
		if !memoryChunkMatchesScope(chunk, conversationID, projectID, scope) {
			continue
		}

		score := cosineSimilarity(queryEmbedding, emb.Embedding.Slice())
		results = append(results, MemoryChunkSearchResult{
			Chunk:        *cloneMemoryChunk(chunk),
			VectorScore:  score,
			KeywordScore: 0,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].VectorScore != results[j].VectorScore {
			return results[i].VectorScore > results[j].VectorScore
		}
		if results[i].Chunk.ImportanceScore != results[j].Chunk.ImportanceScore {
			return results[i].Chunk.ImportanceScore > results[j].Chunk.ImportanceScore
		}
		if results[i].Chunk.ChunkIndex != results[j].Chunk.ChunkIndex {
			return results[i].Chunk.ChunkIndex < results[j].Chunk.ChunkIndex
		}
		return results[i].Chunk.ID.String() < results[j].Chunk.ID.String()
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *MemoryStore) KeywordSearchMemoryChunks(
	ctx context.Context,
	userID uuid.UUID,
	conversationID uuid.UUID,
	projectID *uuid.UUID,
	scope rag.Scope,
	query string,
	ftsConfig string,
	limit int,
) ([]MemoryChunkSearchResult, error) {
	_ = ctx
	_ = ftsConfig

	query = strings.TrimSpace(query)
	if query == "" {
		return []MemoryChunkSearchResult{}, nil
	}
	if limit <= 0 {
		limit = 8
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	queryTokens := tokenizeForSearch(query)
	if len(queryTokens) == 0 {
		return []MemoryChunkSearchResult{}, nil
	}

	results := make([]MemoryChunkSearchResult, 0)

	for _, chunk := range s.memoryChunks {
		doc, ok := s.memoryDocuments[chunk.DocumentID]
		if !ok {
			continue
		}
		if chunk.UserID != userID || doc.UserID != userID {
			continue
		}
		if !doc.Active || doc.Status != model.MemoryDocIndexed {
			continue
		}
		if !memoryChunkMatchesScope(chunk, conversationID, projectID, scope) {
			continue
		}

		score := keywordScore(chunk.Content, queryTokens)
		if score <= 0 {
			continue
		}

		results = append(results, MemoryChunkSearchResult{
			Chunk:        *cloneMemoryChunk(chunk),
			VectorScore:  0,
			KeywordScore: score,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].KeywordScore != results[j].KeywordScore {
			return results[i].KeywordScore > results[j].KeywordScore
		}
		if results[i].Chunk.ImportanceScore != results[j].Chunk.ImportanceScore {
			return results[i].Chunk.ImportanceScore > results[j].Chunk.ImportanceScore
		}
		if results[i].Chunk.ChunkIndex != results[j].Chunk.ChunkIndex {
			return results[i].Chunk.ChunkIndex < results[j].Chunk.ChunkIndex
		}
		return results[i].Chunk.ID.String() < results[j].Chunk.ID.String()
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *MemoryStore) MarkMemoryDocumentIndexed(
	ctx context.Context,
	documentID uuid.UUID,
) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	doc, ok := s.memoryDocuments[documentID]
	if !ok {
		return fmt.Errorf("memory document not found: %s", documentID)
	}

	now := time.Now()
	doc.Status = model.MemoryDocIndexed
	doc.Active = true
	doc.IndexedAt = &now
	doc.FailedAt = nil
	doc.ErrorMsg = ""
	doc.UpdatedAt = now

	return nil
}

func (s *MemoryStore) MarkMemoryDocumentFailed(
	ctx context.Context,
	documentID uuid.UUID,
	errMsg string,
) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	doc, ok := s.memoryDocuments[documentID]
	if !ok {
		return fmt.Errorf("memory document not found: %s", documentID)
	}

	now := time.Now()
	doc.Status = model.MemoryDocFailed
	doc.Active = false
	doc.FailedAt = &now
	doc.ErrorMsg = errMsg
	doc.UpdatedAt = now

	return nil
}

func (s *MemoryStore) DeactivateActiveMemoryDocuments(
	ctx context.Context,
	conversationID uuid.UUID,
	sourceType model.MemorySourceType,
	excludeDocumentID uuid.UUID,
) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, doc := range s.memoryDocuments {
		if doc.ConversationID == nil {
			continue
		}
		if *doc.ConversationID != conversationID {
			continue
		}
		if doc.SourceType != sourceType {
			continue
		}
		if doc.ID == excludeDocumentID {
			continue
		}
		if doc.Active {
			doc.Active = false
			doc.UpdatedAt = time.Now()
		}
	}

	return nil
}

func (s *MemoryStore) UpdateConversationMemoryCheckpoint(
	ctx context.Context,
	conversationID uuid.UUID,
	checkpointMsgID uuid.UUID,
) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.conversations[conversationID.String()]
	if !ok {
		return fmt.Errorf("conversation not found: %s", conversationID)
	}

	now := time.Now()
	conv.MemoryCheckpointMsgID = cloneUUIDPtr(&checkpointMsgID)
	conv.SummaryUpdatedAt = &now
	conv.UpdatedAt = now

	return nil
}

func (s *MemoryStore) DeleteConversationMemorySnapshot(
	ctx context.Context,
	conversationID uuid.UUID,
	sourceType model.MemorySourceType,
) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	docIDs := make([]uuid.UUID, 0)
	for id, doc := range s.memoryDocuments {
		if doc.ConversationID == nil {
			continue
		}
		if *doc.ConversationID == conversationID && doc.SourceType == sourceType {
			docIDs = append(docIDs, id)
		}
	}

	if len(docIDs) == 0 {
		return nil
	}

	docIDSet := make(map[uuid.UUID]struct{}, len(docIDs))
	for _, id := range docIDs {
		docIDSet[id] = struct{}{}
		delete(s.memoryDocuments, id)
	}

	chunkIDs := make([]uuid.UUID, 0)
	for chunkID, chunk := range s.memoryChunks {
		if _, ok := docIDSet[chunk.DocumentID]; ok {
			chunkIDs = append(chunkIDs, chunkID)
			delete(s.memoryChunks, chunkID)
		}
	}

	for _, chunkID := range chunkIDs {
		delete(s.memoryEmbeddings, chunkID)
	}

	return nil
}

func (s *MemoryStore) GetAllMessagesByConvID(
	ctx context.Context,
	conversationID string,
) ([]model.Message, error) {
	return s.GetMessagesByConvID(ctx, conversationID, 0, "asc", "")
}

// --------------------
// Project stubs (not used in tests)
// --------------------

func (s *MemoryStore) CreateProject(_ context.Context, _ *model.Project) error {
	return fmt.Errorf("not implemented")
}

func (s *MemoryStore) GetProject(_ context.Context, _ uuid.UUID) (*model.Project, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *MemoryStore) ListProjectsByUserID(_ context.Context, _ uuid.UUID) ([]*model.Project, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *MemoryStore) UpdateProject(_ context.Context, _ *model.Project) error {
	return fmt.Errorf("not implemented")
}

func (s *MemoryStore) DeleteProject(_ context.Context, _ uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *MemoryStore) CountProjectConversations(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (s *MemoryStore) ListConversationsByProjectID(_ context.Context, _ uuid.UUID) ([]*model.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *MemoryStore) AssignConversationToProject(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

// --------------------
// Helpers
// --------------------

func extractDomainMsgs(in []memoryMsg) []model.Message {
	out := make([]model.Message, len(in))
	for i := range in {
		out[i] = cloneMessage(in[i].Msg)
		if in[i].Metadata != nil {
			out[i].Metadata = cloneMetadata(in[i].Metadata)
		}
	}
	return out
}

func reverseMemoryMsgs(in []memoryMsg) {
	for i, j := 0, len(in)-1; i < j; i, j = i+1, j-1 {
		in[i], in[j] = in[j], in[i]
	}
}

func cloneMetadata(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringPtr(in *string) *string {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func cloneUUIDPtr(in *uuid.UUID) *uuid.UUID {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func cloneTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func cloneMessage(in model.Message) model.Message {
	out := in
	out.Metadata = cloneMetadata(in.Metadata)
	out.FavoriteLabel = cloneStringPtr(in.FavoriteLabel)
	return out
}

func cloneAnnotation(in *model.Annotation) *model.Annotation {
	if in == nil {
		return nil
	}
	out := *in
	out.BgColor = cloneStringPtr(in.BgColor)
	out.TextColor = cloneStringPtr(in.TextColor)

	if in.IsBold != nil {
		v := *in.IsBold
		out.IsBold = &v
	}
	if in.IsUnderline != nil {
		v := *in.IsUnderline
		out.IsUnderline = &v
	}

	if in.ExtraAttrs != nil {
		out.ExtraAttrs = make(map[string]interface{}, len(in.ExtraAttrs))
		for k, v := range in.ExtraAttrs {
			out.ExtraAttrs[k] = v
		}
	}

	return &out
}

func cloneCommentThread(in *model.CommentThread) *model.CommentThread {
	if in == nil {
		return nil
	}
	out := *in
	if in.Comments != nil {
		out.Comments = make([]*model.Comment, len(in.Comments))
		for i, c := range in.Comments {
			out.Comments[i] = cloneComment(c)
		}
	}
	return &out
}

func cloneComment(in *model.Comment) *model.Comment {
	if in == nil {
		return nil
	}
	out := *in
	out.DeletedAt = cloneTimePtr(in.DeletedAt)
	return &out
}

func (s *MemoryStore) listCommentsByThreadIDLocked(threadID uuid.UUID, userID uuid.UUID) []*model.Comment {
	out := make([]*model.Comment, 0)
	for _, c := range s.comments {
		if c.ThreadID == threadID && c.UserID == userID {
			out = append(out, cloneComment(c))
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})

	return out
}

func cloneMemoryDocument(in *model.MemoryDocumentEntity) *model.MemoryDocumentEntity {
	if in == nil {
		return nil
	}
	out := *in
	out.ProjectID = cloneUUIDPtr(in.ProjectID)
	out.ConversationID = cloneUUIDPtr(in.ConversationID)
	out.SourceRefID = cloneUUIDPtr(in.SourceRefID)
	out.IndexedAt = cloneTimePtr(in.IndexedAt)
	out.FailedAt = cloneTimePtr(in.FailedAt)
	return &out
}

func cloneMemoryChunk(in *model.MemoryChunkEntity) *model.MemoryChunkEntity {
	if in == nil {
		return nil
	}
	out := *in
	out.ProjectID = cloneUUIDPtr(in.ProjectID)
	out.ConversationID = cloneUUIDPtr(in.ConversationID)
	return &out
}

func cloneMemoryEmbedding(in *model.MemoryEmbeddingEntity) *model.MemoryEmbeddingEntity {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func memoryChunkMatchesScope(
	chunk *model.MemoryChunkEntity,
	conversationID uuid.UUID,
	projectID *uuid.UUID,
	scope rag.Scope,
) bool {
	switch scope {
	case rag.ScopeConversationOnly:
		return chunk.ConversationID != nil && *chunk.ConversationID == conversationID

	case rag.ScopeConversationAndProject:
		if projectID == nil {
			return false
		}
		inConv := chunk.ConversationID != nil && *chunk.ConversationID == conversationID
		inProject := chunk.ProjectID != nil && *chunk.ProjectID == *projectID
		return inConv || inProject

	case rag.ScopeProjectOnly:
		if projectID == nil {
			return false
		}
		return chunk.ProjectID != nil && *chunk.ProjectID == *projectID

	default:
		return false
	}
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func tokenizeForSearch(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}

	replacer := strings.NewReplacer(
		",", " ",
		".", " ",
		";", " ",
		":", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		"\n", " ",
		"\t", " ",
		"\"", " ",
		"'", " ",
		"?", " ",
		"!", " ",
		"/", " ",
		"\\", " ",
		"|", " ",
	)
	s = replacer.Replace(s)

	parts := strings.Fields(s)
	if len(parts) == 0 {
		return nil
	}

	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func keywordScore(content string, queryTokens []string) float64 {
	if len(queryTokens) == 0 {
		return 0
	}

	lc := strings.ToLower(content)
	if lc == "" {
		return 0
	}

	var score float64
	for _, tok := range queryTokens {
		count := strings.Count(lc, tok)
		if count > 0 {
			score += float64(count)
		}
	}

	return score
}
