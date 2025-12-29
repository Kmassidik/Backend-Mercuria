#!/bin/bash
# Mercuria Backend - Complete Development Setup Script
# Run this script after: docker-compose up -d
# Usage: bash setup.sh

set -e

echo ""
echo "╔═══════════════════════════════════════════════════════╗"
echo "║     🚀 Mercuria Backend Development Setup 🚀         ║"
echo "║           Banking-Grade Microservices                 ║"
echo "╚═══════════════════════════════════════════════════════╝"
echo ""

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_step() {
    echo -e "${BLUE}▶ $1${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

# Function to check if Docker is running
check_docker() {
    print_step "Checking Docker..."
    if ! docker info > /dev/null 2>&1; then
        print_error "Docker is not running. Please start Docker first."
        exit 1
    fi
    print_success "Docker is running"
}

# Function to check if containers are running
check_containers() {
    print_step "Checking Docker containers..."
    
    REQUIRED_CONTAINERS=("mercuria-postgres" "mercuria-redis" "mercuria-kafka" "mercuria-zookeeper")
    
    for container in "${REQUIRED_CONTAINERS[@]}"; do
        if ! docker ps --format '{{.Names}}' | grep -q "^${container}$"; then
            print_error "Container ${container} is not running"
            print_warning "Please run: docker-compose up -d"
            exit 1
        fi
    done
    
    print_success "All required containers are running"
}

# Wait for services to be ready
wait_for_services() {
    print_step "Waiting for services to be ready..."
    
    # Wait for PostgreSQL
    echo -n "  Waiting for PostgreSQL"
    for i in {1..30}; do
        if docker exec mercuria-postgres pg_isready -U postgres > /dev/null 2>&1; then
            echo ""
            print_success "PostgreSQL is ready"
            break
        fi
        echo -n "."
        sleep 1
    done
    
    # Wait for Redis
    echo -n "  Waiting for Redis"
    for i in {1..30}; do
        if docker exec mercuria-redis redis-cli ping > /dev/null 2>&1; then
            echo ""
            print_success "Redis is ready"
            break
        fi
        echo -n "."
        sleep 1
    done
    
    # Wait for Kafka
    echo -n "  Waiting for Kafka"
    for i in {1..60}; do
        if docker exec mercuria-kafka kafka-broker-api-versions --bootstrap-server localhost:9092 > /dev/null 2>&1; then
            echo ""
            print_success "Kafka is ready"
            break
        fi
        echo -n "."
        sleep 2
    done
}

# Create databases
create_databases() {
    print_step "Creating databases..."
    
    DATABASES=("mercuria_auth" "mercuria_wallet" "mercuria_transaction" "mercuria_ledger" "mercuria_analytics")
    
    for db in "${DATABASES[@]}"; do
        if docker exec mercuria-postgres psql -U postgres -lqt | cut -d \| -f 1 | grep -qw "${db}"; then
            print_warning "Database ${db} already exists - skipping"
        else
            docker exec mercuria-postgres psql -U postgres -c "CREATE DATABASE ${db};" > /dev/null 2>&1
            print_success "Created database: ${db}"
        fi
    done
}

# Run migrations
run_migrations() {
    print_step "Running database migrations..."
    
    if [ ! -f "scripts/run_migrations.sh" ]; then
        print_error "Migration script not found: scripts/run_migrations.sh"
        exit 1
    fi
    
    bash scripts/run_migrations.sh
}

# Create Kafka topics
create_kafka_topics() {
    print_step "Creating Kafka topics..."
    
    if [ ! -f "scripts/create_kafka_topics.sh" ]; then
        print_error "Kafka topics script not found: scripts/create_kafka_topics.sh"
        exit 1
    fi
    
    bash scripts/create_kafka_topics.sh
}

# Generate mTLS certificates (optional)
generate_certificates() {
    print_step "Checking mTLS certificates..."
    
    if [ -d "certs/ca" ] && [ -f "certs/ca/ca.crt" ]; then
        print_warning "Certificates already exist - skipping generation"
        return
    fi
    
    if [ ! -f "scripts/generate-certs.sh" ]; then
        print_error "Certificate generation script not found: scripts/generate-certs.sh"
        exit 1
    fi
    
    print_step "Generating mTLS certificates..."
    bash scripts/generate-certs.sh
}

# Create .env file if it doesn't exist
create_env_file() {
    print_step "Checking environment configuration..."
    
    if [ -f ".env" ]; then
        print_warning ".env file already exists - skipping"
        return
    fi
    
    if [ -f "example.env" ]; then
        cp example.env .env
        print_success "Created .env from example.env"
        print_warning "Please review and update .env with your configuration"
    else
        print_error "example.env not found. Please create .env manually"
    fi
}

# Verify Go installation
check_go() {
    print_step "Checking Go installation..."
    
    if ! command -v go &> /dev/null; then
        print_error "Go is not installed. Please install Go 1.22+ first"
        exit 1
    fi
    
    GO_VERSION=$(go version | awk '{print $3}')
    print_success "Go is installed: ${GO_VERSION}"
}

# Install Go dependencies
install_dependencies() {
    print_step "Installing Go dependencies..."
    
    if [ ! -f "go.mod" ]; then
        print_error "go.mod not found. Are you in the project root?"
        exit 1
    fi
    
    go mod download
    go mod tidy
    print_success "Go dependencies installed"
}

# Run tests (optional)
run_tests() {
    if [ "$1" == "--skip-tests" ]; then
        print_warning "Skipping tests (--skip-tests flag provided)"
        return
    fi
    
    print_step "Running tests..."
    
    if go test ./... -v > /dev/null 2>&1; then
        print_success "All tests passed"
    else
        print_warning "Some tests failed - but continuing setup"
    fi
}

# Print service information
print_service_info() {
    echo ""
    echo "╔═══════════════════════════════════════════════════════╗"
    echo "║           🎉 Setup Complete! 🎉                       ║"
    echo "╚═══════════════════════════════════════════════════════╝"
    echo ""
    echo "📊 Infrastructure Services:"
    echo "  • PostgreSQL:  localhost:5432"
    echo "  • Redis:       localhost:6379"
    echo "  • Kafka:       localhost:9092"
    echo "  • Zookeeper:   localhost:2181"
    echo ""
    echo "🔧 Microservices (after starting):"
    echo "  • Auth Service:         http://localhost:8080"
    echo "  • Wallet Service:       http://localhost:8081"
    echo "  • Transaction Service:  http://localhost:8082"
    echo "  • Ledger Service:       http://localhost:8083"
    echo "  • Analytics Service:    http://localhost:8084"
    echo ""
    echo "🔐 Internal mTLS Ports (optional):"
    echo "  • Auth:         https://localhost:9080"
    echo "  • Wallet:       https://localhost:9081"
    echo "  • Transaction:  https://localhost:9082"
    echo "  • Ledger:       https://localhost:9083"
    echo "  • Analytics:    https://localhost:9084"
    echo ""
    echo "📚 Next Steps:"
    echo "  1. Start services:"
    echo "     $ make run-auth"
    echo "     $ make run-wallet"
    echo "     $ make run-transaction"
    echo "     $ make run-ledger"
    echo "     $ make run-analytics"
    echo ""
    echo "  2. Or start all at once:"
    echo "     $ make run-all"
    echo ""
    echo "  3. Check health:"
    echo "     $ curl http://localhost:8080/health"
    echo ""
    echo "  4. View logs:"
    echo "     $ docker-compose logs -f"
    echo ""
    echo "📖 Documentation:"
    echo "  • API Docs: docs/manual-api-docs.md"
    echo "  • Setup Guide: docs/manual-setup-guide.md"
    echo "  • PRD: Mercuria_Backend_PRD_v4.md"
    echo ""
}

# Main execution
main() {
    local skip_tests=false
    
    # Parse arguments
    for arg in "$@"; do
        case $arg in
            --skip-tests)
                skip_tests=true
                ;;
            --help|-h)
                echo "Usage: bash setup.sh [OPTIONS]"
                echo ""
                echo "Options:"
                echo "  --skip-tests    Skip running Go tests"
                echo "  --help, -h      Show this help message"
                exit 0
                ;;
        esac
    done
    
    # Run setup steps
    check_docker
    check_containers
    wait_for_services
    create_databases
    run_migrations
    create_kafka_topics
    generate_certificates
    create_env_file
    check_go
    install_dependencies
    
    if [ "$skip_tests" == true ]; then
        run_tests --skip-tests
    else
        run_tests
    fi
    
    print_service_info
}

# Run main function
main "$@"
