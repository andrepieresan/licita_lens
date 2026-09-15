package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"licitalens.dev/backend/internal/ingestion"
	"licitalens.dev/backend/internal/providers/pncp"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "sync", "backfill":
		runIngestion(os.Args[1], os.Args[2:])
	case "health":
		fmt.Println(`{"status":"ready"}`)
	case "reprocess", "replay-dlq", "reconcile-billing", "seed-demo":
		fmt.Printf("%s: comando disponível; conecte a infraestrutura para execução\n", os.Args[1])
	default:
		usage()
		os.Exit(2)
	}
}

func runIngestion(command string, args []string) {
	flags := flag.NewFlagSet(command, flag.ExitOnError)
	from := flags.String("from", time.Now().Format("2006-01-02"), "data inicial YYYY-MM-DD")
	to := flags.String("to", time.Now().Format("2006-01-02"), "data final YYYY-MM-DD")
	modality := flags.Int("modality", 6, "código da modalidade PNCP")
	maxPages := flags.Int("max-pages", 0, "limite de páginas; 0 processa todas")
	_ = flags.Parse(args)
	start, err := time.Parse("2006-01-02", *from)
	if err != nil {
		fail(err)
	}
	end, err := time.Parse("2006-01-02", *to)
	if err != nil {
		fail(err)
	}
	if command == "sync" {
		start, end = time.Now(), time.Now()
	}
	runner := ingestion.Runner{Client: pncp.NewClient(os.Getenv("PNCP_BASE_URL"), 30*time.Second), Archive: ingestion.FileArchive{Root: env("RAW_ARCHIVE_DIR", ".data/raw")}, Publisher: ingestion.JSONPublisher{Output: os.Stdout}, Logger: slog.New(slog.NewJSONHandler(os.Stderr, nil)), PageSize: envInt("PNCP_PAGE_SIZE", 50), MaxPages: *maxPages}
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		var syncErr error
		if command == "sync" {
			_, syncErr = runner.SyncRecent(context.Background(), day, *modality, envInt("PNCP_RECENT_PAGES", 3))
		} else {
			_, syncErr = runner.SyncDay(context.Background(), day, *modality)
		}
		if syncErr != nil {
			fail(syncErr)
		}
	}
}
func usage() {
	fmt.Fprintln(os.Stderr, "uso: licitalens <backfill|sync|reprocess|replay-dlq|reconcile-billing|health|seed-demo>")
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
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
