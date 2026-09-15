package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"licitalens.dev/backend/internal/ingestion"
	"licitalens.dev/backend/internal/providers/pncp"
	app "licitalens.dev/backend/internal/runtime"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	go func() {
		if err := app.RunHealth("ingestion"); err != nil {
			logger.Error("health server stopped", "error", err)
			cancel()
		}
	}()
	archive, publisher, closePublisher := adapters(ctx, logger)
	defer closePublisher()
	runner := ingestion.Runner{Client: pncp.NewClient(os.Getenv("PNCP_BASE_URL"), 30*time.Second), Archive: archive, Publisher: publisher, Logger: logger, PageSize: envInt("PNCP_PAGE_SIZE", 50)}
	interval := 2 * time.Minute
	if value := os.Getenv("SYNC_INTERVAL"); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			interval = parsed
		}
	}
	sync := func() {
		for _, modality := range modalities() {
			if _, err := runner.SyncRecent(ctx, time.Now(), modality, envInt("PNCP_RECENT_PAGES", 3)); err != nil && ctx.Err() == nil {
				logger.Error("sync failed", "modality", modality, "error", err)
			}
		}
	}
	sync()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sync()
		}
	}
}

func adapters(ctx context.Context, logger *slog.Logger) (ingestion.Archive, ingestion.Publisher, func()) {
	if endpoint, brokers := os.Getenv("S3_ENDPOINT"), os.Getenv("KAFKA_BROKERS"); endpoint != "" && brokers != "" {
		archive, err := ingestion.NewS3Archive(ctx, endpoint, os.Getenv("S3_ACCESS_KEY"), os.Getenv("S3_SECRET_KEY"), env("S3_BUCKET", "licitalens-raw"), env("S3_SECURE", "false") == "true")
		if err != nil {
			logger.Error("initialize S3", "error", err)
			os.Exit(1)
		}
		publisher := ingestion.NewKafkaPublisher(brokers)
		return archive, publisher, func() { _ = publisher.Close() }
	}
	logger.Warn("using local ingestion adapters")
	return ingestion.FileArchive{Root: env("RAW_ARCHIVE_DIR", ".data/raw")}, ingestion.JSONPublisher{Output: os.Stdout}, func() {}
}
func modalities() []int {
	result := []int{}
	for _, value := range strings.Split(env("PNCP_MODALITIES", "6"), ",") {
		if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			result = append(result, parsed)
		}
	}
	return result
}
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}
