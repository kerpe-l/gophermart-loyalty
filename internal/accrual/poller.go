package accrual

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"go.uber.org/zap"

	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

const (
	defaultPollInterval = 2 * time.Second
	maxBackoff          = 60 * time.Second
)

// OrderStore — интерфейс хранилища, необходимый поллеру (consumer-side).
type OrderStore interface {
	GetPendingOrders(ctx context.Context) ([]model.Order, error)
	UpdateOrderStatus(ctx context.Context, number string, status model.OrderStatus, accrual int64) error
}

// Poller периодически опрашивает accrual-сервис и обновляет статусы заказов.
type Poller struct {
	client            *Client
	store             OrderStore
	log               *zap.Logger
	consecutiveErrors int
}

// NewPoller создаёт поллер.
func NewPoller(client *Client, store OrderStore, log *zap.Logger) *Poller {
	return &Poller{
		client: client,
		store:  store,
		log:    log,
	}
}

// Run запускает цикл опроса. Блокирует до отмены контекста.
// Предназначен для запуска в errgroup.
func (p *Poller) Run(ctx context.Context) error {
	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.log.Info("поллер остановлен")
			return nil
		case <-ticker.C:
			next := p.poll(ctx)
			ticker.Reset(next)
		}
	}
}

// poll проходит по незавершённым заказам и возвращает интервал до
// следующего тика. При 429 возвращает Retry-After, при сетевой/5xx ошибке - backoff.
// цикл прерывается на первой же ошибке, чтобы не добивать недоступный accrual оставшимися заказами.
func (p *Poller) poll(ctx context.Context) time.Duration {
	orders, err := p.store.GetPendingOrders(ctx)
	if err != nil {
		p.log.Error("получение незавершённых заказов", zap.Error(err))
		return defaultPollInterval
	}

	for _, order := range orders {
		if ctx.Err() != nil {
			return defaultPollInterval
		}

		err := p.processOrder(ctx, order)
		if err == nil {
			continue
		}

		var tooMany *ErrTooManyRequests
		if errors.As(err, &tooMany) {
			p.log.Warn("accrual: rate limit, ожидаем",
				zap.Duration("retry_after", tooMany.RetryAfter))
			return tooMany.RetryAfter
		}

		p.consecutiveErrors++
		backoff := calcBackoff(p.consecutiveErrors)
		p.log.Error("обработка заказа в accrual",
			zap.String("order", order.Number),
			zap.Error(err),
			zap.Duration("backoff", backoff))
		return backoff
	}

	// Полный проход без ошибок (или пустой список) — считаем accrual здоровым.
	p.consecutiveErrors = 0
	return defaultPollInterval
}

// processOrder запрашивает accrual и обновляет статус одного заказа.
func (p *Poller) processOrder(ctx context.Context, order model.Order) error {
	result, err := p.client.GetOrderAccrual(ctx, order.Number)
	if err != nil {
		var notRegistered *ErrOrderNotRegistered
		if errors.As(err, &notRegistered) {
			return nil // заказ ещё не попал в accrual, пропускаем
		}
		return err
	}

	newStatus := mapAccrualStatus(result.Status)
	var accrualKopecks int64
	if result.Accrual != nil {
		accrualKopecks = int64(math.Round(*result.Accrual * 100))
	}

	// Если ничего не поменялось — не дёргаем БД.
	if newStatus == order.Status && accrualKopecks == order.Accrual {
		return nil
	}

	if err := p.store.UpdateOrderStatus(ctx, order.Number, newStatus, accrualKopecks); err != nil {
		return fmt.Errorf("обновление статуса заказа %s: %w", order.Number, err)
	}

	return nil
}

// mapAccrualStatus конвертирует статус accrual-сервиса в доменный статус.
func mapAccrualStatus(s OrderStatus) model.OrderStatus {
	switch s {
	case StatusProcessed:
		return model.OrderStatusProcessed
	case StatusInvalid:
		return model.OrderStatusInvalid
	case StatusProcessing:
		return model.OrderStatusProcessing
	default:
		return model.OrderStatusNew
	}
}

// calcBackoff вычисляет задержку при последовательных ошибках.
func calcBackoff(consecutiveErrors int) time.Duration {
	backoff := time.Duration(1<<uint(consecutiveErrors)) * time.Second
	if backoff > maxBackoff {
		return maxBackoff
	}
	return backoff
}
