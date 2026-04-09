package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/kerpe-l/gophermart-loyalty/internal/apperrors"
	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

type mockBalanceStore struct {
	getBalanceFn       func(ctx context.Context, userID int64) (*model.Balance, error)
	createWithdrawalFn func(ctx context.Context, userID int64, orderNumber string, amount int64) error
	getWithdrawalsFn   func(ctx context.Context, userID int64) ([]model.Withdrawal, error)
}

func (m *mockBalanceStore) GetBalance(ctx context.Context, userID int64) (*model.Balance, error) {
	return m.getBalanceFn(ctx, userID)
}

func (m *mockBalanceStore) CreateWithdrawal(ctx context.Context, userID int64, orderNumber string, amount int64) error {
	return m.createWithdrawalFn(ctx, userID, orderNumber, amount)
}

func (m *mockBalanceStore) GetWithdrawalsByUserID(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	return m.getWithdrawalsFn(ctx, userID)
}

func TestGetBalance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		store      *mockBalanceStore
		wantStatus int
		wantBody   *balanceResponse
	}{
		{
			name: "баланс с начислениями и списаниями",
			store: &mockBalanceStore{
				getBalanceFn: func(_ context.Context, _ int64) (*model.Balance, error) {
					return &model.Balance{Current: 50050, Withdrawn: 4200}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantBody:   &balanceResponse{Current: 500.5, Withdrawn: 42},
		},
		{
			name: "нулевой баланс",
			store: &mockBalanceStore{
				getBalanceFn: func(_ context.Context, _ int64) (*model.Balance, error) {
					return &model.Balance{}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantBody:   &balanceResponse{Current: 0, Withdrawn: 0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := authedRequest(t, http.MethodGet, "/api/user/balance", "", 1)
			rec := httptest.NewRecorder()

			h := NewBalanceHandler(tc.store, zap.NewNop())
			h.GetBalance(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("статус = %d, want %d", rec.Code, tc.wantStatus)
			}

			if tc.wantBody != nil {
				var got balanceResponse
				if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
					t.Fatalf("декодирование: %v", err)
				}
				if got.Current != tc.wantBody.Current {
					t.Errorf("current = %v, want %v", got.Current, tc.wantBody.Current)
				}
				if got.Withdrawn != tc.wantBody.Withdrawn {
					t.Errorf("withdrawn = %v, want %v", got.Withdrawn, tc.wantBody.Withdrawn)
				}
			}
		})
	}
}

func TestWithdraw(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		store      *mockBalanceStore
		wantStatus int
	}{
		{
			name: "успешное списание",
			body: `{"order":"2377225624","sum":751}`,
			store: &mockBalanceStore{
				createWithdrawalFn: func(_ context.Context, _ int64, _ string, _ int64) error {
					return nil
				},
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "недостаточно средств",
			body: `{"order":"2377225624","sum":751}`,
			store: &mockBalanceStore{
				createWithdrawalFn: func(_ context.Context, _ int64, _ string, _ int64) error {
					return apperrors.ErrInsufficientFunds
				},
			},
			wantStatus: http.StatusPaymentRequired,
		},
		{
			name:       "невалидный номер заказа по Luhn",
			body:       `{"order":"12345","sum":100}`,
			store:      &mockBalanceStore{},
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "пустой номер заказа",
			body:       `{"order":"","sum":100}`,
			store:      &mockBalanceStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "невалидный JSON",
			body:       `not json`,
			store:      &mockBalanceStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "нулевая сумма",
			body:       `{"order":"2377225624","sum":0}`,
			store:      &mockBalanceStore{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := authedRequest(t, http.MethodPost, "/api/user/balance/withdraw", tc.body, 1)
			rec := httptest.NewRecorder()

			h := NewBalanceHandler(tc.store, zap.NewNop())
			h.Withdraw(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("статус = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestGetWithdrawals(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		store      *mockBalanceStore
		wantStatus int
		wantLen    int
	}{
		{
			name: "есть списания",
			store: &mockBalanceStore{
				getWithdrawalsFn: func(_ context.Context, _ int64) ([]model.Withdrawal, error) {
					return []model.Withdrawal{
						{OrderNumber: "2377225624", Amount: 50000, ProcessedAt: now},
					}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantLen:    1,
		},
		{
			name: "нет списаний",
			store: &mockBalanceStore{
				getWithdrawalsFn: func(_ context.Context, _ int64) ([]model.Withdrawal, error) {
					return nil, nil
				},
			},
			wantStatus: http.StatusNoContent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := authedRequest(t, http.MethodGet, "/api/user/withdrawals", "", 1)
			rec := httptest.NewRecorder()

			h := NewBalanceHandler(tc.store, zap.NewNop())
			h.GetWithdrawals(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("статус = %d, want %d", rec.Code, tc.wantStatus)
			}

			if tc.wantLen > 0 {
				var resp []withdrawalResponse
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					t.Fatalf("декодирование: %v", err)
				}
				if len(resp) != tc.wantLen {
					t.Errorf("len = %d, want %d", len(resp), tc.wantLen)
				}
				if resp[0].Sum != 500 {
					t.Errorf("sum = %v, want 500", resp[0].Sum)
				}
			}
		})
	}
}
