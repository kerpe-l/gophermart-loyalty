package handler

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"

	"go.uber.org/zap"

	"github.com/kerpe-l/gophermart-loyalty/internal/apperrors"
	"github.com/kerpe-l/gophermart-loyalty/internal/luhn"
	"github.com/kerpe-l/gophermart-loyalty/internal/middleware"
	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

// BalanceStore — интерфейс хранилища баланса и списаний (consumer-side).
type BalanceStore interface {
	GetBalance(ctx context.Context, userID int64) (*model.Balance, error)
	CreateWithdrawal(ctx context.Context, userID int64, orderNumber string, amount int64) error
	GetWithdrawalsByUserID(ctx context.Context, userID int64) ([]model.Withdrawal, error)
}

// BalanceHandler обрабатывает запросы баланса и списаний.
type BalanceHandler struct {
	store BalanceStore
	log   *zap.Logger
}

// NewBalanceHandler создаёт хендлер баланса.
func NewBalanceHandler(store BalanceStore, log *zap.Logger) *BalanceHandler {
	return &BalanceHandler{
		store: store,
		log:   log,
	}
}

// balanceResponse — JSON-представление баланса для API.
type balanceResponse struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

// GetBalance обрабатывает GET /api/user/balance.
func (h *BalanceHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	bal, err := h.store.GetBalance(r.Context(), userID)
	if err != nil {
		h.log.Error("получение баланса", zap.Error(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	resp := balanceResponse{
		Current:   float64(bal.Current) / 100,
		Withdrawn: float64(bal.Withdrawn) / 100,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error("сериализация баланса", zap.Error(err))
	}
}

// withdrawRequest — тело запроса на списание.
type withdrawRequest struct {
	Order string  `json:"order"`
	Sum   float64 `json:"sum"`
}

// Withdraw обрабатывает POST /api/user/balance/withdraw.
func (h *BalanceHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	var req withdrawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if req.Order == "" || req.Sum <= 0 {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if !luhn.Valid(req.Order) {
		http.Error(w, http.StatusText(http.StatusUnprocessableEntity), http.StatusUnprocessableEntity)
		return
	}

	// Конвертируем рубли → копейки.
	amount := int64(math.Round(req.Sum * 100))

	err := h.store.CreateWithdrawal(r.Context(), userID, req.Order, amount)
	if err != nil {
		if errors.Is(err, apperrors.ErrInsufficientFunds) {
			http.Error(w, http.StatusText(http.StatusPaymentRequired), http.StatusPaymentRequired)
			return
		}
		h.log.Error("списание баллов", zap.Error(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// withdrawalResponse — JSON-представление списания для API.
type withdrawalResponse struct {
	Order       string  `json:"order"`
	Sum         float64 `json:"sum"`
	ProcessedAt string  `json:"processed_at"`
}

// GetWithdrawals обрабатывает GET /api/user/withdrawals.
func (h *BalanceHandler) GetWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	wds, err := h.store.GetWithdrawalsByUserID(r.Context(), userID)
	if err != nil {
		h.log.Error("получение списаний", zap.Error(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if len(wds) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := make([]withdrawalResponse, len(wds))
	for i, wd := range wds {
		resp[i] = withdrawalResponse{
			Order:       wd.OrderNumber,
			Sum:         float64(wd.Amount) / 100,
			ProcessedAt: wd.ProcessedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error("сериализация списаний", zap.Error(err))
	}
}
