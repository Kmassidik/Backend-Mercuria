package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/logger"
)

type Client struct {
	*redis.Client
	logger *logger.Logger
}

func Connect(cfg config.RedisConfig, log *logger.Logger) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 5,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Info("Connected to Redis")

	return &Client{Client: rdb, logger: log}, nil
}

func (c *Client) Health(ctx context.Context) error {
	return c.Ping(ctx).Err()
}

func (c *Client) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	lockKey := fmt.Sprintf("lock:%s", key)
	
	ok, err := c.SetNX(ctx, lockKey, "locked", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}

	if ok {
		c.logger.Debugf("Lock acquired: %s", lockKey)
	}

	return ok, nil
}

func (c *Client) ReleaseLock(ctx context.Context, key string) error {
	lockKey := fmt.Sprintf("lock:%s", key)
	
	err := c.Del(ctx, lockKey).Err()
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	c.logger.Debugf("Lock released: %s", lockKey)
	return nil
}

func (c *Client) CheckIdempotency(ctx context.Context, key string) (bool, error) {
	idempotencyKey := fmt.Sprintf("idempotency:%s", key)
	
	exists, err := c.Exists(ctx, idempotencyKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check idempotency: %w", err)
	}

	return exists > 0, nil
}

func (c *Client) SetIdempotency(ctx context.Context, key string, ttl time.Duration) error {
	idempotencyKey := fmt.Sprintf("idempotency:%s", key)
	
	err := c.Set(ctx, idempotencyKey, "used", ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to set idempotency: %w", err)
	}

	c.logger.Debugf("Idempotency key set: %s", idempotencyKey)
	return nil
}

func (c *Client) CacheWalletBalance(ctx context.Context, walletID string, balance string, ttl time.Duration) error {
	key := fmt.Sprintf("wallet:balance:%s", walletID)
	
	err := c.Set(ctx, key, balance, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to cache balance: %w", err)
	}

	return nil
}

func (c *Client) GetCachedWalletBalance(ctx context.Context, walletID string) (string, error) {
	key := fmt.Sprintf("wallet:balance:%s", walletID)
	
	balance, err := c.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil // Not found
	}
	if err != nil {
		return "", fmt.Errorf("failed to get cached balance: %w", err)
	}

	return balance, nil
}

func (c *Client) InvalidateWalletBalance(ctx context.Context, walletID string) error {
	key := fmt.Sprintf("wallet:balance:%s", walletID)
	
	return c.Del(ctx, key).Err()
}

func (c *Client) IncrementCounter(ctx context.Context, key string, ttl time.Duration) error {
	analyticsKey := fmt.Sprintf("analytics:%s", key)
	
	// Increment counter
	err := c.Incr(ctx, analyticsKey).Err()
	if err != nil {
		return fmt.Errorf("failed to increment counter: %w", err)
	}

	// Set expiration if this is the first increment
	c.Expire(ctx, analyticsKey, ttl)

	return nil
}

func (c *Client) GetCounter(ctx context.Context, key string) (int64, error) {
	analyticsKey := fmt.Sprintf("analytics:%s", key)
	
	val, err := c.Get(ctx, analyticsKey).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get counter: %w", err)
	}

	return val, nil
}