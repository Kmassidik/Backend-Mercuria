package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/db"
	"github.com/kmassidik/mercuria/internal/common/kafka"
	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/internal/common/middleware"
	"github.com/kmassidik/mercuria/internal/common/redis"
	"github.com/kmassidik/mercuria/internal/ledger"
	"github.com/kmassidik/mercuria/pkg/outbox"
)

func main() {
    // Load .env file
    if err := godotenv.Load(); err != nil {
        fmt.Println("No .env file found, using system environment variables")
    }

    // Load configuration
    cfg, err := config.Load("ledger")
    if err != nil {
        fmt.Printf("Failed to load config: %v\n", err)
        os.Exit(1)
    }

    // Initialize logger
    log := logger.New("ledger-service")

    // Connect to database
    database, err := db.Connect(cfg.Database, log)
    if err != nil {
        log.Fatalf("Failed to connect to database: %v", err)
    }
    defer database.Close()

    // Connect to Redis
    redisClient, err := redis.Connect(cfg.Redis, log)
    if err != nil {
        log.Fatalf("Failed to connect to Redis: %v", err)
    }
    defer redisClient.Close()

    // Initialize Kafka producer
    producer := kafka.NewProducer(cfg.Kafka, log)
    defer producer.Close()

    // Verify Kafka is reachable
    log.Info("Checking Kafka connection...")
    kafkaCtx, kafkaCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer kafkaCancel()

    if err := producer.Ping(kafkaCtx); err != nil {
        log.Fatalf("❌ Failed to connect to Kafka: %v", err)
    }
    log.Info("✅ Kafka is healthy")

    // Initialize Kafka consumer
    consumer := kafka.NewConsumer(cfg.Kafka, "transaction.completed", log)
    defer consumer.Close()

    // Initialize repositories
    repo := ledger.NewRepository(database, log)
    outboxRepo := outbox.NewRepository(database.DB, log)

    // Initialize service
    service := ledger.NewService(repo, outboxRepo, database, log)

    // Initialize handler
    handler := ledger.NewHandler(service)

    // Create HTTP server
    mux := http.NewServeMux()

    // Apply middleware
    var httpHandler http.Handler = mux
    httpHandler = middleware.CORS(httpHandler)
    httpHandler = middleware.Logging(log)(httpHandler)
    httpHandler = middleware.Recovery(log)(httpHandler)

    // Register routes
    handler.RegisterRoutes(mux, cfg.JWT.Secret)

    // Track consumer health
    var consumerHealthy atomic.Bool
    consumerHealthy.Store(true)

    // Enhanced health check
    mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
        status := map[string]interface{}{
            "status":           "healthy",
            "service":          "ledger",
            "kafka_consumer":   consumerHealthy.Load(),
            "timestamp":        time.Now(),
        }

        w.Header().Set("Content-Type", "application/json")
        if !consumerHealthy.Load() {
            w.WriteHeader(http.StatusServiceUnavailable)
            status["status"] = "degraded"
        }
        json.NewEncoder(w).Encode(status)
    })


    // Start outbox publisher (background worker)
    outboxPublisher := outbox.NewPublisher(outboxRepo, producer, log, 5*time.Second)
    publisherCtx, cancelPublisher := context.WithCancel(context.Background())
    defer cancelPublisher()

    go outboxPublisher.Start(publisherCtx)
    log.Info("Outbox publisher started")

    // Start Kafka consumer worker (The original second block, now properly corrected and running the consumer with health checks)
    go func() {
        log.Info("Kafka consumer started for ledger-service")
        lastSuccessfulRead := time.Now()

        for {
            select {
            case <-publisherCtx.Done():
                log.Info("Kafka consumer stopped")
                consumerHealthy.Store(false)
                return
            default:
                err := consumer.Consume(publisherCtx, func(ctx context.Context, key, value []byte) error {
                    lastSuccessfulRead = time.Now()
                    consumerHealthy.Store(true)
                    return service.ProcessTransactionEvent(ctx, key, value)
                })

                if err != nil {
                    log.Errorf("Error consuming Kafka message: %v", err)

                    // Mark unhealthy if no successful reads for 30s
                    if time.Since(lastSuccessfulRead) > 30*time.Second {
                        consumerHealthy.Store(false)
                    }

                    time.Sleep(5 * time.Second)
                }
            }
        }
    }()


    server := &http.Server{
        Addr:         ":" + cfg.Service.Port,
        Handler:      httpHandler,
        ReadTimeout:  15 * time.Second,
        WriteTimeout: 15 * time.Second,
        IdleTimeout:  60 * time.Second,
    }

    // Start HTTP server in goroutine (THIS WAS THE MISSING PART)
    go func() {
        log.Infof("HTTP server starting on port %s", cfg.Service.Port)
        // This is the line that actually starts the server and listens for connections
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("Could not start HTTP server: %v", err)
        }
    }()


    // Graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    log.Info("Shutting down server...")

    // Stop background workers
    cancelPublisher()

    // Shutdown HTTP server
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()

    if err := server.Shutdown(shutdownCtx); err != nil {
        log.Fatalf("Server forced to shutdown: %v", err)
    }

    log.Info("Server exited")
}