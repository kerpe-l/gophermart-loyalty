package postgres

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kerpe-l/gophermart-loyalty/internal/apperrors"
	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

// scanOrder — сканирование одной строки в model.Order.
func scanOrder(rows pgx.Rows) (model.Order, error) {
	var o model.Order
	err := rows.Scan(&o.ID, &o.UserID, &o.Number, &o.Status, &o.Accrual, &o.UploadedAt)
	return o, err
}

// CreateOrder создаёт заказ для пользователя.
// Если номер уже существует у этого пользователя — возвращает ErrOrderAlreadyOwned.
// Если номер принадлежит другому пользователю — возвращает ErrOrderOwnedByAnother.
func (s *Storage) CreateOrder(ctx context.Context, userID int64, number string) (*model.Order, error) {
	order := &model.Order{
		UserID: userID,
		Number: number,
		Status: model.OrderStatusNew,
	}

	err := s.pool.QueryRow(ctx,
		`INSERT INTO orders (user_id, number, status)
		 VALUES ($1, $2, $3)
		 RETURNING id, uploaded_at`,
		userID, number, model.OrderStatusNew,
	).Scan(&order.ID, &order.UploadedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		// 23505 — unique_violation (orders.number UNIQUE)
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, s.orderConflictError(ctx, number, userID)
		}
		return nil, fmt.Errorf("создание заказа: %w", err)
	}

	return order, nil
}

// orderConflictError определяет, кому принадлежит существующий заказ.
func (s *Storage) orderConflictError(ctx context.Context, number string, userID int64) error {
	var ownerID int64
	err := s.pool.QueryRow(ctx,
		`SELECT user_id FROM orders WHERE number = $1`, number,
	).Scan(&ownerID)
	if err != nil {
		return fmt.Errorf("проверка владельца заказа: %w", err)
	}

	if ownerID == userID {
		return apperrors.ErrOrderAlreadyOwned
	}
	return apperrors.ErrOrderOwnedByAnother
}

// GetPendingOrders возвращает заказы со статусом NEW или PROCESSING как стримящий итератор.
func (s *Storage) GetPendingOrders(ctx context.Context) iter.Seq2[model.Order, error] {
	return func(yield func(model.Order, error) bool) {
		rows, err := s.pool.Query(ctx,
			`SELECT id, user_id, number, status, accrual, uploaded_at
			 FROM orders
			 WHERE status IN ($1, $2)
			 ORDER BY uploaded_at ASC`,
			model.OrderStatusNew, model.OrderStatusProcessing,
		)
		if err != nil {
			yield(model.Order{}, fmt.Errorf("получение незавершённых заказов: %w", err))
			return
		}

		for v, err := range scanRows(rows, scanOrder) {
			if !yield(v, err) {
				return
			}
		}
	}
}

// UpdateOrderStatus атомарно обновляет статус и начисление заказа.
// WHERE status NOT IN (INVALID, PROCESSED) защищает от двойного начисления.
func (s *Storage) UpdateOrderStatus(ctx context.Context, number string, status model.OrderStatus, accrual int64) error {
	res, err := s.pool.Exec(ctx,
		`UPDATE orders
		 SET status = $1, accrual = $2
		 WHERE number = $3 AND status NOT IN ($4, $5)`,
		status, accrual, number, model.OrderStatusInvalid, model.OrderStatusProcessed,
	)
	if err != nil {
		return fmt.Errorf("обновление статуса заказа: %w", err)
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("заказ %s уже в финальном статусе", number)
	}

	return nil
}

// GetOrdersByUserID возвращает заказы пользователя, отсортированные от новых к старым.
func (s *Storage) GetOrdersByUserID(ctx context.Context, userID int64) ([]model.Order, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, number, status, accrual, uploaded_at
		 FROM orders
		 WHERE user_id = $1
		 ORDER BY uploaded_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("получение заказов пользователя: %w", err)
	}

	orders, err := collectRows(scanRows(rows, scanOrder))
	if err != nil {
		return nil, fmt.Errorf("чтение заказов пользователя: %w", err)
	}
	return orders, nil
}
