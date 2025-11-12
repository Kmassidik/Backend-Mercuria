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
	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/db"
	"github.com/kmassidik/mercuria/internal/common/kafka"
	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/internal/common/middleware"
	"github.com/kmassidik/mercuria/internal/common/redis"
	"github.com/kmassidik/mercuria/internal/transaction"
	"github.com/kmassidik/mercuria/pkg/outbox"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found, using system environment variables")
	}

	// Load configuration
	cfg, err := config.Load("transaction")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New("transaction-service")

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

	// Initialize repositories
	txnRepo := transaction.NewRepository(database, log)
	outboxRepo := outbox.NewRepository(database.DB, log)

	// Initialize service
	service := transaction.NewService(txnRepo, outboxRepo, redisClient, producer, database, log)

	// Initialize handler
	handler := transaction.NewHandler(service, log)

	// Create HTTP server
	mux := http.NewServeMux()

	// Apply middleware
	var httpHandler http.Handler = mux
	httpHandler = middleware.CORS(httpHandler)
	httpHandler = middleware.Logging(log)(httpHandler)
	httpHandler = middleware.Recovery(log)(httpHandler)

	// Register routes
	handler.RegisterRoutes(mux, cfg.JWT.Secret)

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	// Start outbox publisher (background worker)
	// NOTE: This publishes pending events to Kafka
	outboxPublisher := outbox.NewPublisher(outboxRepo, producer, log, 5*time.Second)
	publisherCtx, cancelPublisher := context.WithCancel(context.Background())
	defer cancelPublisher()

	go outboxPublisher.Start(publisherCtx)
	log.Info("Outbox publisher started")

	// Start scheduled transfer worker (background worker)
	// NOTE: This processes scheduled transfers that are due
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-publisherCtx.Done():
				log.Info("Scheduled transfer worker stopped")
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
				processed, err := service.ProcessScheduledTransfers(ctx)
				if err != nil {
					log.Errorf("Failed to process scheduled transfers: %v", err)
				} else if processed > 0 {
					log.Infof("Processed %d scheduled transfers", processed)
				}
				cancel()
			}
		}
	}()
	log.Info("Scheduled transfer worker started")

	server := &http.Server{
		Addr:         ":" + cfg.Service.Port,
		Handler:      httpHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Infof("Transaction service starting on port %s", cfg.Service.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")

	// Cancel background workers
	cancelPublisher()

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Info("Server exited")
}