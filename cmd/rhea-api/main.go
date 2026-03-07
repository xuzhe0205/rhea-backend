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

	// Safe defaults for hosted Postgres / poolers, while still fine locally
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

	providerGemini, err := llm.NewGeminiProvider(ctx, cfg.GeminiAPIKey, cfg.GeminiModel)
	if err != nil {
		log.Fatal(err)
	}

	r := &router.Router{
		Claude: &llm.FakeProvider{Provider: llm.ProviderClaude, Reply: "Claude not implemented yet"},
		Gemini: providerGemini,
		OpenAI: &llm.FakeProvider{Provider: llm.ProviderOpenAI, Reply: "fallback not implemented yet"},
	}

	svc := &agent.Service{
		Store:   st,
		Builder: b,
		Router:  r,
	}

	authSvc := auth.NewService(st)
	authHandler := &httpapi.AuthHandler{AuthSvc: authSvc}

	s := httpapi.NewServer()

	s.Handle("GET /health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	// s.Handle("POST /v1/register", http.HandlerFunc(authHandler.Register))
	s.Handle("POST /v1/login", http.HandlerFunc(authHandler.Login))

	protectedChain := middleware.CreateChain(
		middleware.AuthMiddleware,
	)

	s.Handle("GET /v1/me", protectedChain(http.HandlerFunc(authHandler.GetMe)))

	chatHandler := &httpapi.ChatHandler{Agent: svc}
	s.Handle("POST /v1/chat", protectedChain(middleware.TokenUsageInterceptor(chatHandler)))

	streamHandler := &httpapi.ChatStreamHandler{Agent: svc}
	s.Handle("POST /v1/chat/stream", protectedChain(streamHandler))

	s.Handle("GET /v1/conversations", protectedChain(http.HandlerFunc(chatHandler.ListConversations)))
	s.Handle("GET /v1/conversations/{id}/messages", protectedChain(http.HandlerFunc(chatHandler.ListConversationMessages)))

	handlerWithCORS := middleware.CORS(s.Handler())

	addr := "0.0.0.0:" + port
	log.Printf("rhea-api listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, handlerWithCORS))
}
