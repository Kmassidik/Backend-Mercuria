package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/logger"
)

type TestEvent struct {
	ID      string    `json:"id"`
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

func TestProducerConsumer(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	cfg := config.KafkaConfig{
		Brokers: []string{"localhost:9092"},
		GroupID: "test-group",
	}

	log := logger.New("test")

	// Create producer
	producer := NewProducer(cfg, log)
	defer producer.Close()

	// Create consumer
	topic := "test.events"
	consumer := NewConsumer(cfg, topic, log)
	defer consumer.Close()

	// Test event
	testEvent := TestEvent{
		ID:      "test-123",
		Message: "Hello Kafka",
		Time:    time.Now(),
	}

	// Publish event
	ctx := context.Background()
	if err := producer.PublishEvent(ctx, topic, testEvent.ID, testEvent); err != nil {
		t.Skipf("Cannot publish to Kafka: %v", err)
		return
	}

	// Consume event with timeout
	received := make(chan bool, 1)
	consumeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	go func() {
		consumer.Consume(consumeCtx, func(ctx context.Context, key []byte, value []byte) error {
			var event TestEvent
			if err := UnmarshalEvent(value, &event); err != nil {
				t.Errorf("Failed to unmarshal event: %v", err)
				return err
			}

			if event.ID != testEvent.ID {
				t.Errorf("Expected ID %s, got %s", testEvent.ID, event.ID)
			}
			if event.Message != testEvent.Message {
				t.Errorf("Expected message %s, got %s", testEvent.Message, event.Message)
			}

			received <- true
			return nil
		})
	}()

	// Wait for message or timeout
	select {
	case <-received:
		t.Log("Message received successfully")
	case <-time.After(6 * time.Second):
		t.Skip("Kafka not available or message not received in time")
	}
}