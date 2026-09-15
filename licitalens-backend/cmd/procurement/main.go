package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"licitalens.dev/backend/internal/ingestion"
	app "licitalens.dev/backend/internal/runtime"
	"licitalens.dev/backend/internal/store"
)

func main() {
	if os.Getenv("DATABASE_URL") == "" || os.Getenv("KAFKA_BROKERS") == "" {
		if err := app.RunHealth("procurement"); err != nil {
			log.Fatal(err)
		}
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	database, err := store.NewPostgres(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	consumer := ingestion.NewConsumer(os.Getenv("KAFKA_BROKERS"), "licitalens-procurement", "procurement.discovered.v1", database, logger)
	defer consumer.Close()
	go func() {
		if err := app.RunHealth("procurement"); err != nil {
			logger.Error("health server stopped", "error", err)
			cancel()
		}
	}()
	if err := consumer.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
