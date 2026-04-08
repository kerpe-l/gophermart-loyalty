package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/kerpe-l/gophermart-loyalty/internal/apperrors"
	"github.com/kerpe-l/gophermart-loyalty/internal/auth"
	"github.com/kerpe-l/gophermart-loyalty/internal/middleware"
	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

// mockOrderStore — мок хранилища заказов для тестов.
type mockOrderStore struct {
	createOrderFn      func(ctx context.Context, userID int64, number string) (*model.Order, error)
	getOrdersByUserFn  func(ctx context.Context, userID int64) ([]model.Order, error)
}

func (m *mockOrderStore) CreateOrder(ctx context.Context, userID int64, number string) (*model.Order, error) {
	return m.createOrderFn(ctx, userID, number)
}

func (m *mockOrderStore) GetOrdersByUserID(ctx context.Context, userID int64) ([]model.Order, error) {
	return m.getOrdersByUserFn(ctx, userID)
}

// authedRequest создаёт запрос с аутентификацией через middleware.
func authedRequest(t *testing.T, method, target, body string, userID int64) *http.Request {
	t.Helper()
	authMgr := auth.NewManager("test-secret")
	token, err := authMgr.GenerateToken(userID)
	if err != nil {
		t.Fatal(err)
	}

	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	// Прогоняем через middleware, чтобы user ID попал в context.
	rec := httptest.NewRecorder()
	var captured *http.Request
	middleware.AuthMiddleware(authMgr)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		captured = r
	})).ServeHTTP(rec, req)

	if captured == nil {
		t.Fatal("AuthMiddleware не пропустил запрос")
	}
	return captured
}

func TestCreateOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		store      *mockOrderStore
		wantStatus int
	}{
		{
			name: "новый заказ принят",
			body: "79927398713",
			store: &mockOrderStore{
				createOrderFn: func(_ context.Context, _ int64, _ string) (*model.Order, error) {
					return &model.Order{ID: 1, Number: "79927398713", Status: model.OrderStatusNew}, nil
				},
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name: "заказ уже загружен этим пользователем",
			body: "79927398713",
			store: &mockOrderStore{
				createOrderFn: func(_ context.Context, _ int64, _ string) (*model.Order, error) {
					return nil, apperrors.ErrOrderAlreadyOwned
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "заказ загружен другим пользователем",
			body: "79927398713",
			store: &mockOrderStore{
				createOrderFn: func(_ context.Context, _ int64, _ string) (*model.Order, error) {
					return nil, apperrors.ErrOrderOwnedByAnother
				},
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "невалидный номер по Luhn",
			body:       "12345",
			store:      &mockOrderStore{},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "пустое тело",
			body:       "",
			store:      &mockOrderStore{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := authedRequest(t, http.MethodPost, "/api/user/orders", tc.body, 1)
			rec := httptest.NewRecorder()

			h := NewOrderHandler(tc.store, zap.NewNop())
			h.CreateOrder(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("статус = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestGetOrders(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		store      *mockOrderStore
		wantStatus int
		wantLen    int
	}{
		{
			name: "есть заказы",
			store: &mockOrderStore{
				getOrdersByUserFn: func(_ context.Context, _ int64) ([]model.Order, error) {
					return []model.Order{
						{Number: "79927398713", Status: model.OrderStatusProcessed, Accrual: 50000, UploadedAt: now},
						{Number: "346436439", Status: model.OrderStatusNew, UploadedAt: now},
					}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantLen:    2,
		},
		{
			name: "нет заказов",
			store: &mockOrderStore{
				getOrdersByUserFn: func(_ context.Context, _ int64) ([]model.Order, error) {
					return nil, nil
				},
			},
			wantStatus: http.StatusNoContent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := authedRequest(t, http.MethodGet, "/api/user/orders", "", 1)
			rec := httptest.NewRecorder()

			h := NewOrderHandler(tc.store, zap.NewNop())
			h.GetOrders(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("статус = %d, want %d", rec.Code, tc.wantStatus)
			}

			if tc.wantLen > 0 {
				var resp []orderResponse
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("декодирование ответа: %v", err)
				}
				if len(resp) != tc.wantLen {
					t.Errorf("len = %d, want %d", len(resp), tc.wantLen)
				}
				// Проверяем конвертацию accrual из копеек в рубли.
				if resp[0].Accrual != 500 {
					t.Errorf("accrual = %v, want 500", resp[0].Accrual)
				}
			}
		})
	}
}
