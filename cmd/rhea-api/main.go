package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"rhea-backend/internal/agent"
	"rhea-backend/internal/auth"
	"rhea-backend/internal/config"
	ctxbuilder "rhea-backend/internal/context"
	"rhea-backend/internal/embedding"
	"rhea-backend/internal/httpapi"
	"rhea-backend/internal/httpapi/middleware"
	"rhea-backend/internal/ingest"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/retrieval"
	"rhea-backend/internal/router"
	"rhea-backend/internal/service"
	"rhea-backend/internal/storage"
	"rhea-backend/internal/store"
	"rhea-backend/internal/store/postgres"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	postgresDB, dbErr := store.InitDB(cfg.DBDSN)
	if dbErr != nil {
		log.Fatalf("Failed to initialize DB: %v", dbErr)
	}

	sqlDB, err := postgresDB.DB()
	if err != nil {
		log.Fatalf("Failed to get underlying sql.DB: %v", err)
	}
	defer sqlDB.Close()

	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	st := postgres.NewPostgresStore(postgresDB)

	systemPrompt := agent.GetDefaultSystemPrompt()

	ctx := context.Background()

	// --- 🚀 RHEA 多模型分层初始化 ---

	// 1. Pro: 深度导师 (高智商，给 0.8 的温度保持一点启发性)
	pPro, err := llm.NewGeminiProvider(ctx, cfg.GeminiAPIKey, cfg.ModelPro, 0.8)
	if err != nil {
		log.Fatalf("Failed to init Gemini Pro: %v", err)
	}

	// 2. Flash: 快速助手 (平衡性能，给 0.7)
	pFlash, err := llm.NewGeminiProvider(ctx, cfg.GeminiAPIKey, cfg.ModelFlash, 0.7)
	if err != nil {
		log.Fatalf("Failed to init Gemini Flash: %v", err)
	}

	// 2.5 Flash: 默认优先选择Free Tier的API
	pFlashFree, err := llm.NewGeminiProvider(ctx, cfg.GeminiAPIKeyFree, cfg.ModelFlash, 0.7)
	if err != nil {
		log.Fatalf("Failed to init Gemini Flash: %v", err)
	}

	// 3. Lite: 极速分类器 (只需 0.1 温度确保分类结果稳定)
	pLite, err := llm.NewGeminiProvider(ctx, cfg.GeminiAPIKey, cfg.ModelLite, 0.1)
	if err != nil {
		log.Fatalf("Failed to init Gemini Lite: %v", err)
	}

	embedProvider, err := embedding.NewGeminiEmbeddingProvider(
		ctx,
		cfg.GeminiAPIKey,
		cfg.GeminiEmbeddingModel,
	)
	if err != nil {
		log.Fatalf("Failed to init Gemini embedding provider: %v", err)
	}

	embedSvc := &embedding.Service{
		Provider: embedProvider,
	}

	retrievalSvc := &retrieval.Service{
		Store:      st,
		Embeddings: embedSvc,
		Policy: retrieval.Policy{
			TopK:             8,
			MinFinalScore:    0.35,
			RequireAnySignal: true,
		},
	}

	ingestor := &ingest.ConversationIngestor{
		Store:      st,
		Embeddings: embedSvc,
	}

	builder := &ctxbuilder.Builder{
		Store:         st,
		Retrieval:     retrievalSvc,
		SystemPrompt:  systemPrompt,
		RecentMaxMsgs: 10,
		RetrievalTopK: 8,
	}

	// 装配智能路由器
	r := &router.Router{
		GeminiPro:       pPro,
		GeminiFlash:     pFlash,
		GeminiFlashFree: pFlashFree,
		GeminiLite:      pLite,
	}

	// --- 🚀 初始化核心 Service ---

	svc := &agent.Service{
		Store:    st,
		Builder:  builder,
		Router:   r,
		Ingestor: ingestor,
	}

	authSvc := auth.NewService(st)
	authHandler := &httpapi.AuthHandler{AuthSvc: authSvc}

	s := httpapi.NewServer()

	// 1. 初始化 Annotation 业务服务
	annSvc := service.NewAnnotationService(st)

	// 2. 初始化 Annotation HTTP 处理器
	annotationHandler := &httpapi.AnnotationHandler{AnnSvc: annSvc}

	commentSvc := service.NewCommentService(st)
	commentHandler := &httpapi.CommentHandler{CommentSvc: commentSvc}

	projectSvc := service.NewProjectService(st)
	projectHandler := &httpapi.ProjectHandler{ProjectSvc: projectSvc}

	r2 := storage.NewR2Client(cfg.R2AccountID, cfg.R2AccessKeyID, cfg.R2SecretAccessKey, cfg.R2Bucket)
	uploadHandler := &httpapi.UploadHandler{R2: r2}

	// 健康检查
	s.Handle("GET /health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	// 身份验证
	s.Handle("POST /v1/login", http.HandlerFunc(authHandler.Login))
	// s.Handle("POST /v1/register", http.HandlerFunc(authHandler.Register))

	protectedChain := middleware.CreateChain(
		middleware.AuthMiddleware,
	)

	s.Handle("GET /v1/me", protectedChain(http.HandlerFunc(authHandler.GetMe)))

	// 聊天相关
	chatHandler := &httpapi.ChatHandler{Agent: svc, R2: r2}
	s.Handle("POST /v1/chat", protectedChain(middleware.TokenUsageInterceptor(chatHandler)))

	streamHandler := &httpapi.ChatStreamHandler{Agent: svc}
	s.Handle("POST /v1/chat/stream", protectedChain(streamHandler))

	s.Handle("PATCH /v1/messages/{id}/favorite", protectedChain(http.HandlerFunc(chatHandler.PatchMessageFavorite)))
	s.Handle("GET /v1/messages/favorites", protectedChain(http.HandlerFunc(chatHandler.ListFavoriteMessages)))
	s.Handle("GET /v1/conversations/{id}/favorites/{messageId}/messages", protectedChain(http.HandlerFunc(chatHandler.ListMessagesForFavoriteJump)))
	s.Handle("PATCH /v1/messages/{id}/favorite-label", protectedChain(http.HandlerFunc(chatHandler.PatchMessageFavoriteLabel)))

	s.Handle("GET /v1/conversations", protectedChain(http.HandlerFunc(chatHandler.ListConversations)))
	s.Handle("GET /v1/conversations/{id}/messages", protectedChain(http.HandlerFunc(chatHandler.ListConversationMessages)))
	s.Handle("GET /v1/conversations/{id}", protectedChain(http.HandlerFunc(chatHandler.GetConversation)))
	s.Handle("PATCH /v1/conversations/{id}/pin", protectedChain(http.HandlerFunc(chatHandler.PatchConversationPin)))
	s.Handle("GET /v1/conversations/pinned", protectedChain(http.HandlerFunc(chatHandler.ListPinnedConversations)))

	s.Handle("POST /v1/annotations", protectedChain(http.HandlerFunc(annotationHandler.Annotate)))
	s.Handle("GET /v1/messages/{id}/annotations", protectedChain(http.HandlerFunc(annotationHandler.ListByMessage)))
	s.Handle("GET /v1/conversations/{id}/annotations", protectedChain(http.HandlerFunc(annotationHandler.ListByConversation)))
	s.Handle("DELETE /v1/annotations/{id}", protectedChain(http.HandlerFunc(annotationHandler.Delete)))
	s.Handle("POST /v1/annotations/remove-highlight", protectedChain(http.HandlerFunc(annotationHandler.RemoveHighlightRange)))

	s.Handle("GET /v1/comments/thread", protectedChain(http.HandlerFunc(commentHandler.GetCommentThread)))
	s.Handle("POST /v1/comments", protectedChain(http.HandlerFunc(commentHandler.AddComment)))
	s.Handle("GET /v1/comments/{id}", protectedChain(http.HandlerFunc(commentHandler.GetComment)))
	s.Handle("DELETE /v1/comments/{id}", protectedChain(http.HandlerFunc(commentHandler.DeleteComment)))
	s.Handle("GET /v1/comment-threads", protectedChain(http.HandlerFunc(commentHandler.ListByMessageIDs)))

	// Uploads
	s.Handle("POST /v1/uploads/image", protectedChain(http.HandlerFunc(uploadHandler.UploadImage)))
	s.Handle("DELETE /v1/uploads/image", protectedChain(http.HandlerFunc(uploadHandler.DeleteImage)))

	// Transcription
	transcribeHandler := &httpapi.TranscribeHandler{}
	s.Handle("POST /v1/transcribe", protectedChain(http.HandlerFunc(transcribeHandler.Transcribe)))

	// Share links
	shareHandler := &httpapi.ShareHandler{Store: st, R2: r2}
	s.Handle("POST /v1/share", protectedChain(http.HandlerFunc(shareHandler.CreateShareLink)))
	s.Handle("GET /v1/share/{token}", http.HandlerFunc(shareHandler.GetSharedContent)) // public

	// Projects
	s.Handle("GET /v1/projects", protectedChain(http.HandlerFunc(projectHandler.ListProjects)))
	s.Handle("POST /v1/projects", protectedChain(http.HandlerFunc(projectHandler.CreateProject)))
	s.Handle("GET /v1/projects/{id}", protectedChain(http.HandlerFunc(projectHandler.GetProject)))
	s.Handle("PATCH /v1/projects/{id}", protectedChain(http.HandlerFunc(projectHandler.UpdateProject)))
	s.Handle("DELETE /v1/projects/{id}", protectedChain(http.HandlerFunc(projectHandler.DeleteProject)))
	s.Handle("GET /v1/projects/{id}/conversations", protectedChain(http.HandlerFunc(projectHandler.ListProjectConversations)))
	s.Handle("POST /v1/projects/{id}/conversations", protectedChain(http.HandlerFunc(projectHandler.CreateProjectConversation)))

	handlerWithCORS := middleware.CORS(s.Handler())

	addr := "0.0.0.0:" + port
	log.Printf("rhea-api listening on %s (Intelligent Routing Enabled ⚡)", addr)
	log.Fatal(http.ListenAndServe(addr, handlerWithCORS))
}
