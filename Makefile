.PHONY: help setup clean start stop restart logs test build run-auth run-wallet run-transaction run-ledger run-analytics run-all

# Default target
help:
	@echo "╔═══════════════════════════════════════════════════════╗"
	@echo "║     🚀 Mercuria Backend - Development Commands 🚀    ║"
	@echo "╚═══════════════════════════════════════════════════════╝"
	@echo ""
	@echo "📦 Setup Commands:"
	@echo "  make setup           - Complete first-time setup"
	@echo "  make clean           - Clean all containers and volumes"
	@echo "  make reset           - Clean + fresh setup"
	@echo ""
	@echo "🐳 Docker Commands:"
	@echo "  make start           - Start Docker containers"
	@echo "  make stop            - Stop Docker containers"
	@echo "  make restart         - Restart Docker containers"
	@echo "  make logs            - View Docker logs"
	@echo "  make ps              - Show container status"
	@echo ""
	@echo "🏃 Service Commands:"
	@echo "  make run-auth        - Run Auth service"
	@echo "  make run-wallet      - Run Wallet service"
	@echo "  make run-transaction - Run Transaction service"
	@echo "  make run-ledger      - Run Ledger service"
	@echo "  make run-analytics   - Run Analytics service"
	@echo "  make run-all         - Run all services (parallel)"
	@echo ""
	@echo "🧪 Testing Commands:"
	@echo "  make test            - Run all tests"
	@echo "  make test-auth       - Test Auth service"
	@echo "  make test-wallet     - Test Wallet service"
	@echo "  make test-transaction- Test Transaction service"
	@echo "  make test-ledger     - Test Ledger service"
	@echo "  make test-analytics  - Test Analytics service"
	@echo "  make test-coverage   - Run tests with coverage"
	@echo ""
	@echo "🔨 Build Commands:"
	@echo "  make build           - Build all services"
	@echo "  make build-auth      - Build Auth service"
	@echo "  make build-wallet    - Build Wallet service"
	@echo ""
	@echo "🧹 Maintenance Commands:"
	@echo "  make fmt             - Format Go code"
	@echo "  make lint            - Run linters"
	@echo "  make tidy            - Tidy Go modules"
	@echo "  make migrate         - Run database migrations"
	@echo ""

# ============================================
# Setup & Installation
# ============================================

setup:
	@echo "🚀 Starting Mercuria setup..."
	@docker-compose up -d
	@sleep 5
	@bash setup.sh

clean:
	@echo "🧹 Cleaning up..."
	@docker-compose down -v
	@rm -rf certs/
	@echo "✅ Cleanup complete"

reset: clean
	@echo "♻️  Resetting environment..."
	@make setup

# ============================================
# Docker Management
# ============================================

start:
	@echo "▶️  Starting Docker containers..."
	@docker-compose up -d
	@echo "✅ Containers started"

stop:
	@echo "⏸️  Stopping Docker containers..."
	@docker-compose down
	@echo "✅ Containers stopped"

restart: stop start

logs:
	@docker-compose logs -f

ps:
	@docker-compose ps

# ============================================
# Service Execution
# ============================================

run-auth:
	@echo "🔐 Starting Auth Service on port 8080..."
	@go run cmd/auth/main.go

run-wallet:
	@echo "💰 Starting Wallet Service on port 8081..."
	@go run cmd/wallet/main.go

run-transaction:
	@echo "💸 Starting Transaction Service on port 8082..."
	@go run cmd/transaction/main.go

run-ledger:
	@echo "📒 Starting Ledger Service on port 8083..."
	@go run cmd/ledger/main.go

run-analytics:
	@echo "📊 Starting Analytics Service on port 8084..."
	@go run cmd/analytics/main.go

run-all:
	@echo "🚀 Starting all services..."
	@trap 'kill 0' EXIT; \
	go run cmd/auth/main.go & \
	go run cmd/wallet/main.go & \
	go run cmd/transaction/main.go & \
	go run cmd/ledger/main.go & \
	go run cmd/analytics/main.go & \
	wait

# ============================================
# Testing
# ============================================

test:
	@echo "🧪 Running all tests..."
	@go test ./... -v

test-auth:
	@echo "🧪 Testing Auth service..."
	@go test ./internal/auth/... -v

test-wallet:
	@echo "🧪 Testing Wallet service..."
	@go test ./internal/wallet/... -v

test-transaction:
	@echo "🧪 Testing Transaction service..."
	@go test ./internal/transaction/... -v

test-ledger:
	@echo "🧪 Testing Ledger service..."
	@go test ./internal/ledger/... -v

test-analytics:
	@echo "🧪 Testing Analytics service..."
	@go test ./internal/analytics/... -v

test-coverage:
	@echo "📊 Running tests with coverage..."
	@go test ./... -coverprofile=coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: coverage.html"

# ============================================
# Build
# ============================================

build:
	@echo "🔨 Building all services..."
	@go build -o bin/auth cmd/auth/main.go
	@go build -o bin/wallet cmd/wallet/main.go
	@go build -o bin/transaction cmd/transaction/main.go
	@go build -o bin/ledger cmd/ledger/main.go
	@go build -o bin/analytics cmd/analytics/main.go
	@echo "✅ Build complete: bin/"

build-auth:
	@go build -o bin/auth cmd/auth/main.go

build-wallet:
	@go build -o bin/wallet cmd/wallet/main.go

build-transaction:
	@go build -o bin/transaction cmd/transaction/main.go

build-ledger:
	@go build -o bin/ledger cmd/ledger/main.go

build-analytics:
	@go build -o bin/analytics cmd/analytics/main.go

# ============================================
# Code Quality
# ============================================

fmt:
	@echo "📝 Formatting code..."
	@go fmt ./...
	@echo "✅ Code formatted"

lint:
	@echo "🔍 Running linters..."
	@golangci-lint run --timeout=5m || echo "⚠️  Install golangci-lint: https://golangci-lint.run/usage/install/"

tidy:
	@echo "📦 Tidying Go modules..."
	@go mod tidy
	@echo "✅ Modules tidied"

# ============================================
# Database
# ============================================

migrate:
	@echo "🔄 Running migrations..."
	@bash scripts/run_migrations.sh

migrate-create:
	@read -p "Enter migration name: " name; \
	bash scripts/create_migration.sh $$name

# ============================================
# Utilities
# ============================================

health:
	@echo "🏥 Checking service health..."
	@echo "Auth:        $$(curl -s http://localhost:8080/health || echo 'Down')"
	@echo "Wallet:      $$(curl -s http://localhost:8081/health || echo 'Down')"
	@echo "Transaction: $$(curl -s http://localhost:8082/health || echo 'Down')"
	@echo "Ledger:      $$(curl -s http://localhost:8083/health || echo 'Down')"
	@echo "Analytics:   $$(curl -s http://localhost:8084/health || echo 'Down')"

db-shell:
	@docker exec -it mercuria-postgres psql -U postgres

redis-shell:
	@docker exec -it mercuria-redis redis-cli

kafka-shell:
	@docker exec -it mercuria-kafka bash

kafka-topics:
	@docker exec mercuria-kafka kafka-topics --bootstrap-server localhost:9092 --list

kafka-consume:
	@read -p "Enter topic name: " topic; \
	docker exec mercuria-kafka kafka-console-consumer --bootstrap-server localhost:9092 --topic $$topic --from-beginning
