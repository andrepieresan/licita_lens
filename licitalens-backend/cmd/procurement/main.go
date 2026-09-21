package main

import (
	"context"
	"fmt"
	"io"
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
		metrics := func(w io.Writer) {
			value := consumer.Metrics()
			fmt.Fprintln(w, "# HELP licitalens_procurement_messages_total Procurement events persisted by this process.")
			fmt.Fprintln(w, "# TYPE licitalens_procurement_messages_total counter")
			fmt.Fprintln(w, "licitalens_procurement_messages_total", value.Processed)
			fmt.Fprintln(w, "# HELP licitalens_procurement_dead_letters_total Invalid procurement events accepted by the dead-letter topic.")
			fmt.Fprintln(w, "# TYPE licitalens_procurement_dead_letters_total counter")
			fmt.Fprintln(w, "licitalens_procurement_dead_letters_total", value.DLQSent)
			fmt.Fprintln(w, "# HELP licitalens_procurement_consumer_lag Kafka consumer lag reported by kafka-go.")
			fmt.Fprintln(w, "# TYPE licitalens_procurement_consumer_lag gauge")
			fmt.Fprintln(w, "licitalens_procurement_consumer_lag", value.Lag)
		}
		if err := app.RunHealthWithMetrics("procurement", []app.HealthCheck{{Name: "postgres", Check: database.Ping}}, metrics); err != nil {
			logger.Error("health server stopped", "error", err)
			cancel()
		}
	}()
	if err := consumer.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
