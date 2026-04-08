// Package postgres реализует хранилище на базе PostgreSQL.
package postgres

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/00001_init.sql
var initSQL string

// Storage — хранилище данных на PostgreSQL.
type Storage struct {
	pool *pgxpool.Pool
}

// New создаёт пул соединений и применяет миграции.
func New(ctx context.Context, dsn string) (*Storage, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("подключение к БД: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("пинг БД: %w", err)
	}

	if _, err := pool.Exec(ctx, initSQL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("применение миграций: %w", err)
	}

	return &Storage{pool: pool}, nil
}

// Close закрывает пул соединений.
func (s *Storage) Close() {
	s.pool.Close()
}
