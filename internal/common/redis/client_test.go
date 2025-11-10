package redis

import (
	"context"
	"testing"
	"time"

	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/logger"
)

func TestConnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	cfg := config.RedisConfig{
		Host:     "localhost",
		Port:     "6379",
		Password: "",
		DB:       0,
	}

	log := logger.New("test")
	client, err := Connect(cfg, log)
	if err != nil {
		t.Skipf("Cannot connect to Redis: %v", err)
		return
	}
	defer client.Close()

	// Test health
	ctx := context.Background()
	if err := client.Health(ctx); err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

func TestLockMechanism(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	cfg := config.RedisConfig{
		Host:     "localhost",
		Port:     "6379",
		Password: "",
		DB:       0,
	}

	log := logger.New("test")
	client, err := Connect(cfg, log)
	if err != nil {
		t.Skip("Redis not available")
		return
	}
	defer client.Close()

	ctx := context.Background()
	lockKey := "test-wallet-123"

	// Test acquiring lock
	acquired, err := client.AcquireLock(ctx, lockKey, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	if !acquired {
		t.Error("Expected to acquire lock")
	}

	// Test lock is already held
	acquired, err = client.AcquireLock(ctx, lockKey, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed on second lock attempt: %v", err)
	}
	if acquired {
		t.Error("Should not acquire lock when already held")
	}

	// Release lock
	if err := client.ReleaseLock(ctx, lockKey); err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Should be able to acquire again
	acquired, err = client.AcquireLock(ctx, lockKey, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to re-acquire lock: %v", err)
	}
	if !acquired {
		t.Error("Expected to re-acquire lock after release")
	}

	// Cleanup
	client.ReleaseLock(ctx, lockKey)
}

func TestIdempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	cfg := config.RedisConfig{
		Host:     "localhost",
		Port:     "6379",
		Password: "",
		DB:       0,
	}

	log := logger.New("test")
	client, err := Connect(cfg, log)
	if err != nil {
		t.Skip("Redis not available")
		return
	}
	defer client.Close()

	ctx := context.Background()
	idempotencyKey := "test-request-uuid-123"

	// Check idempotency key doesn't exist
	exists, err := client.CheckIdempotency(ctx, idempotencyKey)
	if err != nil {
		t.Fatalf("Failed to check idempotency: %v", err)
	}
	if exists {
		t.Error("Idempotency key should not exist initially")
	}

	// Set idempotency key
	if err := client.SetIdempotency(ctx, idempotencyKey, 30*time.Minute); err != nil {
		t.Fatalf("Failed to set idempotency: %v", err)
	}

	// Check it now exists
	exists, err = client.CheckIdempotency(ctx, idempotencyKey)
	if err != nil {
		t.Fatalf("Failed to check idempotency: %v", err)
	}
	if !exists {
		t.Error("Idempotency key should exist after setting")
	}

	// Cleanup
	client.Del(ctx, "idempotency:"+idempotencyKey)
}

func TestWalletBalanceCache(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	cfg := config.RedisConfig{
		Host:     "0.0.0.0",
		Port:     "6379",
		Password: "",
		DB:       0,
	}

	log := logger.New("test")
	client, err := Connect(cfg, log)
	if err != nil {
		t.Skip("Redis not available")
		return
	}
	defer client.Close()

	ctx := context.Background()
	walletID := "wallet-test-456"
	balance := "1000.50"

	// Cache balance
	if err := client.CacheWalletBalance(ctx, walletID, balance, 10*time.Minute); err != nil {
		t.Fatalf("Failed to cache balance: %v", err)
	}

	// Retrieve cached balance
	cachedBalance, err := client.GetCachedWalletBalance(ctx, walletID)
	if err != nil {
		t.Fatalf("Failed to get cached balance: %v", err)
	}
	if cachedBalance != balance {
		t.Errorf("Expected balance %s, got %s", balance, cachedBalance)
	}

	// Invalidate cache
	if err := client.InvalidateWalletBalance(ctx, walletID); err != nil {
		t.Fatalf("Failed to invalidate cache: %v", err)
	}

	// Verify cache is empty
	cachedBalance, err = client.GetCachedWalletBalance(ctx, walletID)
	if err != nil {
		t.Fatalf("Failed to check invalidated cache: %v", err)
	}
	if cachedBalance != "" {
		t.Error("Cache should be empty after invalidation")
	}
}

func TestCounter(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	cfg := config.RedisConfig{
		Host:     "localhost",
		Port:     "6379",
		Password: "",
		DB:       0,
	}

	log := logger.New("test")
	client, err := Connect(cfg, log)
	if err != nil {
		t.Skip("Redis not available")
		return
	}
	defer client.Close()

	ctx := context.Background()
	counterKey := "volume:2025-11-10"

	// Increment counter multiple times
	for i := 0; i < 5; i++ {
		if err := client.IncrementCounter(ctx, counterKey, 24*time.Hour); err != nil {
			t.Fatalf("Failed to increment counter: %v", err)
		}
	}

	// Get counter value
	count, err := client.GetCounter(ctx, counterKey)
	if err != nil {
		t.Fatalf("Failed to get counter: %v", err)
	}
	if count != 5 {
		t.Errorf("Expected counter to be 5, got %d", count)
	}

	// Cleanup
	client.Del(ctx, "analytics:"+counterKey)
}