# 🚀 Mercuria Backend - Complete Build Guide

I'll guide you through building this event-driven microservices platform step-by-step. We'll follow **Test-Driven Development (TDD)** and build each component incrementally.

---

## 📋 **Build Roadmap Overview**

Here's what we'll build together:

### **Phase 1: Foundation** (Infrastructure & Shared Components)

1. Project structure setup
2. Common utilities (logger, config, DB, Redis, Kafka)
3. Docker Compose infrastructure

### **Phase 2: Core Services** (One service at a time, TDD approach)

4. **Auth Service** - User registration, login, JWT
5. **Wallet Service** - Wallet management, balance operations
6. **Transaction Service** - Transfer orchestration
7. **Ledger Service** - Immutable audit trail
8. **Analytics Service** - Metrics aggregation

### **Phase 3: Integration & Deployment**

9. Service-to-service communication via Kafka
10. End-to-end testing
11. Monitoring & observability

---

## 🎯 **Step-by-Step Breakdown**

Let me break down what each step involves:

### **Step 1: Project Foundation** (~1 hour)

**What we'll build:**

- Go module initialization
- Folder structure (cmd, internal, pkg)
- Configuration management
- Logger setup
- Database connection utilities
- Redis client wrapper
- Kafka producer/consumer abstractions

**Why it matters:** This is the reusable foundation all services will share.

---

### **Step 2: Auth Service** (~2-3 hours)

**What we'll build:**

```
Tests First (TDD):
├─ User registration validation tests
├─ Password hashing tests
├─ JWT generation/validation tests
└─ Refresh token rotation tests

Then Implementation:
├─ POST /api/v1/register
├─ POST /api/v1/login
├─ POST /api/v1/refresh
├─ GET /api/v1/me
├─ PostgreSQL schema (users, refresh_tokens)
└─ JWT middleware for authentication
```

**Flow Example:**

```
User registers → Hash password → Store in DB → Return JWT + refresh token
User logs in → Validate credentials → Generate new tokens
Protected routes → Validate JWT → Allow access
```

---

### **Step 3: Wallet Service** (~3-4 hours)

**What we'll build:**

```
Tests First:
├─ Wallet creation tests
├─ Balance update tests with Redis locks
├─ Deposit/withdrawal validation tests
└─ Event publishing tests

Implementation:
├─ POST /api/v1/wallets (create)
├─ GET /api/v1/wallets/:id
├─ POST /api/v1/wallets/:id/deposit
├─ POST /api/v1/wallets/:id/withdraw
├─ Redis integration (balance cache, wallet locks)
├─ Kafka event publishing (wallet.created, wallet.balance_updated)
└─ PostgreSQL schema (wallets, wallet_events)
```

**Critical Features:**

- **Redis locks** prevent double-spending
- **Balance caching** improves read performance
- **Outbox pattern** ensures reliable event publishing

---

### **Step 4: Transaction Service** (~3-4 hours)

**What we'll build:**

```
Tests First:
├─ Transaction validation tests
├─ Idempotency tests
├─ Balance verification tests
└─ Event consumption tests

Implementation:
├─ POST /api/v1/transactions (create transfer)
├─ GET /api/v1/transactions/:id
├─ Idempotency key handling
├─ Wallet balance validation
├─ Kafka event publishing (transaction.completed)
├─ Kafka event consumption (wallet.balance_updated)
└─ PostgreSQL schema (transactions, outbox_events)
```

**Transaction Flow:**

```
1. Validate sender has sufficient balance
2. Lock both wallets (Redis)
3. Deduct from sender, add to receiver
4. Publish transaction.completed event
5. Release locks
```

---

### **Step 5: Ledger Service** (~2 hours)

**What we'll build:**

```
Tests First:
├─ Ledger entry immutability tests
├─ Event consumption tests
└─ Query/audit tests

Implementation:
├─ GET /api/v1/ledger
├─ GET /api/v1/ledger/:tx_id
├─ Kafka consumer (transaction.completed)
├─ Immutable ledger entries
└─ PostgreSQL schema (ledger_entries, ledger_outbox)
```

**Purpose:** Every financial operation creates an **immutable audit trail** that can never be modified or deleted.

---

### **Step 6: Analytics Service** (~2 hours)

**What we'll build:**

```
Tests First:
├─ Metrics aggregation tests
├─ Real-time counter tests
└─ User snapshot tests

Implementation:
├─ GET /api/v1/metrics/daily
├─ GET /api/v1/metrics/users/:id
├─ Kafka consumer (ledger.entry_created)
├─ Redis counters (analytics:volume:{date})
└─ PostgreSQL schema (daily_metrics, user_snapshots)
```

---

### **Step 7: Infrastructure & Deployment** (~1-2 hours)

**What we'll build:**

```
├─ docker-compose.yml (all services + dependencies)
├─ Nginx reverse proxy configuration
├─ Prometheus metrics endpoints
├─ Health check endpoints
└─ GitHub Actions CI/CD pipeline
```
