package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/kerpe-l/gophermart-loyalty/internal/config"
	"github.com/kerpe-l/gophermart-loyalty/internal/logger"
	"github.com/kerpe-l/gophermart-loyalty/internal/middleware"
	"github.com/kerpe-l/gophermart-loyalty/internal/storage/postgres"
)

func main() {
	cfg := config.New()

	zapLog, err := logger.New("info")
	if err != nil {
		log.Fatal("инициализация логгера: ", err)
	}
	defer func() {
		// Игнорируем ошибку Sync — stderr часто не syncable.
		_ = zapLog.Sync()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := postgres.New(ctx, cfg.DatabaseURI)
	if err != nil {
		zapLog.Fatal("инициализация хранилища", zap.Error(err))
	}
	defer store.Close()

	r := chi.NewRouter()
	r.Use(middleware.LoggingMiddleware(zapLog))
	r.Use(middleware.GzipMiddleware)

	srv := &http.Server{
		Addr:              cfg.RunAddress,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		zapLog.Info("сервер запускается", zap.String("addr", cfg.RunAddress))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			zapLog.Fatal("ошибка сервера", zap.Error(err))
		}
	}()

	<-ctx.Done()
	zapLog.Info("получен сигнал завершения, останавливаю сервер...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		zapLog.Error("ошибка при остановке сервера", zap.Error(err))
	}

	zapLog.Info("сервер остановлен")
}
