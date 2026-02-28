package main

import (
	"context"
	"log"
	"net/http"
	"os"

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

	st := postgres.NewPostgresStore(postgresDB)

	systemPrompt := agent.GetDefaultSystemPrompt()

	b := &ctxbuilder.Builder{
		Store:         st,
		SystemPrompt:  systemPrompt,
		RecentMaxMsgs: 20,
	}
	ctx := context.Background()

	// For now, use FakeProviders so the server runs without API keys.
	// Next iteration, we wire real providers via config.
	// fpClaude := &llm.FakeProvider{Provider: llm.ProviderClaude, Reply: "claude:ok"}
	poviderGemini, err := llm.NewGeminiProvider(ctx, cfg.GeminiAPIKey, cfg.GeminiModel)
	// fpOpenAI := &llm.FakeProvider{Provider: llm.ProviderOpenAI, Reply: "openai:ok"}

	if err != nil {
		log.Fatal(err)
	}

	r := &router.Router{
		Claude: &llm.FakeProvider{Provider: llm.ProviderClaude, Reply: "Claude not implemented yet"},
		Gemini: poviderGemini,
		OpenAI: &llm.FakeProvider{Provider: llm.ProviderOpenAI, Reply: "fallback not implemented yet"},
	}

	svc := &agent.Service{
		Store:   st,
		Builder: b,
		Router:  r,
	}

	// Auth APIs
	authSvc := auth.NewService(st)
	authHandler := &httpapi.AuthHandler{AuthSvc: authSvc}

	s := httpapi.NewServer()
	s.Handle("POST /v1/register", http.HandlerFunc(authHandler.Register))
	s.Handle("POST /v1/login", http.HandlerFunc(authHandler.Login))

	// --- 定义安保链 ---
	// 这里可以包含多个中间件，比如 [Logging, Auth]
	protectedChain := middleware.CreateChain(
		middleware.AuthMiddleware,
	)

	// /me 接口
	s.Handle("GET /v1/me", protectedChain(http.HandlerFunc(authHandler.GetMe)))

	// Chat & Stream APIs
	// 1. /chat 接口
	// 先包装业务逻辑，再套上 TokenUsage，最后套上 Auth 保安
	chatHandler := &httpapi.ChatHandler{Agent: svc}
	// 注意：我们可以嵌套使用中间件
	s.Handle("POST /v1/chat", protectedChain(middleware.TokenUsageInterceptor(chatHandler)))
	// 2. /chat/stream 接口
	streamHandler := &httpapi.ChatStreamHandler{Agent: svc}
	s.Handle("POST /v1/chat/stream", protectedChain(streamHandler))
	// 3. /conversations 接口
	s.Handle("GET /v1/conversations", protectedChain(http.HandlerFunc(chatHandler.ListConversations)))
	// 4. /conversations/{id}/messages 接口
	s.Handle("GET /v1/conversations/{id}/messages", protectedChain(http.HandlerFunc(chatHandler.ListConversationMessages)))

	addr := ":" + port
	log.Printf("rhea-api listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, s.Handler()))
}
