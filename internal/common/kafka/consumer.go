package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/segmentio/kafka-go"
)

type Consumer struct {
	reader *kafka.Reader
	logger *logger.Logger
}

// EventHandler is a function that processes Kafka events
type EventHandler func(ctx context.Context, key []byte, value []byte) error

// NewConsumer creates a new Kafka consumer
func NewConsumer(cfg config.KafkaConfig, topic string, log *logger.Logger) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.GroupID,
		Topic:          topic,
		MinBytes:       1,
		MaxBytes:       10e6, // 10MB
		CommitInterval: 1 * time.Second,
		StartOffset:    kafka.FirstOffset, // Read from beginning
		MaxWait:        500 * time.Millisecond,
	})

	log.Infof("Kafka consumer initialized for topic: %s", topic)

	return &Consumer{
		reader: reader,
		logger: log,
	}
}

// Consume starts consuming messages and calls the handler for each message
func (c *Consumer) Consume(ctx context.Context, handler EventHandler) error {
	c.logger.Info("Starting Kafka consumer")

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Consumer context cancelled")
			return ctx.Err()
		default:
			msg, err := c.reader.FetchMessage(ctx)
			if err != nil {
				if err == context.Canceled || err == context.DeadlineExceeded {
					c.logger.Info("Consumer stopped")
					return err
				}
				c.logger.Errorf("Failed to fetch message: %v", err)
				time.Sleep(1 * time.Second) // Backoff on error
				continue
			}

			c.logger.Debugf("Received message from topic %s: key=%s", msg.Topic, string(msg.Key))

			// Process message
			if err := handler(ctx, msg.Key, msg.Value); err != nil {
				c.logger.Errorf("Failed to process message: %v", err)
				// Don't commit on error - message will be retried
				continue
			}

			// Commit message
			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				c.logger.Errorf("Failed to commit message: %v", err)
			}
		}
	}
}

// Close closes the consumer
func (c *Consumer) Close() error {
	c.logger.Info("Closing Kafka consumer")
	return c.reader.Close()
}

// UnmarshalEvent is a helper to unmarshal JSON events
func UnmarshalEvent(value []byte, v interface{}) error {
	if err := json.Unmarshal(value, v); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}
	return nil
}