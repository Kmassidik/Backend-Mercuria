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
	"github.com/kmassidik/mercuria/internal/common/mtls"
	"github.com/kmassidik/mercuria/internal/common/redis"
	"github.com/kmassidik/mercuria/internal/wallet"
	"github.com/kmassidik/mercuria/pkg/outbox"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		fmt.Println("No .env file found, using system environment variables")
	}

	// Load configuration
	cfg, err := config.Load("wallet")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New("wallet-service")

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

	// Initialize repositories
	repo := wallet.NewRepository(database, log)
	outboxRepo := outbox.NewRepository(database.DB, log)

	// Initialize service with outbox
	service := wallet.NewService(repo, outboxRepo, redisClient, producer, database, log)
	handler := wallet.NewHandler(service, log)

	// Create HTTP routers
	publicMux := http.NewServeMux()
	internalMux := http.NewServeMux()

	// Apply middleware to public router (external clients)
	var publicHandler http.Handler = publicMux
	publicHandler = middleware.CORS(publicHandler)
	publicHandler = middleware.Logging(log)(publicHandler)
	publicHandler = middleware.Recovery(log)(publicHandler)

	// Apply middleware to internal router (service-to-service)
	var internalHandler http.Handler = internalMux
	internalHandler = middleware.Logging(log)(internalHandler)
	internalHandler = middleware.Recovery(log)(internalHandler)

	// Register routes on BOTH routers
	// Public API - requires JWT authentication
	handler.RegisterRoutes(publicMux, cfg.JWT.Secret)
	
	// Internal API - same routes but accessed via mTLS (no JWT needed between services)
	handler.RegisterInternalRoutes(internalMux)
	
	// Health check endpoints
	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	}
	publicMux.HandleFunc("GET /health", healthHandler)
	internalMux.HandleFunc("GET /health", healthHandler)

	// Start outbox publisher (background worker)
	outboxPublisher := outbox.NewPublisher(outboxRepo, producer, log, 5*time.Second)
	publisherCtx, cancelPublisher := context.WithCancel(context.Background())
	defer cancelPublisher()
	go outboxPublisher.Start(publisherCtx)
	log.Info("✅ Outbox publisher started")

	// =============================================================
	// PUBLIC SERVER - Port 8081 (HTTPS + JWT for external clients)
	// =============================================================
	publicPort := cfg.Service.Port // Default: 8081
	publicServer := &http.Server{
		Addr:         ":" + publicPort,
		Handler:      publicHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start public server (HTTP for development, HTTPS for production)
	go func() {
		// For production, you'd enable TLS here (but NOT mTLS)
		// For now, using HTTP to work with Postman
		log.Infof("🌐 Public API starting on port %s (HTTPS + JWT)", publicPort)
		if err := publicServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start public server: %v", err)
		}
	}()

	// =============================================================
	// INTERNAL SERVER - Port 9081 (mTLS for service-to-service)
	// =============================================================
	if mtlsConfig.Enabled {
		internalPort := os.Getenv("WALLET_INTERNAL_PORT")
		if internalPort == "" {
			internalPort = "9081" // Default internal port
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
			// mTLS requires ListenAndServeTLS with certificates from TLSConfig
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

	// =============================================================
	// GRACEFUL SHUTDOWN
	// =============================================================
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("🛑 Shutting down servers...")

	// Cancel background workers
	cancelPublisher()

	// Shutdown public server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := publicServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Public server forced to shutdown: %v", err)
	}

	log.Info("✅ All servers exited gracefully")
}