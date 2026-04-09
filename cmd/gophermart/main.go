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

	"github.com/kerpe-l/gophermart-loyalty/internal/auth"
	"github.com/kerpe-l/gophermart-loyalty/internal/config"
	"github.com/kerpe-l/gophermart-loyalty/internal/handler"
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

	// TODO: вынести секрет в конфиг/env
	authMgr := auth.NewManager("gophermart-secret-key")
	userHandler := handler.NewUserHandler(store, authMgr, zapLog)
	orderHandler := handler.NewOrderHandler(store, zapLog)
	balanceHandler := handler.NewBalanceHandler(store, zapLog)

	r := chi.NewRouter()
	r.Use(middleware.LoggingMiddleware(zapLog))
	r.Use(middleware.GzipMiddleware)

	r.Post("/api/user/register", userHandler.Register)
	r.Post("/api/user/login", userHandler.Login)

	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(authMgr))
		r.Post("/api/user/orders", orderHandler.CreateOrder)
		r.Get("/api/user/orders", orderHandler.GetOrders)
		r.Get("/api/user/balance", balanceHandler.GetBalance)
		r.Post("/api/user/balance/withdraw", balanceHandler.Withdraw)
		r.Get("/api/user/withdrawals", balanceHandler.GetWithdrawals)
	})

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
