package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"licitalens.dev/backend/internal/domain"
	"licitalens.dev/backend/internal/store"
)

type procurementEvent struct {
	Data domain.Opportunity `json:"data"`
}
type Consumer struct {
	reader *kafka.Reader
	store  store.DataStore
	logger *slog.Logger
}

func NewConsumer(brokers, group, topic string, data store.DataStore, logger *slog.Logger) *Consumer {
	return &Consumer{reader: kafka.NewReader(kafka.ReaderConfig{Brokers: splitBrokers(brokers), GroupID: group, Topic: topic, MinBytes: 1, MaxBytes: 10 << 20, MaxWait: 2 * time.Second}), store: data, logger: logger}
}
func (c *Consumer) Close() error { return c.reader.Close() }
func (c *Consumer) Run(ctx context.Context) error {
	for {
		message, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read procurement event: %w", err)
		}
		var event procurementEvent
		if err := json.Unmarshal(message.Value, &event); err != nil {
			c.logger.Error("invalid procurement event", "error", err)
			continue
		}
		if event.Data.ID == "" {
			c.logger.Error("procurement event without id")
			continue
		}
		if err := c.store.PutOpportunity(event.Data); err != nil {
			return fmt.Errorf("persist opportunity %s: %w", event.Data.ID, err)
		}
	}
}
func splitBrokers(value string) []string {
	result := []string{}
	for _, item := range strings.Split(value, ",") {
		if strings.TrimSpace(item) != "" {
			result = append(result, strings.TrimSpace(item))
		}
	}
	return result
}
