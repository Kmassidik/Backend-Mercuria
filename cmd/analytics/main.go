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
	"github.com/kmassidik/mercuria/internal/common/mtls"
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

	// Load mTLS configuration
	mtlsConfig := mtls.LoadFromEnv()
	if mtlsConfig.Enabled {
		log.Info("🔐 mTLS is ENABLED for internal service communication")
	} else {
		log.Info("⚠️  mTLS is DISABLED - using HTTP only")
	}

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
	consumer := kafka.NewConsumer(cfg.Kafka, "ledger.entry_created", log)
	defer consumer.Close()

	// Verify Kafka is reachable
	log.Info("Checking Kafka connection...")
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

	// =============================================================
	// PUBLIC SERVER - Port 8084 (HTTPS + JWT for external clients)
	// =============================================================
	publicMux := http.NewServeMux()

	// Apply middleware to public router
	var publicHandler http.Handler = publicMux
	publicHandler = middleware.CORS(publicHandler)
	publicHandler = middleware.Logging(log)(publicHandler)
	publicHandler = middleware.Recovery(log)(publicHandler)

	// Register routes with JWT protection
	analytics.SetupRoutes(publicMux, handler, cfg.JWT.Secret)

	publicPort := cfg.Service.Port // Default: 8084
	publicServer := &http.Server{
		Addr:         ":" + publicPort,
		Handler:      publicHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start public server
	go func() {
		log.Infof("🌐 Public API starting on port %s (HTTPS + JWT)", publicPort)
		if err := publicServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start public server: %v", err)
		}
	}()

	// =============================================================
	// INTERNAL SERVER - Port 9084 (mTLS for service-to-service)
	// =============================================================
	if mtlsConfig.Enabled {
		internalMux := http.NewServeMux()

		// Apply middleware to internal router (no JWT needed)
		var internalHandler http.Handler = internalMux
		internalHandler = middleware.Logging(log)(internalHandler)
		internalHandler = middleware.Recovery(log)(internalHandler)

		// Register internal routes (no JWT middleware)
		analytics.SetupInternalRoutes(internalMux, handler)

		internalPort := os.Getenv("ANALYTICS_INTERNAL_PORT")
		if internalPort == "" {
			internalPort = "9084" // Default internal port
		}

		tlsConfig, err := mtlsConfig.ServerTLSConfig()
		if err != nil {
			log.Fatalf("Failed to load mTLS config: %v", err)
		}

		internalServer := &http.Server{
			Addr:         ":" + internalPort,
			Handler:      internalHandler,
			TLSConfig:    tlsConfig,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		}

		go func() {
			log.Infof("🔐 Internal API starting on port %s (mTLS)", internalPort)
			if err := internalServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Failed to start internal server: %v", err)
			}
		}()

		// Add internal server to shutdown list
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			internalServer.Shutdown(shutdownCtx)
		}()
	}

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

	// =============================================================
	// GRACEFUL SHUTDOWN
	// =============================================================
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("🛑 Shutting down servers...")

	// Stop background workers
	cancelConsumer()

	// Shutdown public server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := publicServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Public server forced to shutdown: %v", err)
	}

	log.Info("✅ All servers exited gracefully")
}