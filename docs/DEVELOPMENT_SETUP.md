# 🛠️ Mercuria Development Setup Guide

Complete guide for setting up the Mercuria banking platform for development.

## 📋 Table of Contents

- [Prerequisites](#prerequisites)
- [Quick Setup](#quick-setup)
- [Manual Setup](#manual-setup)
- [Running Services](#running-services)
- [Development Workflow](#development-workflow)
- [Troubleshooting](#troubleshooting)
- [Advanced Configuration](#advanced-configuration)

## Prerequisites

### Required Software

1. **Docker & Docker Compose**

   ```bash
   # Check installation
   docker --version  # Should be 20.10+
   docker-compose --version  # Should be 1.29+
   ```

2. **Go 1.22+**

   ```bash
   # Check installation
   go version  # Should be go1.22 or higher
   ```

3. **Make** (optional but recommended)
   ```bash
   # Check installation
   make --version
   ```

### System Requirements

- **RAM**: 4GB minimum, 8GB recommended
- **Disk**: 10GB free space
- **OS**: Linux, macOS, or Windows with WSL2

## Quick Setup

### Option 1: Automated Setup (Recommended)

```bash
# 1. Clone repository
git clone https://github.com/kmassidik/mercuria-backend.git
cd mercuria-backend

# 2. Start infrastructure
docker-compose up -d

# 3. Run setup script (waits for services, creates DBs, runs migrations, etc.)
bash setup.sh

# 4. Start all services
make run-all
```

**Done!** All services are running. Continue to [Testing Your Setup](#testing-your-setup).

### Option 2: Using Makefile

```bash
# One command does everything
make setup

# Start services
make run-all
```

## Manual Setup

If you prefer step-by-step control:

### 1. Start Infrastructure Services

```bash
# Start PostgreSQL, Redis, Kafka, Zookeeper
docker-compose up -d

# Verify containers are running
docker-compose ps

# Expected output:
# mercuria-postgres    running    0.0.0.0:5432->5432/tcp
# mercuria-redis       running    0.0.0.0:6379->6379/tcp
# mercuria-kafka       running    0.0.0.0:9092->9092/tcp
# mercuria-zookeeper   running    0.0.0.0:2181->2181/tcp
```

### 2. Wait for Services to be Ready

```bash
# Wait for PostgreSQL (30-60 seconds)
docker exec mercuria-postgres pg_isready -U postgres

# Wait for Redis
docker exec mercuria-redis redis-cli ping

# Wait for Kafka (may take 60 seconds)
docker exec mercuria-kafka kafka-broker-api-versions --bootstrap-server localhost:9092
```

### 3. Create Databases

```bash
# Create databases for each service
docker exec mercuria-postgres psql -U postgres -c "CREATE DATABASE mercuria_auth;"
docker exec mercuria-postgres psql -U postgres -c "CREATE DATABASE mercuria_wallet;"
docker exec mercuria-postgres psql -U postgres -c "CREATE DATABASE mercuria_transaction;"
docker exec mercuria-postgres psql -U postgres -c "CREATE DATABASE mercuria_ledger;"
docker exec mercuria-postgres psql -U postgres -c "CREATE DATABASE mercuria_analytics;"

# Verify databases
docker exec mercuria-postgres psql -U postgres -l
```

### 4. Run Database Migrations

```bash
# Run all migrations
bash scripts/run_migrations.sh

# Or run individually per service:
docker exec -i mercuria-postgres psql -U postgres -d mercuria_auth < migrations/auth/001_create_users_table.sql
docker exec -i mercuria-postgres psql -U postgres -d mercuria_wallet < migrations/wallet/001_create_wallets_table.sql
# ... etc
```

### 5. Create Kafka Topics

```bash
# Create all required topics
bash scripts/create_kafka_topics.sh

# Or create individually:
docker exec mercuria-kafka kafka-topics --bootstrap-server localhost:9092 \
  --create --if-not-exists --topic wallet.created --partitions 3 --replication-factor 1

docker exec mercuria-kafka kafka-topics --bootstrap-server localhost:9092 \
  --create --if-not-exists --topic wallet.balance_updated --partitions 3 --replication-factor 1

docker exec mercuria-kafka kafka-topics --bootstrap-server localhost:9092 \
  --create --if-not-exists --topic transaction.completed --partitions 3 --replication-factor 1

docker exec mercuria-kafka kafka-topics --bootstrap-server localhost:9092 \
  --create --if-not-exists --topic ledger.entry_created --partitions 3 --replication-factor 1

# Verify topics
docker exec mercuria-kafka kafka-topics --bootstrap-server localhost:9092 --list
```

### 6. Generate mTLS Certificates (Optional)

```bash
# Generate certificates for service-to-service communication
bash scripts/generate-certs.sh

# This creates:
# certs/ca/ca.crt - Certificate Authority
# certs/auth/service.{crt,key} - Auth service certificate
# certs/wallet/service.{crt,key} - Wallet service certificate
# ... etc
```

### 7. Configure Environment

```bash
# Copy example environment file
cp example.env .env

# Edit .env with your configuration
nano .env

# Required variables:
# - JWT_SECRET (change in production!)
# - DB credentials
# - Service ports
# - Kafka brokers
```

### 8. Install Go Dependencies

```bash
# Download and install dependencies
go mod download
go mod tidy

# Verify
go list -m all
```

## Running Services

### Option 1: All Services at Once

```bash
# Using Makefile
make run-all

# This starts all 5 services in parallel
# Press Ctrl+C to stop all services
```

### Option 2: Individual Services

Open 5 terminal windows:

**Terminal 1 - Auth Service**

```bash
go run cmd/auth/main.go
# Starts on http://localhost:8080
```

**Terminal 2 - Wallet Service**

```bash
go run cmd/wallet/main.go
# Starts on http://localhost:8081
```

**Terminal 3 - Transaction Service**

```bash
go run cmd/transaction/main.go
# Starts on http://localhost:8082
```

**Terminal 4 - Ledger Service**

```bash
go run cmd/ledger/main.go
# Starts on http://localhost:8083
```

**Terminal 5 - Analytics Service**

```bash
go run cmd/analytics/main.go
# Starts on http://localhost:8084
```

### Option 3: Build and Run Binaries

```bash
# Build all services
make build

# Run built binaries
./bin/auth &
./bin/wallet &
./bin/transaction &
./bin/ledger &
./bin/analytics &
```

## Testing Your Setup

### 1. Check Health Endpoints

```bash
# Using Makefile
make health

# Or manually:
curl http://localhost:8080/health  # Auth
curl http://localhost:8081/health  # Wallet
curl http://localhost:8082/health  # Transaction
curl http://localhost:8083/health  # Ledger
curl http://localhost:8084/health  # Analytics
```

Expected response for each:

```json
{ "status": "healthy", "service": "auth", "timestamp": "2025-11-24T..." }
```

### 2. Register a Test User

```bash
curl -X POST http://localhost:8080/api/v1/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "testpass123",
    "first_name": "Test",
    "last_name": "User"
  }'
```

### 3. Login and Get JWT Token

```bash
curl -X POST http://localhost:8080/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "testpass123"
  }'
```

Save the `access_token` from the response.

### 4. Create a Wallet

```bash
export TOKEN="your_access_token_here"

curl -X POST http://localhost:8081/api/v1/wallets \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"currency": "USD"}'
```

### 5. Verify Event Flow

```bash
# Check Kafka consumer logs
# You should see "Kafka consumer started" in ledger and analytics services

# Deposit money to trigger events
curl -X POST http://localhost:8081/api/v1/wallets/{wallet_id}/deposit \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "amount": "100.00",
    "idempotency_key": "'"$(uuidgen)"'"
  }'

# Events should flow: Wallet → Transaction → Ledger → Analytics
```

## Development Workflow

### Running Tests

```bash
# Run all tests
make test

# Run specific service tests
make test-auth
make test-wallet

# Run with coverage
make test-coverage
```

### Code Formatting

```bash
# Format all Go code
make fmt

# Run linters
make lint
```

### Database Operations

```bash
# Access PostgreSQL shell
make db-shell

# Access specific database
docker exec -it mercuria-postgres psql -U postgres -d mercuria_wallet

# Common commands in psql:
\l                    # List databases
\c mercuria_wallet    # Connect to database
\dt                   # List tables
\d+ wallets           # Describe table
SELECT * FROM wallets;
```

### Redis Operations

```bash
# Access Redis shell
make redis-shell

# Common commands:
KEYS *                          # List all keys
GET wallet:balance:abc123       # Get specific key
DEL wallet:balance:abc123       # Delete key
FLUSHALL                        # Clear all data (be careful!)
```

### Kafka Operations

```bash
# List topics
make kafka-topics

# Consume messages from a topic
docker exec mercuria-kafka kafka-console-consumer \
  --bootstrap-server localhost:9092 \
  --topic wallet.created \
  --from-beginning

# Produce test message
docker exec -it mercuria-kafka kafka-console-producer \
  --bootstrap-server localhost:9092 \
  --topic wallet.created
```

## Troubleshooting

### Services Won't Start

**Problem**: PostgreSQL connection refused

```
Failed to connect to database: connection refused
```

**Solution**:

```bash
# Check if PostgreSQL is running
docker ps | grep postgres

# Check PostgreSQL logs
docker logs mercuria-postgres

# Restart PostgreSQL
docker-compose restart postgres

# Wait for it to be ready
docker exec mercuria-postgres pg_isready -U postgres
```

---

**Problem**: Kafka connection timeout

```
Failed to connect to Kafka: context deadline exceeded
```

**Solution**:

```bash
# Kafka takes 60+ seconds to start
docker logs mercuria-kafka

# Wait longer, then verify
docker exec mercuria-kafka kafka-broker-api-versions --bootstrap-server localhost:9092

# If still failing, restart Kafka
docker-compose restart kafka zookeeper
```

---

**Problem**: Port already in use

```
bind: address already in use
```

**Solution**:

```bash
# Find process using port (example: 8080)
lsof -i :8080

# Kill process
kill -9 <PID>

# Or change port in .env file
AUTH_PORT=8090
```

### Database Issues

**Problem**: Migration fails

```
relation "users" already exists
```

**Solution**:

```bash
# Drop and recreate database
docker exec mercuria-postgres psql -U postgres -c "DROP DATABASE mercuria_auth;"
docker exec mercuria-postgres psql -U postgres -c "CREATE DATABASE mercuria_auth;"

# Run migrations again
bash scripts/run_migrations.sh
```

---

**Problem**: Cannot connect to specific database

```
database "mercuria_wallet" does not exist
```

**Solution**:

```bash
# List databases
docker exec mercuria-postgres psql -U postgres -l

# Create missing database
docker exec mercuria-postgres psql -U postgres -c "CREATE DATABASE mercuria_wallet;"
```

### Kafka Issues

**Problem**: Topic doesn't exist

```
UNKNOWN_TOPIC_OR_PARTITION
```

**Solution**:

```bash
# List topics
docker exec mercuria-kafka kafka-topics --bootstrap-server localhost:9092 --list

# Create missing topic
bash scripts/create_kafka_topics.sh
```

### Clean Reset

If everything is broken:

```bash
# Stop and remove everything
docker-compose down -v

# Remove generated certificates
rm -rf certs/

# Start fresh
make setup
```

## Advanced Configuration

### Enabling mTLS

1. Generate certificates:

```bash
bash scripts/generate-certs.sh
```

2. Update `.env`:

```bash
MTLS_ENABLED=true
MTLS_CA_CERT=./certs/ca/ca.crt

# Auth service
MTLS_SERVER_CERT=./certs/auth/service.crt
MTLS_SERVER_KEY=./certs/auth/service.key

# Wallet service
MTLS_SERVER_CERT=./certs/wallet/service.crt
MTLS_SERVER_KEY=./certs/wallet/service.key
# ... etc
```

3. Restart services

### Custom Ports

Edit `.env`:

```bash
AUTH_PORT=9080
WALLET_PORT=9081
TRANSACTION_PORT=9082
LEDGER_PORT=9083
ANALYTICS_PORT=9084
```

### Production Environment

```bash
# Set production mode
ENV=production

# Use strong JWT secret
JWT_SECRET=$(openssl rand -base64 32)

# Enable mTLS
MTLS_ENABLED=true

# Use production database
DB_HOST=production-db.example.com
DB_PASSWORD=$(openssl rand -base64 24)

# Enable rate limiting (future)
RATE_LIMIT_ENABLED=true
```

## Useful Commands

```bash
# View all running services
docker-compose ps

# View logs
docker-compose logs -f                 # All services
docker-compose logs -f postgres        # Specific service

# Restart service
docker-compose restart postgres

# Stop all services
docker-compose down

# Stop and remove volumes (clean slate)
docker-compose down -v

# Check disk usage
docker system df

# Clean up Docker
docker system prune -a
```

## Next Steps

1. **Read the API Documentation**: [docs/manual-api-docs.md](docs/manual-api-docs.md)
2. **Understand the Architecture**: [Mercuria_Backend_PRD_v4.md](Mercuria_Backend_PRD_v4.md)
3. **Write Tests**: See existing tests in `internal/*/` directories
4. **Deploy to Kubernetes**: (Coming soon)

## Getting Help

- **GitHub Issues**: Report bugs or request features
- **Documentation**: Check `docs/` directory
- **Code Examples**: See `tests/integration/` for examples

---

Happy coding! 🚀
