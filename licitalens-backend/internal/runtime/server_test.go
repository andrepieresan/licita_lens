package runtime

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeploymentModeRejectsContradictoryLegacyDemoFlag(t *testing.T) {
	t.Setenv("DEPLOYMENT_MODE", "self_hosted")
	t.Setenv("DEMO_MODE", "true")
	if _, err := deploymentMode(); err == nil {
		t.Fatal("expected contradictory deployment configuration to fail")
	}
}

func TestSelfHostedRequiresSecureCoreSecretsButNotStripe(t *testing.T) {
	t.Setenv("DEPLOYMENT_MODE", "self_hosted")
	t.Setenv("DEMO_MODE", "false")
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("AUTH_JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("ADMIN_API_KEY", "0123456789abcdef01234567")
	t.Setenv("ALLOWED_ORIGINS", "https://app.example.com")
	if err := validateDeploymentConfig("self_hosted"); err != nil {
		t.Fatalf("self hosted should not require Stripe: %v", err)
	}
}

func TestCloudRequiresStripe(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("AUTH_JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("ADMIN_API_KEY", "0123456789abcdef01234567")
	t.Setenv("ALLOWED_ORIGINS", "https://app.example.com")
	if err := validateDeploymentConfig("cloud"); err == nil {
		t.Fatal("expected cloud configuration without Stripe to fail")
	}
}

func TestDeploymentRejectsExampleSecrets(t *testing.T) {
	t.Setenv("DEPLOYMENT_MODE", "self_hosted")
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("AUTH_JWT_SECRET", "replace-with-at-least-32-random-characters")
	t.Setenv("ADMIN_API_KEY", "0123456789abcdef01234567")
	t.Setenv("ALLOWED_ORIGINS", "https://app.example.com")
	if err := validateDeploymentConfig("self_hosted"); err == nil {
		t.Fatal("example secret must be rejected")
	}
}

func TestDeploymentRejectsUnderscorePlaceholders(t *testing.T) {
	t.Setenv("DEPLOYMENT_MODE", "self_hosted")
	t.Setenv("DEMO_MODE", "false")
	t.Setenv("DATABASE_URL", "postgres://licitalens:CHANGE_ME@postgres/licitalens")
	t.Setenv("AUTH_JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("ADMIN_API_KEY", "0123456789abcdef01234567")
	t.Setenv("ALLOWED_ORIGINS", "https://app.example.com")
	if err := validateDeploymentConfig("self_hosted"); err == nil {
		t.Fatal("underscore placeholder must be rejected")
	}
}

func TestCloudRejectsSMTPWithoutRequiredTLS(t *testing.T) {
	for key, value := range map[string]string{
		"DATABASE_URL": "postgres://example", "AUTH_JWT_SECRET": "0123456789abcdef0123456789abcdef",
		"ADMIN_API_KEY": "0123456789abcdef01234567", "ALLOWED_ORIGINS": "https://app.example.com",
		"STRIPE_SECRET_KEY": "sk_test", "STRIPE_PRICE_ESSENTIAL": "price_essential", "STRIPE_PRICE_PRO": "price_pro",
		"STRIPE_WEBHOOK_SECRET": "whsec_test", "STRIPE_SUCCESS_URL": "https://app.example.com/success",
		"STRIPE_CANCEL_URL": "https://app.example.com/cancel", "SMTP_HOST": "smtp.example.com",
		"SMTP_FROM": "alerts@example.com", "SMTP_TLS_MODE": "disabled", "APP_BASE_URL": "https://app.example.com",
	} {
		t.Setenv(key, value)
	}
	if err := validateDeploymentConfig("cloud"); err == nil {
		t.Fatal("cloud SMTP must require TLS")
	}
}

func TestHealthHandlerReportsFailedDependency(t *testing.T) {
	handler := HealthHandler("worker", HealthCheck{Name: "postgres", Check: func(context.Context) error {
		return errors.New("down")
	}})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != http.StatusServiceUnavailable || response.Body.String() == "" {
		t.Fatalf("unexpected readiness response: %d %s", response.Code, response.Body.String())
	}
}

func TestHealthHandlerExposesWorkerMetrics(t *testing.T) {
	handler := healthHandler("worker", nil, func(w io.Writer) { _, _ = io.WriteString(w, "example_worker_metric 1\n") })
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK || response.Body.String() != "example_worker_metric 1\n" {
		t.Fatalf("unexpected metrics response: %d %q", response.Code, response.Body.String())
	}
}

func TestBillingReconcilerRejectsNonCloudMode(t *testing.T) {
	t.Setenv("DEPLOYMENT_MODE", "self_hosted")
	t.Setenv("DEMO_MODE", "false")
	if err := RunBillingReconciler(); err == nil {
		t.Fatal("billing reconciler must not start outside cloud mode")
	}
}
