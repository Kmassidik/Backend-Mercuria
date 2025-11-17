package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/kmassidik/mercuria/internal/analytics"
	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/db"
	"github.com/kmassidik/mercuria/internal/common/kafka"
	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/internal/common/middleware"
	"github.com/kmassidik/mercuria/internal/common/redis"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found, using system environment variables")
	}

	// Load configuration
	cfg, err := config.Load("analytics")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New("analytics-service")

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

	// Initialize Kafka consumer
	consumer := kafka.NewConsumer(cfg.Kafka, "analytics-group", log)
	defer consumer.Close()

	// Verify Kafka is reachable (through consumer check)
	log.Info("Checking Kafka connection...")
	_, kafkaCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer kafkaCancel()

	// Simple check - if consumer initialized, Kafka should be available
	if consumer == nil {
		log.Fatal("❌ Failed to initialize Kafka consumer")
	}
	log.Info("✅ Kafka consumer initialized")

	// Initialize repositories
	repo := analytics.NewRepository(database.DB)

	// Initialize service
	service := analytics.NewService(repo, redisClient)

	// Initialize handler
	handler := analytics.NewHandler(service)

	// Create HTTP server
	mux := http.NewServeMux()

	// Apply middleware
	var httpHandler http.Handler = mux
	httpHandler = middleware.CORS(httpHandler)
	httpHandler = middleware.Logging(log)(httpHandler)
	httpHandler = middleware.Recovery(log)(httpHandler)

	// Register routes
	analytics.SetupRoutes(mux, handler)

	// Start Kafka consumer worker
	consumerCtx, cancelConsumer := context.WithCancel(context.Background())
	defer cancelConsumer()

	go func() {
		log.Info("Kafka consumer started for analytics-service on topic: ledger.entry_created")

		for {
			select {
			case <-consumerCtx.Done():
				log.Info("Kafka consumer stopped")
				return
			default:
				err := consumer.Consume(consumerCtx, func(ctx context.Context, key, value []byte) error {
					return service.ProcessKafkaEvent(ctx, value)
				})
				if err != nil {
					log.Errorf("Error consuming Kafka message: %v", err)
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

	// Start HTTP server in goroutine
	go func() {
		log.Infof("Analytics service starting on port %s", cfg.Service.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	// Stop background workers
	cancelConsumer()

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Info("Server exited")
}