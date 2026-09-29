package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"licitalens.dev/backend/internal/ingestion"
	"licitalens.dev/backend/internal/providers/pncp"
	app "licitalens.dev/backend/internal/runtime"
	"licitalens.dev/backend/internal/store"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	archive, publisher, closePublisher := adapters(ctx, logger)
	defer closePublisher()
	var checkpoints store.DataStore
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		if db, err := store.NewPostgres(ctx, databaseURL); err == nil {
			checkpoints = db
			defer db.Close()
		} else {
			logger.Error("checkpoint store unavailable", "error", err)
			return
		}
	}
	runner := ingestion.Runner{
		Client: pncp.NewClientWithFallback(
			os.Getenv("PNCP_BASE_URL"), os.Getenv("PNCP_FALLBACK_URL"), os.Getenv("PNCP_FALLBACK_API_KEY"),
			envDuration("PNCP_REQUEST_TIMEOUT", 30*time.Second),
		),
		Archive: archive, Publisher: publisher, Logger: logger, PageSize: envInt("PNCP_PAGE_SIZE", 50), Checkpoints: checkpoints,
		MaxAttempts: envInt("PNCP_MAX_ATTEMPTS", 3), RetryDelay: envDuration("PNCP_RETRY_DELAY", 500*time.Millisecond),
	}
	var completedCycle atomic.Bool
	var lastSuccess, lastFailure atomic.Int64
	var lastRunTimestamp, lastRunSuccessful atomic.Int64
	var syncedPages, syncedRecords, syncFailures atomic.Uint64
	healthChecks := []app.HealthCheck{{Name: "initial_sync", Check: func(context.Context) error {
		if !completedCycle.Load() {
			return errors.New("initial synchronization pending")
		}
		return nil
	}}}
	if pinger, ok := checkpoints.(store.Pinger); ok {
		healthChecks = append(healthChecks, app.HealthCheck{Name: "postgres", Check: pinger.Ping})
	}
	go func() {
		metrics := func(w io.Writer) {
			fmt.Fprintln(w, "# HELP licitalens_ingestion_last_success_timestamp_seconds Unix timestamp of the most recent successful PNCP page sync.")
			fmt.Fprintln(w, "# TYPE licitalens_ingestion_last_success_timestamp_seconds gauge")
			fmt.Fprintln(w, "licitalens_ingestion_last_success_timestamp_seconds", lastSuccess.Load())
			fmt.Fprintln(w, "# HELP licitalens_ingestion_last_failure_timestamp_seconds Unix timestamp of the most recent failed PNCP sync.")
			fmt.Fprintln(w, "# TYPE licitalens_ingestion_last_failure_timestamp_seconds gauge")
			fmt.Fprintln(w, "licitalens_ingestion_last_failure_timestamp_seconds", lastFailure.Load())
			fmt.Fprintln(w, "# HELP licitalens_ingestion_last_run_timestamp_seconds Unix timestamp of the most recently completed sync cycle.")
			fmt.Fprintln(w, "# TYPE licitalens_ingestion_last_run_timestamp_seconds gauge")
			fmt.Fprintln(w, "licitalens_ingestion_last_run_timestamp_seconds", lastRunTimestamp.Load())
			fmt.Fprintln(w, "# HELP licitalens_ingestion_last_run_success Whether the most recently completed sync cycle succeeded (1) or failed (0).")
			fmt.Fprintln(w, "# TYPE licitalens_ingestion_last_run_success gauge")
			fmt.Fprintln(w, "licitalens_ingestion_last_run_success", lastRunSuccessful.Load())
			fmt.Fprintln(w, "# HELP licitalens_ingestion_pages_total Number of PNCP pages synchronized by this process.")
			fmt.Fprintln(w, "# TYPE licitalens_ingestion_pages_total counter")
			fmt.Fprintln(w, "licitalens_ingestion_pages_total", syncedPages.Load())
			fmt.Fprintln(w, "# HELP licitalens_ingestion_records_total Number of PNCP records synchronized by this process.")
			fmt.Fprintln(w, "# TYPE licitalens_ingestion_records_total counter")
			fmt.Fprintln(w, "licitalens_ingestion_records_total", syncedRecords.Load())
			fmt.Fprintln(w, "# HELP licitalens_ingestion_failures_total Failed PNCP sync attempts by this process.")
			fmt.Fprintln(w, "# TYPE licitalens_ingestion_failures_total counter")
			fmt.Fprintln(w, "licitalens_ingestion_failures_total", syncFailures.Load())
		}
		if err := app.RunHealthWithMetrics("ingestion", healthChecks, metrics); err != nil {
			logger.Error("health server stopped", "error", err)
			cancel()
		}
	}()
	interval := 5 * time.Hour
	if value := os.Getenv("SYNC_INTERVAL"); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			interval = parsed
		}
	}
	// A durable day cursor lets a restarted worker recover every pending day,
	// while the lookback window continues to catch delayed provider updates.
	// The recovery cap is configurable; zero means unlimited.
	sync := func(lookbackDays int) {
		startedAt := time.Now().UTC()
		cyclePages, cycleRecords := 0, 0
		cycleErrors := []string{}
		if lookbackDays < 1 {
			lookbackDays = 1
		}
		now := time.Now().UTC().Truncate(24 * time.Hour)
		start := now.AddDate(0, 0, -lookbackDays+1)
		if checkpoints != nil {
			if cursor, err := checkpoints.IngestionCheckpoint(ctx, "pncp", "recovery"); err != nil {
				logger.Error("read ingestion recovery cursor", "error", err)
			} else if cursor != "" {
				if last, parseErr := time.Parse("2006-01-02", cursor); parseErr == nil && last.AddDate(0, 0, 1).Before(start) {
					start = last.AddDate(0, 0, 1)
				}
			}
		}
		maxRecoveryDays := envInt("PNCP_MAX_RECOVERY_DAYS", 30)
		if maxRecoveryDays > 0 {
			minimum := now.AddDate(0, 0, -maxRecoveryDays+1)
			if start.Before(minimum) {
				start = minimum
			}
		}
		runDayRange(start, now, func(day time.Time) bool {
			dayOK := true
			for _, modality := range modalities() {
				if result, err := runner.SyncRecent(ctx, day, modality, envInt("PNCP_RECENT_PAGES", 3)); err != nil {
					dayOK = false
					syncFailures.Add(1)
					lastFailure.Store(time.Now().UTC().Unix())
					if ctx.Err() == nil {
						logger.Error("sync failed", "modality", modality, "day", day.Format("2006-01-02"), "error", err)
						cycleErrors = append(cycleErrors, fmt.Sprintf("modalidade %d (%s): %v", modality, day.Format("2006-01-02"), err))
					}
				} else {
					syncedPages.Add(uint64(result.Pages))
					syncedRecords.Add(uint64(result.Records))
					cyclePages += result.Pages
					cycleRecords += result.Records
				}
			}
			if !dayOK {
				// Keep the recovery cursor before the first incomplete day. Advancing
				// to a later successful day would hide this gap on the next cycle.
				return false
			}
			completedCycle.Store(true)
			lastSuccess.Store(time.Now().UTC().Unix())
			if checkpoints != nil {
				if err := checkpoints.SaveIngestionCheckpoint(ctx, "pncp", "recovery", day.Format("2006-01-02")); err != nil {
					logger.Error("save ingestion recovery cursor", "day", day.Format("2006-01-02"), "error", err)
					cycleErrors = append(cycleErrors, "salvar cursor de recuperação: "+err.Error())
					return false
				}
			}
			return true
		})
		if statusStore, ok := checkpoints.(store.IngestionRunStore); ok {
			if ctx.Err() != nil {
				cycleErrors = append(cycleErrors, "execução interrompida: "+ctx.Err().Error())
			}
			finishedAt := time.Now().UTC()
			run := store.IngestionRun{StartedAt: startedAt, FinishedAt: finishedAt, Success: len(cycleErrors) == 0, Pages: cyclePages, Records: cycleRecords, Error: boundedError(strings.Join(cycleErrors, " | "))}
			if err := statusStore.RecordIngestionRun(ctx, run); err != nil {
				logger.Error("record ingestion run", "error", err)
			} else {
				lastRunTimestamp.Store(finishedAt.Unix())
				if run.Success {
					lastRunSuccessful.Store(1)
				} else {
					lastRunSuccessful.Store(0)
				}
			}
		}
	}
	sync(envInt("PNCP_INITIAL_DAYS", 7))
	if strings.EqualFold(os.Getenv("PNCP_RUN_ONCE"), "true") {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sync(envInt("PNCP_LOOKBACK_DAYS", 2))
		}
	}
}

func boundedError(value string) string {
	const maxRunErrorCharacters = 2048
	characters := []rune(value)
	if len(characters) > maxRunErrorCharacters {
		return string(characters[:maxRunErrorCharacters]) + "…"
	}
	return value
}

func runDayRange(start, end time.Time, syncDay func(time.Time) bool) {
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if !syncDay(day) {
			return
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
	// Search every current PNCP modality by default so user profiles can filter
	// the complete official feed. Operators may narrow this for rate limits.
	for _, value := range strings.Split(env("PNCP_MODALITIES", "1,2,3,4,5,6,7,8,9,10,11,12,13"), ",") {
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

func envDuration(key string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
