package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kerpe-l/gophermart-loyalty/internal/apperrors"
	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

// CreateUser создаёт пользователя и возвращает его с заполненным ID.
// При дублировании логина возвращает apperrors.ErrUserExists.
func (s *Storage) CreateUser(ctx context.Context, login string, passwordHash []byte) (*model.User, error) {
	user := &model.User{
		Login:    login,
		Password: passwordHash,
	}

	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (login, password) VALUES ($1, $2)
		 RETURNING id, created_at`,
		login, passwordHash,
	).Scan(&user.ID, &user.CreatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		// 23505 — unique_violation
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, apperrors.ErrUserExists
		}
		return nil, fmt.Errorf("создание пользователя: %w", err)
	}

	return user, nil
}

// GetUserByLogin возвращает пользователя по логину.
// Если пользователь не найден, возвращает apperrors.ErrInvalidCredentials.
func (s *Storage) GetUserByLogin(ctx context.Context, login string) (*model.User, error) {
	user := &model.User{Login: login}

	err := s.pool.QueryRow(ctx,
		`SELECT id, password, created_at FROM users WHERE login = $1`,
		login,
	).Scan(&user.ID, &user.Password, &user.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("получение пользователя по логину: %w", err)
	}

	return user, nil
}
