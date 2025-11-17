package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafka.Writer
	logger *logger.Logger
}

// NewProducer creates a new Kafka producer
func NewProducer(cfg config.KafkaConfig, log *logger.Logger) *Producer {
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(cfg.Brokers...),
		Balancer:               &kafka.LeastBytes{},
		RequiredAcks:           kafka.RequireAll,
		Async:                  false,
		AllowAutoTopicCreation: true,
	}

	log.Info("Kafka producer initialized")

	return &Producer{
		writer: writer,
		logger: log,
	}
}

// PublishEvent publishes an event to a Kafka topic
func (p *Producer) PublishEvent(ctx context.Context, topic string, key string, event interface{}) error {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	msg := kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: eventBytes,
	}

	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		p.logger.Errorf("Failed to publish event to topic %s: %v", topic, err)
		return fmt.Errorf("failed to publish event: %w", err)
	}

	p.logger.Debugf("Event published to topic %s with key %s", topic, key)
	return nil
}

// Close closes the producer
func (p *Producer) Close() error {
	p.logger.Info("Closing Kafka producer")
	return p.writer.Close()
}

// Ping checks if Kafka is reachable
func (p *Producer) Ping(ctx context.Context) error {
	// Create a temporary connection to check Kafka availability
	conn, err := kafka.DialContext(ctx, "tcp", p.writer.Addr.String())
	if err != nil {
		return fmt.Errorf("kafka not reachable: %w", err)
	}
	defer conn.Close()

	// Try to get broker metadata
	brokers, err := conn.Brokers()
	if err != nil {
		return fmt.Errorf("failed to get kafka brokers: %w", err)
	}

	if len(brokers) == 0 {
		return fmt.Errorf("no kafka brokers available")
	}

	p.logger.Debugf("Kafka is healthy, found %d broker(s)", len(brokers))
	return nil
}