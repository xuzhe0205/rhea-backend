package auth

import (
	"context"

	"github.com/google/uuid"
)

// 使用私有类型作为 key，防止其他包意外覆盖 context 里的值
type contextKey string

const userIDKey contextKey = "rhea_user_id"

// SetUserID 将 UserID 存入 context
func SetUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// GetUserID 从 context 中取出 UserID
func GetUserID(ctx context.Context) (uuid.UUID, bool) {
	uid, ok := ctx.Value(userIDKey).(uuid.UUID)
	return uid, ok
}
