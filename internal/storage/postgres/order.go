package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kerpe-l/gophermart-loyalty/internal/apperrors"
	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

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
	defer rows.Close()

	var orders []model.Order
	for rows.Next() {
		var o model.Order
		if err := rows.Scan(&o.ID, &o.UserID, &o.Number, &o.Status, &o.Accrual, &o.UploadedAt); err != nil {
			return nil, fmt.Errorf("сканирование заказа: %w", err)
		}
		orders = append(orders, o)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("итерация заказов: %w", err)
	}

	return orders, nil
}
