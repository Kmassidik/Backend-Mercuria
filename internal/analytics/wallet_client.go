package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/kmassidik/mercuria/internal/common/mtls"
)

type WalletClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewWalletClient() *WalletClient {
	walletURL := os.Getenv("WALLET_SERVICE_URL")
	if walletURL == "" {
		walletURL = "http://localhost:8081" // Default to public API
	}

	// Check if mTLS is enabled for internal communication
	mtlsConfig := mtls.LoadFromEnv()
	
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	// If mTLS is enabled, configure TLS client
	if mtlsConfig.Enabled {
		tlsConfig, err := mtlsConfig.ClientTLSConfig()
		if err == nil {
			httpClient.Transport = &http.Transport{
				TLSClientConfig: tlsConfig,
			}
			
			// Use internal wallet service URL if mTLS is enabled
			if internalURL := os.Getenv("WALLET_INTERNAL_URL"); internalURL != "" {
				walletURL = internalURL
			}
		}
	}

	return &WalletClient{
		baseURL:    walletURL,
		httpClient: httpClient,
	}
}

type Wallet struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
}

type WalletsResponse struct {
	Wallets []Wallet `json:"wallets"`
	Total   int      `json:"total"`
}

// GetUserWalletIDs fetches all wallet IDs for a given user
// Uses Authorization token to authenticate with Wallet Service
func (c *WalletClient) GetUserWalletIDs(ctx context.Context, userID, authToken string) ([]string, error) {
	url := fmt.Sprintf("%s/api/v1/wallets/my-wallets", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Forward the JWT token for authentication
	req.Header.Set("Authorization", authToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wallet service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("wallet service returned %d: %s", resp.StatusCode, string(body))
	}

	var walletsResp WalletsResponse
	if err := json.NewDecoder(resp.Body).Decode(&walletsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Extract wallet IDs
	walletIDs := make([]string, len(walletsResp.Wallets))
	for i, w := range walletsResp.Wallets {
		walletIDs[i] = w.ID
	}

	return walletIDs, nil
}

// GetUserWalletIDsInternal uses mTLS for internal service-to-service calls
// This version doesn't require JWT - mTLS certificate is the authentication
func (c *WalletClient) GetUserWalletIDsInternal(ctx context.Context, userID string) ([]string, error) {
	// Use internal wallet service URL
	internalURL := os.Getenv("WALLET_INTERNAL_URL")
	if internalURL == "" {
		return nil, fmt.Errorf("WALLET_INTERNAL_URL not configured")
	}

	url := fmt.Sprintf("%s/api/v1/internal/wallets/user/%s", internalURL, userID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// mTLS client should already be configured
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wallet service error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("wallet service returned %d: %s", resp.StatusCode, string(body))
	}

	var walletsResp WalletsResponse
	if err := json.NewDecoder(resp.Body).Decode(&walletsResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	walletIDs := make([]string, len(walletsResp.Wallets))
	for i, w := range walletsResp.Wallets {
		walletIDs[i] = w.ID
	}

	return walletIDs, nil
}

// NewMTLSWalletClient creates a wallet client with mTLS configuration
func NewMTLSWalletClient() (*WalletClient, error) {
	mtlsConfig := mtls.LoadFromEnv()
	if !mtlsConfig.Enabled {
		return nil, fmt.Errorf("mTLS is not enabled")
	}

	tlsConfig, err := mtlsConfig.ClientTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load mTLS config: %w", err)
	}

	internalURL := os.Getenv("WALLET_INTERNAL_URL")
	if internalURL == "" {
		internalURL = "https://localhost:9081" // Default internal port
	}

	return &WalletClient{
		baseURL: internalURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
	}, nil
}

// HealthCheck checks if Wallet Service is reachable
func (c *WalletClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/health", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wallet service unhealthy: status %d", resp.StatusCode)
	}

	return nil
}