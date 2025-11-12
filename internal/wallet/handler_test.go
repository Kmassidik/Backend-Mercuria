package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kmassidik/mercuria/internal/common/logger"
	"github.com/kmassidik/mercuria/internal/common/middleware"
)

// MockService is a mock implementation of ServiceInterface for testing
// NOTE: This implements the ServiceInterface so it can be passed to NewHandler
type MockService struct {
	CreateWalletFunc    func(ctx context.Context, req *CreateWalletRequest) (*Wallet, error)
	GetWalletFunc       func(ctx context.Context, walletID string) (*Wallet, error)
	DepositFunc         func(ctx context.Context, walletID string, req *DepositRequest) (*Wallet, error)
	WithdrawFunc        func(ctx context.Context, walletID string, req *WithdrawRequest) (*Wallet, error)
	GetWalletEventsFunc func(ctx context.Context, walletID string, limit, offset int) ([]WalletEvent, error)
}

// Implement ServiceInterface methods
func (m *MockService) CreateWallet(ctx context.Context, req *CreateWalletRequest) (*Wallet, error) {
	if m.CreateWalletFunc != nil {
		return m.CreateWalletFunc(ctx, req)
	}
	return nil, fmt.Errorf("CreateWalletFunc not set")
}

func (m *MockService) GetWallet(ctx context.Context, walletID string) (*Wallet, error) {
	if m.GetWalletFunc != nil {
		return m.GetWalletFunc(ctx, walletID)
	}
	return nil, fmt.Errorf("GetWalletFunc not set")
}

func (m *MockService) Deposit(ctx context.Context, walletID string, req *DepositRequest) (*Wallet, error) {
	if m.DepositFunc != nil {
		return m.DepositFunc(ctx, walletID, req)
	}
	return nil, fmt.Errorf("DepositFunc not set")
}

func (m *MockService) Withdraw(ctx context.Context, walletID string, req *WithdrawRequest) (*Wallet, error) {
	if m.WithdrawFunc != nil {
		return m.WithdrawFunc(ctx, walletID, req)
	}
	return nil, fmt.Errorf("WithdrawFunc not set")
}

func (m *MockService) GetWalletEvents(ctx context.Context, walletID string, limit, offset int) ([]WalletEvent, error) {
	if m.GetWalletEventsFunc != nil {
		return m.GetWalletEventsFunc(ctx, walletID, limit, offset)
	}
	return nil, fmt.Errorf("GetWalletEventsFunc not set")
}

// Verify MockService implements ServiceInterface at compile time
var _ ServiceInterface = (*MockService)(nil)

// TEST: Create wallet endpoint
func TestHandlerCreateWallet(t *testing.T) {
	log := logger.New("test")

	tests := []struct {
		name           string
		body           interface{}
		userID         string
		mockResponse   *Wallet
		mockError      error
		expectedStatus int
	}{
		{
			name: "successful wallet creation",
			body: CreateWalletRequest{
				Currency: "USD",
			},
			userID: "user-123",
			mockResponse: &Wallet{
				ID:       "wallet-123",
				UserID:   "user-123",
				Currency: "USD",
				Balance:  "0.0000",
				Status:   StatusActive,
			},
			mockError:      nil,
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "invalid request body",
			body:           "invalid-json",
			userID:         "user-123",
			mockResponse:   nil,
			mockError:      nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "validation error",
			body: CreateWalletRequest{
				Currency: "INVALID",
			},
			userID:         "user-123",
			mockResponse:   nil,
			mockError:      fmt.Errorf("invalid currency"),
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock service
			mockService := &MockService{
				CreateWalletFunc: func(ctx context.Context, req *CreateWalletRequest) (*Wallet, error) {
					return tt.mockResponse, tt.mockError
				},
			}

			handler := NewHandler(mockService, log)

			// Create request
			var bodyBytes []byte
			if str, ok := tt.body.(string); ok {
				bodyBytes = []byte(str)
			} else {
				bodyBytes, _ = json.Marshal(tt.body)
			}

			req := httptest.NewRequest("POST", "/api/v1/wallets", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			// Add user ID to context (simulating JWT middleware)
			ctx := context.WithValue(req.Context(), middleware.UserIDKey, tt.userID)
			req = req.WithContext(ctx)

			// Record response
			rr := httptest.NewRecorder()
			handler.CreateWallet(rr, req)

			// Check status
			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			// Verify response for successful case
			if tt.expectedStatus == http.StatusCreated && tt.mockResponse != nil {
				var resp WalletResponse
				json.NewDecoder(rr.Body).Decode(&resp)

				if resp.Wallet.ID != tt.mockResponse.ID {
					t.Errorf("Expected wallet ID %s, got %s", tt.mockResponse.ID, resp.Wallet.ID)
				}
			}
		})
	}
}

// TEST: Get wallet endpoint
func TestHandlerGetWallet(t *testing.T) {
	log := logger.New("test")

	tests := []struct {
		name           string
		walletID       string
		userID         string
		mockResponse   *Wallet
		mockError      error
		expectedStatus int
	}{
		{
			name:     "successful get wallet",
			walletID: "wallet-123",
			userID:   "user-123",
			mockResponse: &Wallet{
				ID:       "wallet-123",
				UserID:   "user-123",
				Currency: "USD",
				Balance:  "100.50",
				Status:   StatusActive,
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "wallet not found",
			walletID:       "wallet-999",
			userID:         "user-123",
			mockResponse:   nil,
			mockError:      fmt.Errorf("wallet not found"),
			expectedStatus: http.StatusNotFound,
		},
		{
			name:     "access denied - different user",
			walletID: "wallet-123",
			userID:   "user-456", // Different user
			mockResponse: &Wallet{
				ID:       "wallet-123",
				UserID:   "user-123", // Wallet belongs to user-123
				Currency: "USD",
				Balance:  "100.50",
				Status:   StatusActive,
			},
			mockError:      nil,
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &MockService{
				GetWalletFunc: func(ctx context.Context, walletID string) (*Wallet, error) {
					return tt.mockResponse, tt.mockError
				},
			}

			handler := NewHandler(mockService, log)

			req := httptest.NewRequest("GET", "/api/v1/wallets/"+tt.walletID, nil)
			req.SetPathValue("id", tt.walletID)

			ctx := context.WithValue(req.Context(), middleware.UserIDKey, tt.userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.GetWallet(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

// TEST: Deposit endpoint
func TestHandlerDeposit(t *testing.T) {
	log := logger.New("test")

	tests := []struct {
		name           string
		walletID       string
		userID         string
		body           DepositRequest
		mockWallet     *Wallet
		mockUpdated    *Wallet
		mockError      error
		expectedStatus int
	}{
		{
			name:     "successful deposit",
			walletID: "wallet-123",
			userID:   "user-123",
			body: DepositRequest{
				Amount:         "50.00",
				IdempotencyKey: "deposit-key-1",
			},
			mockWallet: &Wallet{
				ID:       "wallet-123",
				UserID:   "user-123",
				Currency: "USD",
				Balance:  "100.00",
				Status:   StatusActive,
			},
			mockUpdated: &Wallet{
				ID:       "wallet-123",
				UserID:   "user-123",
				Currency: "USD",
				Balance:  "150.00",
				Status:   StatusActive,
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:     "invalid amount",
			walletID: "wallet-123",
			userID:   "user-123",
			body: DepositRequest{
				Amount:         "-50.00", // Negative amount
				IdempotencyKey: "deposit-key-2",
			},
			mockWallet: &Wallet{
				ID:       "wallet-123",
				UserID:   "user-123",
				Currency: "USD",
				Balance:  "100.00",
				Status:   StatusActive,
			},
			mockUpdated:    nil,
			mockError:      fmt.Errorf("invalid amount"),
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &MockService{
				GetWalletFunc: func(ctx context.Context, walletID string) (*Wallet, error) {
					if tt.mockWallet != nil {
						return tt.mockWallet, nil
					}
					return nil, fmt.Errorf("wallet not found")
				},
				DepositFunc: func(ctx context.Context, walletID string, req *DepositRequest) (*Wallet, error) {
					return tt.mockUpdated, tt.mockError
				},
			}

			handler := NewHandler(mockService, log)

			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/api/v1/wallets/"+tt.walletID+"/deposit", bytes.NewBuffer(bodyBytes))
			req.SetPathValue("id", tt.walletID)
			req.Header.Set("Content-Type", "application/json")

			ctx := context.WithValue(req.Context(), middleware.UserIDKey, tt.userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.Deposit(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

// TEST: Withdraw endpoint
func TestHandlerWithdraw(t *testing.T) {
	log := logger.New("test")

	tests := []struct {
		name           string
		walletID       string
		userID         string
		body           WithdrawRequest
		mockWallet     *Wallet
		mockUpdated    *Wallet
		mockError      error
		expectedStatus int
	}{
		{
			name:     "successful withdrawal",
			walletID: "wallet-123",
			userID:   "user-123",
			body: WithdrawRequest{
				Amount:         "25.00",
				IdempotencyKey: "withdraw-key-1",
			},
			mockWallet: &Wallet{
				ID:       "wallet-123",
				UserID:   "user-123",
				Currency: "USD",
				Balance:  "100.00",
				Status:   StatusActive,
			},
			mockUpdated: &Wallet{
				ID:       "wallet-123",
				UserID:   "user-123",
				Currency: "USD",
				Balance:  "75.00",
				Status:   StatusActive,
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:     "insufficient balance",
			walletID: "wallet-123",
			userID:   "user-123",
			body: WithdrawRequest{
				Amount:         "200.00",
				IdempotencyKey: "withdraw-key-2",
			},
			mockWallet: &Wallet{
				ID:       "wallet-123",
				UserID:   "user-123",
				Currency: "USD",
				Balance:  "100.00",
				Status:   StatusActive,
			},
			mockUpdated:    nil,
			mockError:      fmt.Errorf("insufficient balance"),
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &MockService{
				GetWalletFunc: func(ctx context.Context, walletID string) (*Wallet, error) {
					if tt.mockWallet != nil {
						return tt.mockWallet, nil
					}
					return nil, fmt.Errorf("wallet not found")
				},
				WithdrawFunc: func(ctx context.Context, walletID string, req *WithdrawRequest) (*Wallet, error) {
					return tt.mockUpdated, tt.mockError
				},
			}

			handler := NewHandler(mockService, log)

			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/api/v1/wallets/"+tt.walletID+"/withdraw", bytes.NewBuffer(bodyBytes))
			req.SetPathValue("id", tt.walletID)
			req.Header.Set("Content-Type", "application/json")

			ctx := context.WithValue(req.Context(), middleware.UserIDKey, tt.userID)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()
			handler.Withdraw(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}

// TEST: Get wallet events endpoint
func TestHandlerGetWalletEvents(t *testing.T) {
	log := logger.New("test")

	mockEvents := []WalletEvent{
		{
			ID:            "event-1",
			WalletID:      "wallet-123",
			EventType:     EventTypeDeposit,
			Amount:        "50.00",
			BalanceBefore: "100.00",
			BalanceAfter:  "150.00",
		},
		{
			ID:            "event-2",
			WalletID:      "wallet-123",
			EventType:     EventTypeWithdrawal,
			Amount:        "25.00",
			BalanceBefore: "150.00",
			BalanceAfter:  "125.00",
		},
	}

	mockService := &MockService{
		GetWalletFunc: func(ctx context.Context, walletID string) (*Wallet, error) {
			return &Wallet{
				ID:       "wallet-123",
				UserID:   "user-123",
				Currency: "USD",
				Balance:  "125.00",
				Status:   StatusActive,
			}, nil
		},
		GetWalletEventsFunc: func(ctx context.Context, walletID string, limit, offset int) ([]WalletEvent, error) {
			return mockEvents, nil
		},
	}

	handler := NewHandler(mockService, log)

	req := httptest.NewRequest("GET", "/api/v1/wallets/wallet-123/events?limit=10&offset=0", nil)
	req.SetPathValue("id", "wallet-123")

	ctx := context.WithValue(req.Context(), middleware.UserIDKey, "user-123")
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.GetWalletEvents(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var resp WalletEventsResponse
	json.NewDecoder(rr.Body).Decode(&resp)

	if len(resp.Events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(resp.Events))
	}
}