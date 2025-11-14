package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Service  ServiceConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Kafka    KafkaConfig
	JWT      JWTConfig
}

type ServiceConfig struct {
	Name        string
	Port        string
	Environment string // dev, staging, production
}

type DatabaseConfig struct {
	Host            string
	Port            string
	User            string
	Password        string
	DBName          string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type KafkaConfig struct {
	Brokers []string
	GroupID string
}

type JWTConfig struct {
	Secret           string
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration
}

// getDefaultPort returns the default port for each service according to PRD
func getDefaultPort(serviceName string) string {
	defaultPorts := map[string]string{
		"auth":        "8080",
		"wallet":      "8081",
		"transaction": "8082",
		"ledger":      "8083",
		"analytics":   "8084",
	}
	
	if port, exists := defaultPorts[serviceName]; exists {
		return port
	}
	return "8080" // fallback
}

func Load(serviceName string) (*Config, error) {
	
	servicePortEnv := fmt.Sprintf("%s_PORT", strings.ToUpper(serviceName))
	defaultPort := getDefaultPort(serviceName)
	
	cfg := &Config{
		Service: ServiceConfig{
			Name:        serviceName,
			Port:        getEnv(servicePortEnv, getEnv("PORT", defaultPort)),
			Environment: getEnv("ENV", "dev"),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnv("DB_PORT", "5432"),
			User:            getEnv("DB_USER", "postgres"),
			Password:        getEnv("DB_PASSWORD", "postgres"),
			DBName: 		 getEnv("DB_NAME", fmt.Sprintf("mercuria_%s", serviceName)),
			MaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvAsDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
		},
		Kafka: KafkaConfig{
			Brokers: []string{getEnv("KAFKA_BROKERS", "localhost:9092")},
			GroupID: fmt.Sprintf("%s-group", serviceName),
		},
		JWT: JWTConfig{
			Secret:          getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
			AccessTokenTTL:  getEnvAsDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvAsDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
	}

	// Validation for production
	if cfg.Service.Environment == "production" {
		if cfg.JWT.Secret == "your-secret-key-change-in-production" {
			return nil, fmt.Errorf("JWT_SECRET must be set in production")
		}
		if cfg.Database.Password == "postgres" {
			return nil, fmt.Errorf("DB_PASSWORD must be set in production")
		}
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}