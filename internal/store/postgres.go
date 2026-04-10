package store

import (
	"log"
	"rhea-backend/internal/model" // 确保路径正确

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
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
