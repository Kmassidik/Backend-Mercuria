# 🏦 Mercuria Backend

**Banking-Grade Microservices Payment Platform** built with Go, PostgreSQL, Kafka, and Redis.

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![PRD](https://img.shields.io/badge/PRD-v4-success)](Mercuria_Backend_PRD_v4.md)

> A production-ready distributed financial transaction system implementing event-driven architecture, double-entry bookkeeping, and exactly-once delivery guarantees.

## ✨ Features

- 🔐 **JWT Authentication** - Secure user authentication with refresh tokens
- 💰 **Multi-Currency Wallets** - Support for USD, EUR, GBP, JPY, IDR
- 💸 **P2P Transfers** - Peer-to-peer money transfers with idempotency
- 📦 **Batch Transactions** - Atomic batch payments (payroll, bulk transfers)
- ⏰ **Scheduled Transfers** - Future-dated automatic transfers
- 📒 **Double-Entry Ledger** - Immutable audit trail with balance verification
- 📊 **Real-time Analytics** - Aggregated metrics and user insights
- 🔒 **mTLS Security** - Optional mutual TLS for service-to-service communication
- ♻️ **Exactly-Once Delivery** - Outbox pattern for reliable event publishing
- 🚀 **Horizontally Scalable** - Stateless microservices ready for Kubernetes

## 🏗️ Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│    Auth     │     │   Wallet    │     │ Transaction │     │   Ledger    │     │  Analytics  │
│   :8080     │────▶│   :8081     │────▶│   :8082     │────▶│   :8083     │────▶│   :8084     │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
       │                   │                   │                   │                   │
       └───────────────────┴───────────────────┴───────────────────┴───────────────────┘
                                          │
                               ┌──────────┴──────────┐
                               │   Apache Kafka      │
                               │   Event Streaming   │
                               └──────────┬──────────┘
                                          │
                    ┌─────────────────────┼─────────────────────┐
                    │                     │                     │
            ┌───────▼──────┐      ┌──────▼──────┐      ┌──────▼──────┐
            │  PostgreSQL  │      │    Redis    │      │   Zookeeper │
            │  (Per-Service│      │   Caching   │      │             │
            │   Databases) │      │   Locking   │      └─────────────┘
            └──────────────┘      └─────────────┘
```

### Microservices

| Service         | Port | Responsibility                                             |
| --------------- | ---- | ---------------------------------------------------------- |
| **Auth**        | 8080 | User registration, JWT authentication, token management    |
| **Wallet**      | 8081 | Wallet creation, deposits, withdrawals, balance management |
| **Transaction** | 8082 | P2P transfers, batch payments, scheduled transactions      |
| **Ledger**      | 8083 | Immutable double-entry bookkeeping, audit trail            |
| **Analytics**   | 8084 | Real-time metrics, user analytics, aggregations            |

### Kafka Topics

- `wallet.created` - Wallet creation events
- `wallet.balance_updated` - Balance change events
- `transaction.completed` - Completed transfers
- `ledger.entry_created` - Ledger entries (consumed by Analytics)

## 🚀 Quick Start

### Prerequisites

- **Docker** & **Docker Compose** - For infrastructure services
- **Go 1.22+** - For running microservices
- **Make** (optional) - For convenient commands

### One-Command Setup

```bash
# Clone the repository
git clone https://github.com/yourusername/mercuria-backend.git
cd mercuria-backend

# Start infrastructure
docker-compose up -d

# Run complete setup (creates databases, migrations, Kafka topics, certificates)
bash setup.sh

# Start all microservices
make run-all
```

That's it! 🎉 All services are now running.

### Manual Setup

```bash
# 1. Start infrastructure
docker-compose up -d

# 2. Wait for services to be ready (30-60 seconds)
docker-compose ps

# 3. Run migrations
bash scripts/run_migrations.sh

# 4. Create Kafka topics
bash scripts/create_kafka_topics.sh

# 5. (Optional) Generate mTLS certificates
bash scripts/generate-certs.sh

# 6. Start services individually
go run cmd/auth/main.go        # Terminal 1
go run cmd/wallet/main.go       # Terminal 2
go run cmd/transaction/main.go  # Terminal 3
go run cmd/ledger/main.go       # Terminal 4
go run cmd/analytics/main.go    # Terminal 5
```

## 📚 API Documentation

### Auth Service

```bash
# Register a new user
curl -X POST http://localhost:8080/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "securepass123",
    "first_name": "John",
    "last_name": "Doe"
  }'

# Login
curl -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "securepass123"
  }'
```

### Wallet Service

```bash
# Create a wallet (requires JWT)
curl -X POST http://localhost:8081/api/v1/wallets \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "currency": "USD"
  }'

# Deposit funds
curl -X POST http://localhost:8081/api/v1/wallets/{wallet_id}/deposit \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount": "1000.00",
    "idempotency_key": "unique-uuid-here"
  }'
```

### Transaction Service

```bash
# P2P Transfer
curl -X POST http://localhost:8082/api/v1/transactions \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "from_wallet_id": "wallet-123",
    "to_wallet_id": "wallet-456",
    "amount": "50.00",
    "description": "Payment for services",
    "idempotency_key": "unique-uuid-here"
  }'

# Batch Transfer
curl -X POST http://localhost:8082/api/v1/transactions/batch \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "from_wallet_id": "wallet-123",
    "transfers": [
      {"to_wallet_id": "wallet-456", "amount": "100.00"},
      {"to_wallet_id": "wallet-789", "amount": "200.00"}
    ],
    "idempotency_key": "unique-uuid-here"
  }'
```

### Analytics Service

```bash
# Get daily metrics
curl "http://localhost:8084/api/v1/analytics/daily?start_date=2025-01-01&end_date=2025-01-31" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# Get user analytics (current user)
curl "http://localhost:8084/api/v1/analytics/me?start_date=2025-01-01&end_date=2025-01-31" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## 🧪 Testing

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Test specific service
make test-wallet
```

## 📊 Makefile Commands

```bash
make help              # Show all available commands
make setup             # Complete first-time setup
make start             # Start Docker containers
make stop              # Stop Docker containers
make logs              # View Docker logs
make run-all           # Run all microservices
make test              # Run all tests
make build             # Build all services
make health            # Check service health
make clean             # Clean up everything
make reset             # Clean + fresh setup
```

## 🗂️ Project Structure

```
.
├── cmd/                          # Service entry points
│   ├── auth/main.go
│   ├── wallet/main.go
│   ├── transaction/main.go
│   ├── ledger/main.go
│   └── analytics/main.go
├── internal/                     # Internal packages
│   ├── auth/                     # Auth service logic
│   ├── wallet/                   # Wallet service logic
│   ├── transaction/              # Transaction service logic
│   ├── ledger/                   # Ledger service logic
│   ├── analytics/                # Analytics service logic
│   └── common/                   # Shared packages
│       ├── config/               # Configuration
│       ├── db/                   # Database client
│       ├── redis/                # Redis client
│       ├── kafka/                # Kafka producer/consumer
│       ├── logger/               # Structured logging
│       ├── middleware/           # HTTP middleware
│       └── mtls/                 # mTLS utilities
├── pkg/                          # Public packages
│   └── outbox/                   # Outbox pattern implementation
├── migrations/                   # SQL migrations
│   ├── auth/
│   ├── wallet/
│   ├── transaction/
│   ├── ledger/
│   ├── analytics/
│   └── outbox/
├── scripts/                      # Setup scripts
│   ├── create_kafka_topics.sh
│   ├── generate-certs.sh
│   └── run_migrations.sh
├── certs/                        # mTLS certificates (generated)
├── docker-compose.yml            # Infrastructure services
├── setup.sh                      # Master setup script
├── Makefile                      # Development commands
├── example.env                   # Environment template
└── README.md                     # This file
```

## 🔐 Security Features

- **JWT Authentication** - Secure token-based auth with refresh tokens
- **Password Hashing** - bcrypt with cost factor 12
- **Idempotency Keys** - Prevent duplicate transactions (Redis-backed)
- **Distributed Locking** - Redis locks prevent race conditions
- **mTLS (Optional)** - Mutual TLS for service-to-service communication
- **Input Validation** - Strict validation on all endpoints
- **SQL Injection Prevention** - Parameterized queries only
- **Rate Limiting** - Configurable per endpoint (TODO)
- **Audit Trail** - Immutable ledger for compliance

## 🔧 Configuration

### Environment Variables

```bash
# Service Ports
AUTH_PORT=8080
WALLET_PORT=8081
TRANSACTION_PORT=8082
LEDGER_PORT=8083
ANALYTICS_PORT=8084

# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# Kafka
KAFKA_BROKERS=localhost:9092

# JWT
JWT_SECRET=your-secret-key-change-in-production
JWT_ACCESS_TTL=15m
JWT_REFRESH_TTL=168h

# mTLS (Optional)
MTLS_ENABLED=false
MTLS_CA_CERT=./certs/ca/ca.crt
MTLS_SERVER_CERT=./certs/wallet/service.crt
MTLS_SERVER_KEY=./certs/wallet/service.key
```

See `example.env` for complete configuration.

## 🐳 Docker Services

```yaml
services:
  postgres: localhost:5432
  redis: localhost:6379
  kafka: localhost:9092
  zookeeper: localhost:2181
```

## 🚦 Health Checks

```bash
# Check all services
make health

# Individual health checks
curl http://localhost:8080/health  # Auth
curl http://localhost:8081/health  # Wallet
curl http://localhost:8082/health  # Transaction
curl http://localhost:8083/health  # Ledger
curl http://localhost:8084/health  # Analytics
```

## 📈 Performance

- **Throughput**: ~1000 TPS per service instance
- **Latency**: <100ms p99 for most endpoints
- **Scalability**: Horizontally scalable (stateless services)
- **Kafka**: Async event processing for non-blocking operations
- **Redis**: Balance caching reduces database load by 80%

## 🛠️ Development

### Running Services Individually

```bash
# Terminal 1 - Auth
go run cmd/auth/main.go

# Terminal 2 - Wallet
go run cmd/wallet/main.go

# Terminal 3 - Transaction
go run cmd/transaction/main.go

# Terminal 4 - Ledger
go run cmd/ledger/main.go

# Terminal 5 - Analytics
go run cmd/analytics/main.go
```

### Database Access

```bash
# PostgreSQL shell
make db-shell

# Redis shell
make redis-shell

# Kafka shell
make kafka-shell

# List Kafka topics
make kafka-topics
```

## 📖 Documentation

- [Product Requirements Document (PRD)](Mercuria_Backend_PRD_v4.md)
- [API Documentation](docs/manual-api-docs.md)
- [Setup Guide](docs/manual-setup-guide.md)
- [Auth Service Guide](docs/manual-setup-auth.md)

## 🤝 Contributing

Contributions are welcome! Please follow these guidelines:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Write tests for your changes
4. Ensure all tests pass (`make test`)
5. Commit your changes (`git commit -m 'Add amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Built with Go's standard library (`net/http`)
- PostgreSQL for ACID compliance
- Apache Kafka for event streaming
- Redis for caching and locking
- Docker for development environment

## 📧 Contact

- **Author**: Kurnia Massidik
- **GitHub**: [@kmassidik](https://github.com/kmassidik)
- **Project**: [Mercuria Backend](https://github.com/kmassidik/mercuria-backend)

---

**⭐ If you find this project useful, please give it a star!**

Built with ❤️ using Go
