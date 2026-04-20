package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/kerpe-l/gophermart-loyalty/internal/apperrors"
	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

// GetBalance возвращает текущий баланс пользователя.
// Оба SELECT SUM выполняются в одной REPEATABLE READ транзакции, чтобы параллельный CreateWithdrawal не дал разъехаться accrued и withdrawn.
func (s *Storage) GetBalance(ctx context.Context, userID int64) (*model.Balance, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("начало транзакции: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var accrued int64
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(accrual), 0) FROM orders WHERE user_id = $1`,
		userID,
	).Scan(&accrued)
	if err != nil {
		return nil, fmt.Errorf("подсчёт начислений: %w", err)
	}

	var withdrawn int64
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM withdrawals WHERE user_id = $1`,
		userID,
	).Scan(&withdrawn)
	if err != nil {
		return nil, fmt.Errorf("подсчёт списаний: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("коммит транзакции баланса: %w", err)
	}

	return &model.Balance{
		Current:   accrued - withdrawn,
		Withdrawn: withdrawn,
	}, nil
}

// CreateWithdrawal списывает баллы со счёта пользователя.
// Сериализует параллельные списания одного пользователя через pg_advisory_xact_lock.
func (s *Storage) CreateWithdrawal(ctx context.Context, userID int64, orderNumber string, amount int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("начало транзакции: %w", err)
	}
	defer func() {
		// Откатываем, если коммит не был вызван.
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}

	var accrued int64
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(accrual), 0) FROM orders WHERE user_id = $1`,
		userID,
	).Scan(&accrued)
	if err != nil {
		return fmt.Errorf("подсчёт начислений: %w", err)
	}

	var withdrawn int64
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM withdrawals WHERE user_id = $1`,
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
