package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/internal/common/middleware"
)

type Service struct {
	repo   *Repository
	config config.JWTConfig
	logger *logger.Logger
}

func NewService(repo *Repository, cfg config.JWTConfig, log *logger.Logger) *Service {
	return &Service{
		repo:   repo,
		config: cfg,
		logger: log,
	}
}

// Register creates a new user account
func (s *Service) Register(ctx context.Context, req *RegisterRequest) (*AuthResponse, error) {
	// Validate request
	if err := ValidateRegisterRequest(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Check if user already exists
	_, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err == nil {
		return nil, fmt.Errorf("user with email %s already exists", req.Email)
	}

	// Hash password
	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Create user
	user := &User{
		Email:        req.Email,
		PasswordHash: passwordHash,
		FirstName:    req.FirstName,
		LastName:     req.LastName,
	}

	createdUser, err := s.repo.CreateUser(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Generate tokens
	accessToken, err := middleware.GenerateToken(createdUser.ID, createdUser.Email, s.config)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := middleware.GenerateRefreshToken(createdUser.ID, s.config)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Store refresh token hash
	tokenHash := hashToken(refreshToken)
	refreshTokenRecord := &RefreshToken{
		UserID:    createdUser.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(s.config.RefreshTokenTTL),
	}

	if _, err := s.repo.CreateRefreshToken(ctx, refreshTokenRecord); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	s.logger.Infof("User registered: %s", createdUser.Email)

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         createdUser,
	}, nil
}

// Login authenticates a user
func (s *Service) Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error) {
	// Validate request
	if err := ValidateLoginRequest(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Get user by email
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	// Verify password
	if !VerifyPassword(user.PasswordHash, req.Password) {
		return nil, fmt.Errorf("invalid email or password")
	}

	// Generate tokens
	accessToken, err := middleware.GenerateToken(user.ID, user.Email, s.config)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := middleware.GenerateRefreshToken(user.ID, s.config)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Store refresh token hash
	tokenHash := hashToken(refreshToken)
	refreshTokenRecord := &RefreshToken{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(s.config.RefreshTokenTTL),
	}

	if _, err := s.repo.CreateRefreshToken(ctx, refreshTokenRecord); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	s.logger.Infof("User logged in: %s", user.Email)

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	}, nil
}

// RefreshAccessToken generates a new access token using refresh token
func (s *Service) RefreshAccessToken(ctx context.Context, refreshTokenString string) (*AuthResponse, error) {
	if refreshTokenString == "" {
		return nil, fmt.Errorf("refresh token is required")
	}

	// Hash the token to look up in database
	tokenHash := hashToken(refreshTokenString)

	// Get refresh token from database
	refreshToken, err := s.repo.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token")
	}

	// Check if token is revoked
	if refreshToken.Revoked {
		return nil, fmt.Errorf("refresh token has been revoked")
	}

	// Check if token is expired
	if time.Now().After(refreshToken.ExpiresAt) {
		return nil, fmt.Errorf("refresh token has expired")
	}

	// Get user
	user, err := s.repo.GetUserByID(ctx, refreshToken.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	// Generate new access token
	accessToken, err := middleware.GenerateToken(user.ID, user.Email, s.config)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// Optionally: Rotate refresh token (best practice)
	// Revoke old token
	if err := s.repo.RevokeRefreshToken(ctx, tokenHash); err != nil {
		s.logger.Warnf("Failed to revoke old refresh token: %v", err)
	}

	// Generate new refresh token
	newRefreshToken, err := middleware.GenerateRefreshToken(user.ID, s.config)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new refresh token: %w", err)
	}

	// Store new refresh token
	newTokenHash := hashToken(newRefreshToken)
	newRefreshTokenRecord := &RefreshToken{
		UserID:    user.ID,
		TokenHash: newTokenHash,
		ExpiresAt: time.Now().Add(s.config.RefreshTokenTTL),
	}

	if _, err := s.repo.CreateRefreshToken(ctx, newRefreshTokenRecord); err != nil {
		return nil, fmt.Errorf("failed to store new refresh token: %w", err)
	}

	s.logger.Infof("Access token refreshed for user: %s", user.Email)

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		User:         user,
	}, nil
}

// GetCurrentUser retrieves the current authenticated user
func (s *Service) GetCurrentUser(ctx context.Context, userID string) (*User, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return user, nil
}

// hashToken creates a SHA-256 hash of a token
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}