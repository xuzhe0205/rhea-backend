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
	"rhea-backend/internal/httpapi"
	"rhea-backend/internal/httpapi/middleware"
	"rhea-backend/internal/llm"
	"rhea-backend/internal/router"
	"rhea-backend/internal/service"
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

	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  systemPrompt,
		RecentMaxMsgs: 20,
	}
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

	// 装配智能路由器
	r := &router.Router{
		GeminiPro:       pPro,
		GeminiFlash:     pFlash,
		GeminiFlashFree: pFlashFree,
		GeminiLite:      pLite,
	}

	// --- 🚀 初始化核心 Service ---

	svc := &agent.Service{
		Store:   st,
		Builder: b,
		Router:  r,
	}

	authSvc := auth.NewService(st)
	authHandler := &httpapi.AuthHandler{AuthSvc: authSvc}

	s := httpapi.NewServer()

	// 1. 初始化 Annotation 业务服务
	annSvc := service.NewAnnotationService(st)

	// 2. 初始化 Annotation HTTP 处理器
	annotationHandler := &httpapi.AnnotationHandler{AnnSvc: annSvc}

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
	chatHandler := &httpapi.ChatHandler{Agent: svc}
	s.Handle("POST /v1/chat", protectedChain(middleware.TokenUsageInterceptor(chatHandler)))

	streamHandler := &httpapi.ChatStreamHandler{Agent: svc}
	s.Handle("POST /v1/chat/stream", protectedChain(streamHandler))

	s.Handle("GET /v1/conversations", protectedChain(http.HandlerFunc(chatHandler.ListConversations)))
	s.Handle("GET /v1/conversations/{id}/messages", protectedChain(http.HandlerFunc(chatHandler.ListConversationMessages)))
	s.Handle("GET /v1/conversations/{id}/token-sum", protectedChain(http.HandlerFunc(chatHandler.GetConversationTokenSum)))

	s.Handle("POST /v1/annotations", protectedChain(http.HandlerFunc(annotationHandler.Annotate)))
	s.Handle("GET /v1/messages/{id}/annotations", protectedChain(http.HandlerFunc(annotationHandler.ListByMessage)))
	s.Handle("DELETE /v1/annotations/{id}", protectedChain(http.HandlerFunc(annotationHandler.Delete)))

	handlerWithCORS := middleware.CORS(s.Handler())

	addr := "0.0.0.0:" + port
	log.Printf("rhea-api listening on %s (Intelligent Routing Enabled ⚡)", addr)
	log.Fatal(http.ListenAndServe(addr, handlerWithCORS))
}
