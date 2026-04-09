package postgres

import (
	"context"
	"fmt"

	"github.com/kerpe-l/gophermart-loyalty/internal/apperrors"
	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

// GetBalance возвращает текущий баланс пользователя.
// Баланс рассчитывается динамически: сумма начислений − сумма списаний.
func (s *Storage) GetBalance(ctx context.Context, userID int64) (*model.Balance, error) {
	var bal model.Balance

	err := s.pool.QueryRow(ctx,
		`SELECT
			COALESCE(SUM(o.accrual), 0),
			COALESCE(w.total, 0)
		 FROM orders o
		 LEFT JOIN (
			SELECT user_id, SUM(amount) AS total
			FROM withdrawals
			WHERE user_id = $1
			GROUP BY user_id
		 ) w ON w.user_id = o.user_id
		 WHERE o.user_id = $1`,
		userID,
	).Scan(&bal.Current, &bal.Withdrawn)

	if err != nil {
		// Если у пользователя нет ни заказов, ни списаний — вернём нулевой баланс.
		return &model.Balance{}, nil
	}

	bal.Current -= bal.Withdrawn
	return &bal, nil
}

// CreateWithdrawal списывает баллы со счёта пользователя.
// Выполняется в транзакции с блокировкой FOR UPDATE для защиты от гонки
// параллельных списаний.
func (s *Storage) CreateWithdrawal(ctx context.Context, userID int64, orderNumber string, amount int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("начало транзакции: %w", err)
	}
	defer func() {
		// Откатываем, если коммит не был вызван.
		_ = tx.Rollback(ctx) // nolint: после Commit Rollback — no-op
	}()

	// Считаем баланс внутри транзакции с блокировкой строк заказов.
	var accrued int64
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(accrual), 0)
		 FROM orders
		 WHERE user_id = $1
		 FOR UPDATE`,
		userID,
	).Scan(&accrued)
	if err != nil {
		return fmt.Errorf("подсчёт начислений: %w", err)
	}

	var withdrawn int64
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0)
		 FROM withdrawals
		 WHERE user_id = $1
		 FOR UPDATE`,
		userID,
	).Scan(&withdrawn)
	if err != nil {
		return fmt.Errorf("подсчёт списаний: %w", err)
	}

	if accrued-withdrawn < amount {
		return apperrors.ErrInsufficientFunds
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO withdrawals (user_id, order_number, amount)
		 VALUES ($1, $2, $3)`,
		userID, orderNumber, amount,
	)
	if err != nil {
		return fmt.Errorf("создание списания: %w", err)
	}

	return tx.Commit(ctx)
}

// GetWithdrawalsByUserID возвращает историю списаний, от новых к старым.
func (s *Storage) GetWithdrawalsByUserID(ctx context.Context, userID int64) ([]model.Withdrawal, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, order_number, amount, processed_at
		 FROM withdrawals
		 WHERE user_id = $1
		 ORDER BY processed_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("получение списаний: %w", err)
	}
	defer rows.Close()

	var wds []model.Withdrawal
	for rows.Next() {
		var w model.Withdrawal
		if err := rows.Scan(&w.ID, &w.UserID, &w.OrderNumber, &w.Amount, &w.ProcessedAt); err != nil {
			return nil, fmt.Errorf("сканирование списания: %w", err)
		}
		wds = append(wds, w)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("итерация списаний: %w", err)
	}

	return wds, nil
}
