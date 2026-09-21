package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	mode, err := deploymentMode()
	if err != nil {
		return err
	}
	demo := mode == "demo"
	if err := validateDeploymentConfig(mode); err != nil {
		return err
	}
	data, closeStore, err := dataStore(logger, demo)
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
	server := &http.Server{Addr: ":" + env("PORT", "8080"), Handler: httpapi.New(data, provider, authenticator, billing.NewStripeFromEnv(), localJWT, logger, mode), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 90 * time.Second}
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

func dataStore(logger *slog.Logger, demo bool) (store.DataStore, func(), error) {
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if database, err := store.NewPostgres(ctx, databaseURL); err == nil {
			logger.Info("using postgres persistence")
			return database, database.Close, nil
		}
		if !demo {
			return nil, func() {}, fmt.Errorf("DATABASE_URL configurado, mas PostgreSQL não está disponível")
		}
		logger.Warn("postgres unavailable; falling back to memory demo store")
	}
	memory := store.NewMemory()
	if demo {
		store.SeedDemo(memory)
	}
	return memory, func() {}, nil
}

func deploymentMode() (string, error) {
	mode := env("DEPLOYMENT_MODE", "")
	if mode == "" {
		if env("DEMO_MODE", "true") == "true" {
			return "demo", nil
		}
		return "cloud", nil
	}
	if mode != "demo" && mode != "self_hosted" && mode != "cloud" {
		return "", fmt.Errorf("DEPLOYMENT_MODE inválido: use demo, self_hosted ou cloud")
	}
	if legacy := os.Getenv("DEMO_MODE"); legacy != "" && (legacy == "true") != (mode == "demo") {
		return "", fmt.Errorf("DEPLOYMENT_MODE e DEMO_MODE são contraditórios")
	}
	return mode, nil
}

func validateDeploymentConfig(mode string) error {
	if mode == "demo" {
		return nil
	}
	required := []string{"DATABASE_URL", "AUTH_JWT_SECRET", "ADMIN_API_KEY", "ALLOWED_ORIGINS"}
	if mode == "cloud" {
		required = append(required, "STRIPE_SECRET_KEY", "STRIPE_PRICE_ESSENTIAL", "STRIPE_PRICE_PRO", "STRIPE_WEBHOOK_SECRET", "STRIPE_SUCCESS_URL", "STRIPE_CANCEL_URL", "SMTP_HOST", "SMTP_FROM", "SMTP_TLS_MODE", "APP_BASE_URL")
	}
	for _, key := range required {
		if env(key, "") == "" {
			return fmt.Errorf("configuração %s incompleta: %s é obrigatório", mode, key)
		}
	}
	if unsafeDatabasePlaceholder(env("DATABASE_URL", "")) {
		return fmt.Errorf("DATABASE_URL contém um valor de exemplo e não pode ser usado fora do modo demo")
	}
	if secret := env("AUTH_JWT_SECRET", ""); !secureSecret(secret, 32) || secret == "licitalens-local-dev-secret-change-me" {
		return fmt.Errorf("AUTH_JWT_SECRET deve ter ao menos 32 caracteres aleatórios fora do modo demo")
	}
	if key := env("ADMIN_API_KEY", ""); !secureSecret(key, 24) {
		return fmt.Errorf("ADMIN_API_KEY deve ter ao menos 24 caracteres aleatórios fora do modo demo")
	}
	if host := env("SMTP_HOST", ""); host != "" {
		tlsMode := strings.ToLower(env("SMTP_TLS_MODE", "auto"))
		if tlsMode != "auto" && tlsMode != "starttls" && tlsMode != "implicit" && tlsMode != "disabled" {
			return fmt.Errorf("SMTP_TLS_MODE inválido: use auto, starttls, implicit ou disabled")
		}
		if mode == "cloud" && tlsMode != "starttls" && tlsMode != "implicit" {
			return fmt.Errorf("SMTP_TLS_MODE deve exigir starttls ou implicit no modo cloud")
		}
		if (env("SMTP_USER", "") == "") != (env("SMTP_PASSWORD", "") == "") {
			return fmt.Errorf("SMTP_USER e SMTP_PASSWORD devem ser configurados juntos")
		}
		if value := env("SMTP_TIMEOUT", ""); value != "" {
			if timeout, err := time.ParseDuration(value); err != nil || timeout <= 0 {
				return fmt.Errorf("SMTP_TIMEOUT deve ser uma duração positiva")
			}
		}
	}
	return nil
}

func secureSecret(value string, minimum int) bool {
	value = strings.TrimSpace(value)
	if len(value) < minimum {
		return false
	}
	return !unsafePlaceholder(value)
}

func unsafePlaceholder(value string) bool {
	lower := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	return strings.Contains(lower, "replace-with") || strings.Contains(lower, "change-me") || strings.Contains(lower, "changeme") || strings.Contains(lower, "example")
}

func unsafeDatabasePlaceholder(value string) bool {
	lower := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	return strings.Contains(lower, "replace-with") || strings.Contains(lower, "change-me") || strings.Contains(lower, "changeme")
}

type HealthCheck struct {
	Name  string
	Check func(context.Context) error
}

// MetricsWriter appends Prometheus text exposition for a worker. It must not
// block on remote dependencies; health checks already cover those separately.
type MetricsWriter func(io.Writer)

func HealthHandler(service string, checks ...HealthCheck) http.Handler {
	return healthHandler(service, checks, nil)
}

func healthHandler(service string, checks []HealthCheck, metrics MetricsWriter) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "live", "service": service})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		status, code := "ready", http.StatusOK
		results := map[string]string{}
		for _, check := range checks {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			err := check.Check(ctx)
			cancel()
			if err != nil {
				results[check.Name] = "unavailable"
				status, code = "degraded", http.StatusServiceUnavailable
			} else {
				results[check.Name] = "ok"
			}
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "service": service, "checks": results})
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		if metrics != nil {
			metrics(w)
		}
	})
	return mux
}

func RunHealth(service string, checks ...HealthCheck) error {
	server := &http.Server{
		Addr:              ":" + env("PORT", "8080"),
		Handler:           HealthHandler(service, checks...),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	return server.ListenAndServe()
}

func RunHealthWithMetrics(service string, checks []HealthCheck, metrics MetricsWriter) error {
	server := &http.Server{
		Addr:              ":" + env("PORT", "8080"),
		Handler:           healthHandler(service, checks, metrics),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	return server.ListenAndServe()
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
