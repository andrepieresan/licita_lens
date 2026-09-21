package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/store"
)

// RunBillingReconciler periodically repairs missed Stripe webhook deliveries.
// It is intentionally cloud-only: self-hosted installations do not require a
// payment provider to keep the product available.
func RunBillingReconciler() error {
	mode, err := deploymentMode()
	if err != nil {
		return err
	}
	if mode != "cloud" {
		return errors.New("billing reconciler is available only in cloud mode")
	}
	if err := validateDeploymentConfig(mode); err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	data, closeStore, err := dataStore(logger, false)
	if err != nil {
		return err
	}
	defer closeStore()
	stripe := billing.NewStripeFromEnv()
	if !stripe.Enabled() {
		return errors.New("Stripe is not configured")
	}
	interval := envDuration("STRIPE_RECONCILE_INTERVAL", time.Hour)
	var completed atomic.Bool
	var lastSuccess, lastFailure atomic.Int64
	var processed, failures atomic.Uint64
	healthChecks := []HealthCheck{{Name: "reconciliation", Check: func(context.Context) error {
		if !completed.Load() {
			return errors.New("Stripe reconciliation pending or failed")
		}
		return nil
	}}}
	if pinger, ok := data.(store.Pinger); ok {
		healthChecks = append(healthChecks, HealthCheck{Name: "postgres", Check: pinger.Ping})
	}
	go func() {
		metrics := func(w io.Writer) {
			fmt.Fprintln(w, "# HELP licitalens_billing_reconcile_last_success_timestamp_seconds Unix timestamp of the last successful Stripe reconciliation.")
			fmt.Fprintln(w, "# TYPE licitalens_billing_reconcile_last_success_timestamp_seconds gauge")
			fmt.Fprintln(w, "licitalens_billing_reconcile_last_success_timestamp_seconds", lastSuccess.Load())
			fmt.Fprintln(w, "# HELP licitalens_billing_reconcile_last_failure_timestamp_seconds Unix timestamp of the last failed Stripe reconciliation.")
			fmt.Fprintln(w, "# TYPE licitalens_billing_reconcile_last_failure_timestamp_seconds gauge")
			fmt.Fprintln(w, "licitalens_billing_reconcile_last_failure_timestamp_seconds", lastFailure.Load())
			fmt.Fprintln(w, "# HELP licitalens_billing_reconcile_processed_total Subscription states applied by the reconciler.")
			fmt.Fprintln(w, "# TYPE licitalens_billing_reconcile_processed_total counter")
			fmt.Fprintln(w, "licitalens_billing_reconcile_processed_total", processed.Load())
			fmt.Fprintln(w, "# HELP licitalens_billing_reconcile_failures_total Failed reconciliation cycles.")
			fmt.Fprintln(w, "# TYPE licitalens_billing_reconcile_failures_total counter")
			fmt.Fprintln(w, "licitalens_billing_reconcile_failures_total", failures.Load())
		}
		if err := RunHealthWithMetrics("billing", healthChecks, metrics); err != nil {
			logger.Error("health server stopped", "error", err)
		}
	}()
	run := func() {
		items, listErr := stripe.ListSubscriptions(context.Background())
		if listErr != nil {
			completed.Store(false)
			failures.Add(1)
			lastFailure.Store(time.Now().UTC().Unix())
			logger.Error("Stripe reconciliation failed", "error", listErr)
			return
		}
		for _, item := range items {
			changed, applyErr := data.ApplySubscription(context.Background(), item.OrganizationID, item.Subscription, item.EventID, fmtHash(item.EventID))
			if applyErr != nil {
				completed.Store(false)
				failures.Add(1)
				lastFailure.Store(time.Now().UTC().Unix())
				logger.Error("persist Stripe reconciliation", "organization_id", item.OrganizationID, "error", applyErr)
				return
			}
			if changed {
				processed.Add(1)
			}
		}
		completed.Store(true)
		lastSuccess.Store(time.Now().UTC().Unix())
		logger.Info("Stripe reconciliation completed", "checked", len(items))
	}
	run()
	if os.Getenv("STRIPE_RECONCILE_ONCE") == "true" {
		return nil
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		run()
	}
	return nil
}

func fmtHash(value string) string {
	return "reconcile:" + value
}
