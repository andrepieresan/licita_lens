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

	"licitalens.dev/backend/internal/notifications"
	"licitalens.dev/backend/internal/store"
)

func RunNotificationWorker() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mode, err := deploymentMode()
	if err != nil {
		return err
	}
	if err := validateDeploymentConfig(mode); err != nil {
		return err
	}
	data, closeStore, err := dataStore(logger, mode == "demo")
	if err != nil {
		return err
	}
	defer closeStore()
	worker := notifications.NewWorker(data, logger, notifications.WorkerConfig{
		DemoMode:  mode == "demo",
		CloudMode: mode == "cloud",
		SMTP: notifications.SMTPConfig{
			Host:     env("SMTP_HOST", "localhost"),
			Port:     env("SMTP_PORT", "1025"),
			From:     env("SMTP_FROM", "alerts@licitalens.local"),
			User:     env("SMTP_USER", ""),
			Password: env("SMTP_PASSWORD", ""),
			TLSMode:  env("SMTP_TLS_MODE", "auto"),
			Timeout:  envDuration("SMTP_TIMEOUT", 15*time.Second),
		},
	})
	interval := envDuration("NOTIFICATIONS_INTERVAL", time.Minute)
	if once := os.Getenv("NOTIFICATIONS_ONCE"); once == "true" {
		result, err := worker.Run(context.Background())
		if err != nil {
			return err
		}
		logger.Info("notification cycle finished", "result", result)
		return nil
	}
	var completedCycle atomic.Bool
	var lastSuccess, lastFailure atomic.Int64
	var cycles, failures, suppressed, opportunitySent, followUpSent atomic.Uint64
	healthChecks := []HealthCheck{{Name: "notification_cycle", Check: func(context.Context) error {
		if !completedCycle.Load() {
			return errors.New("notification cycle pending or failed")
		}
		return nil
	}}}
	if pinger, ok := data.(store.Pinger); ok {
		healthChecks = append(healthChecks, HealthCheck{Name: "postgres", Check: pinger.Ping})
	}
	go func() {
		metrics := func(w io.Writer) {
			fmt.Fprintln(w, "# HELP licitalens_notifications_last_success_timestamp_seconds Unix timestamp of the last error-free notification cycle.")
			fmt.Fprintln(w, "# TYPE licitalens_notifications_last_success_timestamp_seconds gauge")
			fmt.Fprintln(w, "licitalens_notifications_last_success_timestamp_seconds", lastSuccess.Load())
			fmt.Fprintln(w, "# HELP licitalens_notifications_last_failure_timestamp_seconds Unix timestamp of the most recent notification cycle failure.")
			fmt.Fprintln(w, "# TYPE licitalens_notifications_last_failure_timestamp_seconds gauge")
			fmt.Fprintln(w, "licitalens_notifications_last_failure_timestamp_seconds", lastFailure.Load())
			fmt.Fprintln(w, "# HELP licitalens_notifications_cycles_total Notification cycles executed by this process.")
			fmt.Fprintln(w, "# TYPE licitalens_notifications_cycles_total counter")
			fmt.Fprintln(w, "licitalens_notifications_cycles_total", cycles.Load())
			fmt.Fprintln(w, "# HELP licitalens_notifications_failures_total Delivery and cycle failures observed by this process.")
			fmt.Fprintln(w, "# TYPE licitalens_notifications_failures_total counter")
			fmt.Fprintln(w, "licitalens_notifications_failures_total", failures.Load())
			fmt.Fprintln(w, "# HELP licitalens_notifications_suppressed_total Deliveries suppressed after repeated failures by this process.")
			fmt.Fprintln(w, "# TYPE licitalens_notifications_suppressed_total counter")
			fmt.Fprintln(w, "licitalens_notifications_suppressed_total", suppressed.Load())
			fmt.Fprintln(w, "# HELP licitalens_notifications_opportunity_sent_total Opportunity alerts delivered by this process.")
			fmt.Fprintln(w, "# TYPE licitalens_notifications_opportunity_sent_total counter")
			fmt.Fprintln(w, "licitalens_notifications_opportunity_sent_total", opportunitySent.Load())
			fmt.Fprintln(w, "# HELP licitalens_notifications_followup_sent_total Follow-up reminders delivered by this process.")
			fmt.Fprintln(w, "# TYPE licitalens_notifications_followup_sent_total counter")
			fmt.Fprintln(w, "licitalens_notifications_followup_sent_total", followUpSent.Load())
		}
		if err := RunHealthWithMetrics("notifications", healthChecks, metrics); err != nil {
			logger.Error("health server stopped", "error", err)
		}
	}()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	logger.Info("notification worker started", "interval", interval.String())
	for {
		result, err := worker.Run(context.Background())
		cycles.Add(1)
		opportunitySent.Add(uint64(result.OpportunitySent))
		followUpSent.Add(uint64(result.FollowUpSent))
		suppressed.Add(uint64(result.Suppressed))
		if err != nil {
			completedCycle.Store(false)
			failures.Add(1)
			lastFailure.Store(time.Now().UTC().Unix())
			logger.Error("notification cycle failed", "error", err)
		} else if result.Errors > 0 {
			completedCycle.Store(false)
			failures.Add(uint64(result.Errors))
			lastFailure.Store(time.Now().UTC().Unix())
			logger.Warn("notification cycle completed with delivery errors", "result", result)
		} else {
			completedCycle.Store(true)
			lastSuccess.Store(time.Now().UTC().Unix())
			logger.Info("notification cycle finished", "result", result)
		}
		<-ticker.C
	}
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
