package accrual

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetOrderAccrual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantErr    bool
		wantStatus OrderStatus
		wantRate   bool // ожидаем ErrTooManyRequests
		wantNotReg bool // ожидаем ErrOrderNotRegistered
	}{
		{
			name: "200 PROCESSED с начислением",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"order":"123","status":"PROCESSED","accrual":500.5}`))
			},
			wantStatus: StatusProcessed,
		},
		{
			name: "200 INVALID без начисления",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"order":"123","status":"INVALID"}`))
			},
			wantStatus: StatusInvalid,
		},
		{
			name: "204 не зарегистрирован",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			},
			wantErr:    true,
			wantNotReg: true,
		},
		{
			name: "429 rate limit с Retry-After",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "30")
				w.WriteHeader(http.StatusTooManyRequests)
			},
			wantErr:  true,
			wantRate: true,
		},
		{
			name: "500 серверная ошибка",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			client := NewClient(srv.URL)
			result, err := client.GetOrderAccrual(context.Background(), "123")

			if tc.wantErr {
				if err == nil {
					t.Fatal("ожидалась ошибка, но её нет")
				}
				if tc.wantRate {
					var rateErr *ErrTooManyRequests
					if !errors.As(err, &rateErr) {
						t.Fatalf("ожидалась ErrTooManyRequests, got %T", err)
					}
					if rateErr.RetryAfter != 30*time.Second {
						t.Errorf("RetryAfter = %v, want 30s", rateErr.RetryAfter)
					}
				}
				if tc.wantNotReg {
					var notReg *ErrOrderNotRegistered
					if !errors.As(err, &notReg) {
						t.Fatalf("ожидалась ErrOrderNotRegistered, got %T", err)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}

			if result.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", result.Status, tc.wantStatus)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"число", "30", 30 * time.Second},
		{"пустой", "", 60 * time.Second},
		{"невалидный", "abc", 60 * time.Second},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseRetryAfter(tc.header)
			if got != tc.want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}
