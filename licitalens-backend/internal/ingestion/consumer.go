package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
	"licitalens.dev/backend/internal/domain"
	"licitalens.dev/backend/internal/store"
)

type procurementEvent struct {
	Data domain.Opportunity `json:"data"`
}
type Consumer struct {
	reader    *kafka.Reader
	dlq       *kafka.Writer
	store     store.DataStore
	logger    *slog.Logger
	processed atomic.Uint64
	dlqSent   atomic.Uint64
}

func NewConsumer(brokers, group, topic string, data store.DataStore, logger *slog.Logger) *Consumer {
	brokerList := splitBrokers(brokers)
	return &Consumer{
		reader: kafka.NewReader(kafka.ReaderConfig{Brokers: brokerList, GroupID: group, Topic: topic, MinBytes: 1, MaxBytes: 10 << 20, MaxWait: 2 * time.Second}),
		dlq:    &kafka.Writer{Addr: kafka.TCP(brokerList...), Topic: "procurement.dead-letter.v1", Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll},
		store:  data,
		logger: logger,
	}
}
func (c *Consumer) Close() error {
	readerErr := c.reader.Close()
	writerErr := c.dlq.Close()
	if readerErr != nil {
		return readerErr
	}
	return writerErr
}

type ConsumerMetrics struct {
	Processed uint64
	DLQSent   uint64
	Lag       int64
}

func (c *Consumer) Metrics() ConsumerMetrics {
	return ConsumerMetrics{Processed: c.processed.Load(), DLQSent: c.dlqSent.Load(), Lag: c.reader.Stats().Lag}
}

func (c *Consumer) Run(ctx context.Context) error {
	for {
		message, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read procurement event: %w", err)
		}
		var event procurementEvent
		if err := json.Unmarshal(message.Value, &event); err != nil {
			if err := c.deadLetter(ctx, message, "invalid_json", err.Error()); err != nil {
				return err
			}
			continue
		}
		if event.Data.ID == "" {
			if err := c.deadLetter(ctx, message, "missing_opportunity_id", "event data has no id"); err != nil {
				return err
			}
			continue
		}
		if err := c.store.PutOpportunity(event.Data); err != nil {
			return fmt.Errorf("persist opportunity %s: %w", event.Data.ID, err)
		}
		if err := c.reader.CommitMessages(ctx, message); err != nil {
			return fmt.Errorf("commit procurement event: %w", err)
		}
		c.processed.Add(1)
	}
}

type deadLetterEvent struct {
	ID                string    `json:"id"`
	Type              string    `json:"type"`
	OccurredAt        time.Time `json:"occurred_at"`
	Reason            string    `json:"reason"`
	Error             string    `json:"error"`
	OriginalTopic     string    `json:"original_topic"`
	OriginalKey       []byte    `json:"original_key,omitempty"`
	OriginalValue     []byte    `json:"original_value"`
	OriginalPartition int       `json:"original_partition"`
	OriginalOffset    int64     `json:"original_offset"`
}

func (c *Consumer) deadLetter(ctx context.Context, message kafka.Message, reason, detail string) error {
	id := fmt.Sprintf("%s:%d:%d", message.Topic, message.Partition, message.Offset)
	payload, err := json.Marshal(deadLetterEvent{
		ID: id, Type: "procurement.dead-letter.v1", OccurredAt: time.Now().UTC(), Reason: reason, Error: detail,
		OriginalTopic: message.Topic, OriginalKey: message.Key, OriginalValue: message.Value,
		OriginalPartition: message.Partition, OriginalOffset: message.Offset,
	})
	if err != nil {
		return fmt.Errorf("encode procurement dead letter: %w", err)
	}
	if err := c.dlq.WriteMessages(ctx, kafka.Message{Key: []byte(id), Value: payload}); err != nil {
		return fmt.Errorf("publish procurement dead letter: %w", err)
	}
	if err := c.reader.CommitMessages(ctx, message); err != nil {
		return fmt.Errorf("commit dead-lettered procurement event: %w", err)
	}
	c.dlqSent.Add(1)
	c.logger.Warn("procurement event moved to dead letter", "id", id, "reason", reason)
	return nil
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
