package ledger

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// GET /api/v1/ledger/{id}
func (h *Handler) GetLedgerEntry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	entry, err := h.service.GetLedgerEntry(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entry)
}

// GET /api/v1/ledger/transaction/{id}
func (h *Handler) GetTransactionLedger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ledger, err := h.service.GetTransactionLedger(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ledger)
}

// GET /api/v1/ledger/wallet?wallet_id=xxx&limit=20&offset=0
func (h *Handler) GetWalletLedger(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	walletID := q.Get("wallet_id")
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit == 0 {
		limit = 20
	}

	entries, balance, err := h.service.GetWalletLedger(r.Context(), walletID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := WalletLedgerResponse{
		WalletID:       walletID,
		Entries:        entries,
		CurrentBalance: balance,
		Total:          len(entries),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GET /api/v1/ledger/stats?wallet_id=xxx
func (h *Handler) GetWalletStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	walletID := q.Get("wallet_id")

	stats, err := h.service.GetWalletStats(r.Context(), walletID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GET /api/v1/ledger?limit=50&offset=0
func (h *Handler) GetAllEntries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit == 0 {
		limit = 50
	}

	entries, err := h.service.GetAllEntries(r.Context(), limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := LedgerEntriesResponse{
		Entries: entries,
		Total:   len(entries),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
