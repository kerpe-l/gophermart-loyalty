package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/kerpe-l/gophermart-loyalty/internal/apperrors"
	"github.com/kerpe-l/gophermart-loyalty/internal/luhn"
	"github.com/kerpe-l/gophermart-loyalty/internal/middleware"
	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

// OrderStore — интерфейс хранилища заказов (consumer-side).
type OrderStore interface {
	CreateOrder(ctx context.Context, userID int64, number string) (*model.Order, error)
	GetOrdersByUserID(ctx context.Context, userID int64) ([]model.Order, error)
}

// OrderHandler обрабатывает запросы, связанные с заказами.
type OrderHandler struct {
	store OrderStore
	log   *zap.Logger
}

// NewOrderHandler создаёт хендлер заказов.
func NewOrderHandler(store OrderStore, log *zap.Logger) *OrderHandler {
	return &OrderHandler{
		store: store,
		log:   log,
	}
}

// CreateOrder обрабатывает POST /api/user/orders.
func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	number := strings.TrimSpace(string(body))
	if number == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	if !luhn.Valid(number) {
		http.Error(w, http.StatusText(http.StatusUnprocessableEntity), http.StatusUnprocessableEntity)
		return
	}

	_, err = h.store.CreateOrder(r.Context(), userID, number)
	if err != nil {
		if errors.Is(err, apperrors.ErrOrderAlreadyOwned) {
			w.WriteHeader(http.StatusOK)
			return
		}
		if errors.Is(err, apperrors.ErrOrderOwnedByAnother) {
			http.Error(w, http.StatusText(http.StatusConflict), http.StatusConflict)
			return
		}
		h.log.Error("создание заказа", zap.Error(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// orderResponse — JSON-представление заказа для API.
type orderResponse struct {
	Number     string  `json:"number"`
	Status     string  `json:"status"`
	Accrual    float64 `json:"accrual,omitempty"`
	UploadedAt string  `json:"uploaded_at"`
}

// GetOrders обрабатывает GET /api/user/orders.
func (h *OrderHandler) GetOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	orders, err := h.store.GetOrdersByUserID(r.Context(), userID)
	if err != nil {
		h.log.Error("получение заказов", zap.Error(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := make([]orderResponse, len(orders))
	for i, o := range orders {
		resp[i] = orderResponse{
			Number:     o.Number,
			Status:     string(o.Status),
			UploadedAt: o.UploadedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if o.Accrual > 0 {
			resp[i].Accrual = float64(o.Accrual) / 100
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Error("сериализация заказов", zap.Error(err))
	}
}
