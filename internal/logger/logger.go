// Package logger предоставляет инициализацию zap-логгера для всего приложения.
package logger

import (
	"go.uber.org/zap"
)

func New(level string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()

	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return nil, err
	}
	cfg.Level = lvl

	return cfg.Build()
}
