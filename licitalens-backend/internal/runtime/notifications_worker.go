package runtime

import (
	"context"
	"log/slog"
	"os"
	"time"

	"licitalens.dev/backend/internal/notifications"
)

func RunNotificationWorker() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	data, closeStore, err := dataStore(logger)
	if err != nil {
		return err
	}
	defer closeStore()
	worker := notifications.NewWorker(data, logger, notifications.WorkerConfig{
		DemoMode: env("DEMO_MODE", "true") == "true",
		SMTP: notifications.SMTPConfig{
			Host: env("SMTP_HOST", "localhost"),
			Port: env("SMTP_PORT", "1025"),
			From: env("SMTP_FROM", "alerts@licitalens.local"),
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
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	logger.Info("notification worker started", "interval", interval.String())
	for {
		result, err := worker.Run(context.Background())
		if err != nil {
			logger.Error("notification cycle failed", "error", err)
		} else {
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
