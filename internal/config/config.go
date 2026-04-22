// Package config отвечает за разбор конфигурации приложения из флагов и переменных окружения.
package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

const minJWTSecretLen = 32

var ErrJWTSecretMissing = errors.New("JWT-секрет не задан: укажите -j или JWT_SECRET")

// Config содержит параметры запуска сервиса.
type Config struct {
	RunAddress           string
	DatabaseURI          string
	AccrualSystemAddress string
	JWTSecret            string
}

// New разбирает флаги командной строки и переменные окружения и валидирует результат.
// Переменные окружения имеют приоритет над флагами.
// Возвращает ошибку, если JWT-секрет не задан или слишком короткий.
func New() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.RunAddress, "a", "localhost:8080", "адрес и порт запуска сервиса")
	flag.StringVar(&cfg.DatabaseURI, "d", "", "адрес подключения к базе данных")
	flag.StringVar(&cfg.AccrualSystemAddress, "r", "", "адрес системы расчёта начислений")
	flag.StringVar(&cfg.JWTSecret, "j", "", "секретный ключ для подписи JWT (мин. 32 байта)")
	flag.Parse()

	if v := os.Getenv("RUN_ADDRESS"); v != "" {
		cfg.RunAddress = v
	}
	if v := os.Getenv("DATABASE_URI"); v != "" {
		cfg.DatabaseURI = v
	}
	if v := os.Getenv("ACCRUAL_SYSTEM_ADDRESS"); v != "" {
		cfg.AccrualSystemAddress = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JWTSecret = v
	}

	if cfg.JWTSecret == "" {
		return nil, ErrJWTSecretMissing
	}
	if len(cfg.JWTSecret) < minJWTSecretLen {
		return nil, fmt.Errorf("JWT-секрет короче %d байт: длина %d", minJWTSecretLen, len(cfg.JWTSecret))
	}

	return cfg, nil
}
