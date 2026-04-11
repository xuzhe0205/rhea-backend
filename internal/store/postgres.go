package store

import (
	"fmt"
	"log"
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
	return db, nil
}
