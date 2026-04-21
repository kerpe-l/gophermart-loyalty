// Package postgres реализует хранилище на базе PostgreSQL.
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql-драйвер pgx для goose
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Storage — хранилище данных на PostgreSQL.
type Storage struct {
	pool *pgxpool.Pool
}

// New накатывает миграции goose и открывает пул соединений pgx.
func New(ctx context.Context, dsn string) (*Storage, error) {
	if err := runMigrations(ctx, dsn); err != nil {
		return nil, fmt.Errorf("применение миграций: %w", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("подключение к БД: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("пинг БД: %w", err)
	}

	return &Storage{pool: pool}, nil
}

// Close закрывает пул соединений.
func (s *Storage) Close() {
	s.pool.Close()
}

// runMigrations накатывает все миграции до последней версии.
func runMigrations(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("открытие sql.DB: %w", err)
	}
	defer func() { _ = db.Close() }()

	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("sub-fs миграций: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations)
	if err != nil {
		return fmt.Errorf("инициализация goose: %w", err)
	}

	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("up: %w", err)
	}
	return nil
}
