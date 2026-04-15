package accrual

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

// mockOrderStore — мок хранилища для тестов поллера.
type mockOrderStore struct {
	getPendingFn   func(ctx context.Context) ([]model.Order, error)
	updateStatusFn func(ctx context.Context, number string, status model.OrderStatus, accrual int64) error
}

func (m *mockOrderStore) GetPendingOrders(ctx context.Context) ([]model.Order, error) {
	return m.getPendingFn(ctx)
}

func (m *mockOrderStore) UpdateOrderStatus(ctx context.Context, number string, status model.OrderStatus, accrual int64) error {
	return m.updateStatusFn(ctx, number, status, accrual)
}

func TestPollerProcessesOrders(t *testing.T) {
	t.Parallel()

	var updated atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"order":"12345678903","status":"PROCESSED","accrual":100.50}`))
	}))
	defer srv.Close()

	store := &mockOrderStore{
		getPendingFn: func(_ context.Context) ([]model.Order, error) {
			if updated.Load() > 0 {
				return nil, nil // после первого обновления — пусто
			}
			return []model.Order{
				{Number: "12345678903", Status: model.OrderStatusNew},
			}, nil
		},
		updateStatusFn: func(_ context.Context, number string, status model.OrderStatus, accrualVal int64) error {
			if number != "12345678903" {
				t.Errorf("number = %q, want 12345678903", number)
			}
			if status != model.OrderStatusProcessed {
				t.Errorf("status = %q, want PROCESSED", status)
			}
			if accrualVal != 10050 {
				t.Errorf("accrual = %d, want 10050", accrualVal)
			}
			updated.Add(1)
			return nil
		},
	}

	client := NewClient(srv.URL)
	poller := NewPoller(client, store, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		// Ждём обновления и отменяем контекст.
		for updated.Load() == 0 {
			time.Sleep(50 * time.Millisecond)
		}
		cancel()
	}()

	err := poller.Run(ctx)
	if err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}

	if updated.Load() == 0 {
		t.Error("заказ не был обновлён")
	}
}

func TestPollerHandles429(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	store := &mockOrderStore{
		getPendingFn: func(_ context.Context) ([]model.Order, error) {
			return []model.Order{
				{Number: "12345678903", Status: model.OrderStatusNew},
			}, nil
		},
		updateStatusFn: func(_ context.Context, _ string, _ model.OrderStatus, _ int64) error {
			return nil
		},
	}

	client := NewClient(srv.URL)
	poller := NewPoller(client, store, zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := poller.Run(ctx)
	if err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}

	// При 429 поллер не должен долбить сервис.
	if cnt := requestCount.Load(); cnt > 5 {
		t.Errorf("слишком много запросов при 429: %d", cnt)
	}
}

func TestPollerSkipsUpdateWhenStatusUnchanged(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"order":"12345678903","status":"REGISTERED"}`))
	}))
	defer srv.Close()

	var updateCalls atomic.Int32

	store := &mockOrderStore{
		getPendingFn: func(_ context.Context) ([]model.Order, error) {
			return []model.Order{{Number: "12345678903", Status: model.OrderStatusNew}}, nil
		},
		updateStatusFn: func(_ context.Context, _ string, _ model.OrderStatus, _ int64) error {
			updateCalls.Add(1)
			return nil
		},
	}

	client := NewClient(srv.URL)
	poller := NewPoller(client, store, zap.NewNop())

	if next := poller.poll(context.Background()); next != defaultPollInterval {
		t.Errorf("poll должен вернуть defaultPollInterval на штатном ответе, got %v", next)
	}

	if n := updateCalls.Load(); n != 0 {
		t.Errorf("UpdateOrderStatus не должен вызываться для REGISTERED → NEW, вызван %d раз", n)
	}
}

func TestPollerBackoffPersistsAndResets(t *testing.T) {
	t.Parallel()

	var failing atomic.Bool
	failing.Store(true)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"order":"12345678903","status":"PROCESSED","accrual":1.0}`))
	}))
	defer srv.Close()

	store := &mockOrderStore{
		getPendingFn: func(_ context.Context) ([]model.Order, error) {
			return []model.Order{{Number: "12345678903", Status: model.OrderStatusNew}}, nil
		},
		updateStatusFn: func(_ context.Context, _ string, _ model.OrderStatus, _ int64) error {
			return nil
		},
	}

	client := NewClient(srv.URL)
	poller := NewPoller(client, store, zap.NewNop())

	ctx := context.Background()

	first := poller.poll(ctx)
	second := poller.poll(ctx)
	if first >= second {
		t.Errorf("backoff должен расти между вызовами: first=%v, second=%v", first, second)
	}

	failing.Store(false)
	third := poller.poll(ctx)
	if third != defaultPollInterval {
		t.Errorf("после успешного цикла должен вернуться defaultPollInterval, got %v", third)
	}
}

func TestMapAccrualStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   OrderStatus
		want model.OrderStatus
	}{
		{StatusProcessed, model.OrderStatusProcessed},
		{StatusInvalid, model.OrderStatusInvalid},
		{StatusProcessing, model.OrderStatusProcessing},
		{StatusRegistered, model.OrderStatusNew},
	}

	for _, tc := range tests {
		t.Run(string(tc.in), func(t *testing.T) {
			t.Parallel()
			got := mapAccrualStatus(tc.in)
			if got != tc.want {
				t.Errorf("mapAccrualStatus(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
