// Package config отвечает за разбор конфигурации приложения из флагов и переменных окружения.
package config

import (
	"flag"
	"os"
)

// Config содержит параметры запуска сервиса.
type Config struct {
	RunAddress           string
	DatabaseURI          string
	AccrualSystemAddress string
}

// New разбирает флаги командной строки и переменные окружения.
// Переменные окружения имеют приоритет над флагами.
func New() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.RunAddress, "a", "localhost:8080", "адрес и порт запуска сервиса")
	flag.StringVar(&cfg.DatabaseURI, "d", "", "адрес подключения к базе данных")
	flag.StringVar(&cfg.AccrualSystemAddress, "r", "", "адрес системы расчёта начислений")
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

	return cfg
}
