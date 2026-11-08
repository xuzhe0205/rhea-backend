package store

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"rhea-backend/internal/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitDB opens a GORM connection and runs AutoMigrate.
// It retries up to 5 times with exponential back-off to survive the brief DNS
// unavailability window that occurs when a Render free-tier instance cold-starts.
func InitDB(dsn string) (*gorm.DB, error) {
	const maxAttempts = 5
	var db *gorm.DB
	var err error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Error),
		})
		if err == nil {
			break
		}
		if attempt == maxAttempts {
			return nil, fmt.Errorf("failed to connect after %d attempts: %w", maxAttempts, err)
		}
		wait := time.Duration(attempt*attempt) * time.Second // 1s, 4s, 9s, 16s
		log.Printf("DB connection attempt %d/%d failed: %v — retrying in %s", attempt, maxAttempts, err, wait)
		time.Sleep(wait)
	}

	// 自动迁移：GORM 会根据你的 struct 自动建表或加字段
	err = db.AutoMigrate(
		&model.UserEntity{},
		&model.ProjectEntity{},
		&model.ConversationEntity{},
		&model.MessageEntity{},
		&model.AnnotationEntity{},
		&model.CommentEntity{},
		&model.CommentThreadEntity{},
		&model.MemoryChunkEntity{},
		&model.MemoryDocumentEntity{},
		&model.MemoryEmbeddingEntity{},
		&model.ShareLinkEntity{},
	)
	if err != nil {
		return nil, err
	}

	log.Println("✅ Database migration completed!")

	enableRLS(db)
	return db, nil
}

// enableRLS turns on row level security for every table in the public schema.
//
// Supabase exposes the public schema through its REST API (PostgREST) to the
// anon/authenticated roles. RLS with no policies denies those roles everything.
// The backend connects as the table owner, which bypasses RLS, so app queries
// are unaffected. Failures are logged, not fatal: ALTER TABLE requires
// ownership, and a non-owner connection can't be locked out by this anyway.
func enableRLS(db *gorm.DB) {
	var tables []string
	if err := db.Raw(
		`SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND NOT rowsecurity`,
	).Scan(&tables).Error; err != nil {
		log.Printf("⚠️ RLS: could not list tables: %v", err)
		return
	}

	for _, t := range tables {
		stmt := fmt.Sprintf(`ALTER TABLE public.%s ENABLE ROW LEVEL SECURITY`, quoteIdent(t))
		if err := db.Exec(stmt).Error; err != nil {
			log.Printf("⚠️ RLS: could not enable on public.%s: %v", t, err)
			continue
		}
		log.Printf("🔒 RLS enabled on public.%s", t)
	}
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// Ping runs a real table read so Supabase counts it as project activity
// (a bare connection check may not).
func Ping(ctx context.Context, db *gorm.DB) error {
	var n int
	return db.WithContext(ctx).Raw(`SELECT 1 FROM user_entities LIMIT 1`).Scan(&n).Error
}
