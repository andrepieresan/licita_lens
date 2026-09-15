package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"licitalens.dev/backend/internal/auth"
	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/httpapi"
	"licitalens.dev/backend/internal/providers/ai"
	"licitalens.dev/backend/internal/store"
)

func RunAPI() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	demo := env("DEMO_MODE", "true") == "true"
	if !demo {
		if err := validateProductionConfig(); err != nil {
			return err
		}
	}
	data, closeStore, err := dataStore(logger)
	if err != nil {
		return err
	}
	defer closeStore()
	var provider ai.Provider
	switch os.Getenv("AI_PROVIDER") {
	case "openai":
		provider = ai.NewOpenAI(os.Getenv("OPENAI_API_KEY"), os.Getenv("OPENAI_EMBEDDING_MODEL"), os.Getenv("OPENAI_TEXT_MODEL"))
	case "ollama":
		provider = ai.NewOllama(os.Getenv("OLLAMA_BASE_URL"), os.Getenv("OLLAMA_EMBEDDING_MODEL"), os.Getenv("OLLAMA_TEXT_MODEL"))
	}
	var authenticator auth.Authenticator
	localJWT := auth.NewLocalJWTFromEnv()
	if issuer := os.Getenv("KEYCLOAK_ISSUER"); issuer != "" {
		authenticator = auth.ChainAuthenticators(auth.NewKeycloak(issuer, os.Getenv("KEYCLOAK_AUDIENCE")), localJWT)
	} else {
		authenticator = localJWT
	}
	server := &http.Server{Addr: ":" + env("PORT", "8080"), Handler: httpapi.New(data, provider, authenticator, billing.NewStripeFromEnv(), localJWT, logger, demo), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 90 * time.Second}
	stop, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-stop.Done()
		ctx, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		_ = server.Shutdown(ctx)
	}()
	logger.Info("service started", "address", server.Addr)
	err = server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func dataStore(logger *slog.Logger) (store.DataStore, func(), error) {
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if database, err := store.NewPostgres(ctx, databaseURL); err == nil {
			logger.Info("using postgres persistence")
			return database, database.Close, nil
		}
		if env("DEMO_MODE", "true") != "true" {
			return nil, func() {}, fmt.Errorf("DATABASE_URL configurado, mas PostgreSQL não está disponível")
		}
		logger.Warn("postgres unavailable; falling back to memory demo store")
	}
	memory := store.NewMemory()
	if env("DEMO_MODE", "true") == "true" {
		store.SeedDemo(memory)
	}
	return memory, func() {}, nil
}

func validateProductionConfig() error {
	required := []string{"DATABASE_URL", "KEYCLOAK_ISSUER", "KEYCLOAK_AUDIENCE", "ADMIN_API_KEY", "ALLOWED_ORIGINS", "STRIPE_SECRET_KEY", "STRIPE_PRICE_ESSENTIAL", "STRIPE_PRICE_PRO", "STRIPE_WEBHOOK_SECRET", "STRIPE_SUCCESS_URL", "STRIPE_CANCEL_URL"}
	for _, key := range required {
		if env(key, "") == "" {
			return fmt.Errorf("configuração de produção incompleta: %s é obrigatório", key)
		}
	}
	return nil
}

func RunHealth(service string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"live","service":"` + service + `"}`))
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready","service":"` + service + `"}`))
	})
	return http.ListenAndServe(":"+env("PORT", "8080"), mux)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
