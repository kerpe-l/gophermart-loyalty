package accrual

import (
	"context"
	"errors"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/zap"

	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

// roundTripFunc — in-memory http.RoundTripper. Нужен для synctest-тестов.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// newStubClient — Client, в котором HTTP идёт через rt без реальной сети.
func newStubClient(rt http.RoundTripper) *Client {
	return &Client{
		baseURL:    "http://stub",
		httpClient: &http.Client{Transport: rt, Timeout: 10 * time.Second},
	}
}

// jsonResponse — хелпер для http.Response.
func jsonResponse(status int, body string) *http.Response {
	resp := &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	return resp
}

// mockOrderStore — мок хранилища для тестов поллера.
type mockOrderStore struct {
	getPendingFn   func(ctx context.Context) ([]model.Order, error)
	updateStatusFn func(ctx context.Context, number string, status model.OrderStatus, accrual int64) error
}

func (m *mockOrderStore) GetPendingOrders(ctx context.Context) iter.Seq2[model.Order, error] {
	return func(yield func(model.Order, error) bool) {
		orders, err := m.getPendingFn(ctx)
		if err != nil {
			yield(model.Order{}, err)
			return
		}
		for _, o := range orders {
			if !yield(o, nil) {
				return
			}
		}
	}
}

func (m *mockOrderStore) UpdateOrderStatus(ctx context.Context, number string, status model.OrderStatus, accrual int64) error {
	return m.updateStatusFn(ctx, number, status, accrual)
}

func TestPollerProcessesOrders(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var updated atomic.Int32

		rt := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK,
				`{"order":"12345678903","status":"PROCESSED","accrual":100.50}`), nil
		})

		store := &mockOrderStore{
			getPendingFn: func(_ context.Context) ([]model.Order, error) {
				if updated.Load() > 0 {
					return nil, nil // после первого обновления — пусто
				}
				return []model.Order{{Number: "12345678903", Status: model.OrderStatusNew}}, nil
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

		poller := NewPoller(newStubClient(rt), store, zap.NewNop())

		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan error, 1)
		go func() {
			done <- poller.Run(ctx)
		}()

		// Виртуальный sleep.
		time.Sleep(defaultPollInterval + time.Second)
		synctest.Wait()

		if updated.Load() == 0 {
			t.Error("заказ не был обновлён")
		}

		cancel()
		if err := <-done; err != nil {
			t.Fatalf("Run вернул ошибку: %v", err)
		}
	})
}

func TestPollerHandles429(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var requestCount atomic.Int32

		rt := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			requestCount.Add(1)
			resp := &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"Retry-After": []string{"1"}},
				Body:       io.NopCloser(strings.NewReader("")),
			}
			return resp, nil
		})

		store := &mockOrderStore{
			getPendingFn: func(_ context.Context) ([]model.Order, error) {
				return []model.Order{{Number: "12345678903", Status: model.OrderStatusNew}}, nil
			},
			updateStatusFn: func(_ context.Context, _ string, _ model.OrderStatus, _ int64) error {
				return nil
			},
		}

		poller := NewPoller(newStubClient(rt), store, zap.NewNop())

		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan error, 1)
		go func() {
			done <- poller.Run(ctx)
		}()

		// Виртуальный sleep.
		time.Sleep(3 * time.Second)

		cancel()
		if err := <-done; err != nil {
			t.Fatalf("Run вернул ошибку: %v", err)
		}

		if cnt := requestCount.Load(); cnt > 5 {
			t.Errorf("слишком много запросов при 429: %d", cnt)
		}
	})
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

func TestPollerBackoffOnUpdateError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
			return errors.New("imitation: БД недоступна")
		},
	}

	client := NewClient(srv.URL)
	poller := NewPoller(client, store, zap.NewNop())

	first := poller.poll(context.Background())
	if poller.consecutiveErrors != 1 {
		t.Errorf("consecutiveErrors после первой ошибки = %d, want 1", poller.consecutiveErrors)
	}

	second := poller.poll(context.Background())
	if second <= first {
		t.Errorf("backoff должен расти: first=%v, second=%v", first, second)
	}
	if poller.consecutiveErrors != 2 {
		t.Errorf("consecutiveErrors после второй ошибки = %d, want 2", poller.consecutiveErrors)
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
