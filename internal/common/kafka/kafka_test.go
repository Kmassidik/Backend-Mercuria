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
		GroupID: "test-group-" + time.Now().Format("20060102150405"),
	}

	log := logger.New("test")

	// Create unique topic for this test
	topic := "test-events-" + time.Now().Format("20060102150405")

	// Create consumer FIRST
	consumer := NewConsumer(cfg, topic, log)
	defer consumer.Close()

	// Create producer
	producer := NewProducer(cfg, log)
	defer producer.Close()

	// Test event
	testEvent := TestEvent{
		ID:      "test-123",
		Message: "Hello Kafka",
		Time:    time.Now(),
	}

	// Start consumer in background
	received := make(chan bool, 1)
	errChan := make(chan error, 1)
	consumeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		err := consumer.Consume(consumeCtx, func(ctx context.Context, key []byte, value []byte) error {
			var event TestEvent
			if err := UnmarshalEvent(value, &event); err != nil {
				errChan <- err
				return err
			}

			if event.ID != testEvent.ID {
				t.Errorf("Expected ID %s, got %s", testEvent.ID, event.ID)
			}
			if event.Message != testEvent.Message {
				t.Errorf("Expected message %s, got %s", testEvent.Message, event.Message)
			}

			t.Logf("✅ Message received: ID=%s, Message=%s", event.ID, event.Message)
			received <- true
			cancel() // Stop consuming
			return nil
		})
		
		if err != nil && err != context.Canceled {
			errChan <- err
		}
	}()

	// Wait for consumer to be ready
	time.Sleep(2 * time.Second)

	// Publish event
	ctx := context.Background()
	if err := producer.PublishEvent(ctx, topic, testEvent.ID, testEvent); err != nil {
		t.Fatalf("Failed to publish to Kafka: %v", err)
	}

	t.Log("✅ Message published successfully")

	// Wait for message or timeout
	select {
	case <-received:
		t.Log("✅ Test passed: Message received successfully")
	case err := <-errChan:
		t.Fatalf("❌ Consumer error: %v", err)
	case <-time.After(8 * time.Second):
		t.Fatal("❌ Timeout: Message not received in time")
	}
}