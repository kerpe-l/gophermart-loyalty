// Package accrual реализует HTTP-клиент к внешнему сервису расчёта начислений.
package accrual

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// OrderStatus — статус заказа в системе начислений.
type OrderStatus string

const (
	StatusRegistered OrderStatus = "REGISTERED"
	StatusInvalid    OrderStatus = "INVALID"
	StatusProcessing OrderStatus = "PROCESSING"
	StatusProcessed  OrderStatus = "PROCESSED"
)

// OrderResult — ответ accrual-сервиса по конкретному заказу.
type OrderResult struct {
	Order   string      `json:"order"`
	Status  OrderStatus `json:"status"`
	Accrual *float64    `json:"accrual,omitempty"`
}

// ErrTooManyRequests сигнализирует о превышении лимита запросов (429).
type ErrTooManyRequests struct {
	RetryAfter time.Duration
}

func (e *ErrTooManyRequests) Error() string {
	return fmt.Sprintf("too many requests, retry after %s", e.RetryAfter)
}

// ErrOrderNotRegistered — заказ не зарегистрирован в accrual (204).
type ErrOrderNotRegistered struct{}

func (e *ErrOrderNotRegistered) Error() string {
	return "order not registered in accrual system"
}

// Client — HTTP-клиент к accrual-сервису.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient создаёт клиент к accrual-сервису.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetOrderAccrual запрашивает информацию о начислении по номеру заказа.
// Возвращает:
//   - *OrderResult, nil — при успешном ответе (200)
//   - nil, *ErrOrderNotRegistered — заказ не найден (204)
//   - nil, *ErrTooManyRequests — превышен лимит (429), с Retry-After
//   - nil, error — при 5xx или сетевых ошибках
func (c *Client) GetOrderAccrual(ctx context.Context, orderNumber string) (*OrderResult, error) {
	url := fmt.Sprintf("%s/api/orders/%s", c.baseURL, orderNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("создание запроса: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("выполнение запроса к accrual: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var result OrderResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("декодирование ответа accrual: %w", err)
		}
		return &result, nil

	case http.StatusNoContent:
		return nil, &ErrOrderNotRegistered{}

	case http.StatusTooManyRequests:
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, &ErrTooManyRequests{RetryAfter: retryAfter}

	default:
		return nil, fmt.Errorf("accrual вернул статус %d", resp.StatusCode)
	}
}

// parseRetryAfter разбирает заголовок Retry-After (в секундах).
// При ошибке парсинга возвращает 60 секунд по умолчанию.
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 60 * time.Second
	}
	seconds, err := strconv.Atoi(header)
	if err != nil {
		return 60 * time.Second
	}
	return time.Duration(seconds) * time.Second
}
