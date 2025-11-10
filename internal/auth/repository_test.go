package auth

import (
	"context"
	"testing"
	"time"

	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/db"
	"github.com/kmassidik/mercuria/internal/common/logger"
)

func setupTestDB(t *testing.T) *Repository {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	cfg := config.DatabaseConfig{
		Host:            "localhost",
		Port:            "5432",
		User:            "postgres",
		Password:        "postgres",
		DBName:          "mercuria_auth_test",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}

	log := logger.New("test")
	database, err := db.Connect(cfg, log)
	if err != nil {
		t.Skipf("Cannot connect to database: %v", err)
		return nil
	}

	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		email VARCHAR(255) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		first_name VARCHAR(100),
		last_name VARCHAR(100),
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS refresh_tokens (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token_hash VARCHAR(255) NOT NULL,
		expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		revoked BOOLEAN DEFAULT FALSE
	);
	`

	if _, err := database.Exec(schema); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	return NewRepository(database, log)
}

func cleanupTestDB(_ *testing.T, repo *Repository) {
	if repo == nil {
		return
	}

	// Clean up tables
	repo.db.Exec("TRUNCATE users, refresh_tokens CASCADE")
	repo.db.Close()
}

func TestCreateUser(t *testing.T) {
	repo := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, repo)

	ctx := context.Background()

	user := &User{
		Email:        "test@example.com",
		PasswordHash: "hashed_password",
		FirstName:    "Test",
		LastName:     "User",
	}

	// Create user
	createdUser, err := repo.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if createdUser.ID == "" {
		t.Error("Expected user ID to be set")
	}

	if createdUser.Email != user.Email {
		t.Errorf("Expected email %s, got %s", user.Email, createdUser.Email)
	}

	// Try to create duplicate user
	_, err = repo.CreateUser(ctx, user)
	if err == nil {
		t.Error("Expected error for duplicate email")
	}
}

func TestGetUserByEmail(t *testing.T) {
	repo := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, repo)

	ctx := context.Background()

	// Create user
	user := &User{
		Email:        "test@example.com",
		PasswordHash: "hashed_password",
		FirstName:    "Test",
		LastName:     "User",
	}

	createdUser, err := repo.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Get user by email
	foundUser, err := repo.GetUserByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if foundUser.ID != createdUser.ID {
		t.Errorf("Expected user ID %s, got %s", createdUser.ID, foundUser.ID)
	}

	// Get non-existent user
	_, err = repo.GetUserByEmail(ctx, "nonexistent@example.com")
	if err == nil {
		t.Error("Expected error for non-existent user")
	}
}

func TestGetUserByID(t *testing.T) {
	repo := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, repo)

	ctx := context.Background()

	// Create user
	user := &User{
		Email:        "test@example.com",
		PasswordHash: "hashed_password",
	}

	createdUser, err := repo.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Get user by ID
	foundUser, err := repo.GetUserByID(ctx, createdUser.ID)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if foundUser.Email != user.Email {
		t.Errorf("Expected email %s, got %s", user.Email, foundUser.Email)
	}
}

func TestRefreshTokenOperations(t *testing.T) {
	repo := setupTestDB(t)
	if repo == nil {
		return
	}
	defer cleanupTestDB(t, repo)

	ctx := context.Background()

	// Create user first
	user := &User{
		Email:        "test@example.com",
		PasswordHash: "hashed_password",
	}

	createdUser, err := repo.CreateUser(ctx, user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create refresh token
	token := &RefreshToken{
		UserID:    createdUser.ID,
		TokenHash: "hashed_token",
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	}

	createdToken, err := repo.CreateRefreshToken(ctx, token)
	if err != nil {
		t.Fatalf("Failed to create refresh token: %v", err)
	}

	if createdToken.ID == "" {
		t.Error("Expected token ID to be set")
	}

	// Get refresh token
	foundToken, err := repo.GetRefreshToken(ctx, createdToken.TokenHash)
	if err != nil {
		t.Fatalf("Failed to get refresh token: %v", err)
	}

	if foundToken.UserID != createdUser.ID {
		t.Errorf("Expected user ID %s, got %s", createdUser.ID, foundToken.UserID)
	}

	// Revoke token
	err = repo.RevokeRefreshToken(ctx, createdToken.TokenHash)
	if err != nil {
		t.Fatalf("Failed to revoke token: %v", err)
	}

	// Verify token is revoked
	foundToken, err = repo.GetRefreshToken(ctx, createdToken.TokenHash)
	if err != nil {
		t.Fatalf("Failed to get token after revoke: %v", err)
	}

	if !foundToken.Revoked {
		t.Error("Expected token to be revoked")
	}
}