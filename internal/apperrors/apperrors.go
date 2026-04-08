// Package apperrors определяет доменные ошибки и единый маппинг в HTTP-статусы.
package apperrors

import (
	"errors"
	"net/http"
)

// Sentinel-ошибки бизнес-логики.
var (
	ErrUserExists          = errors.New("пользователь с таким логином уже существует")
	ErrInvalidCredentials  = errors.New("неверная пара логин/пароль")
	ErrOrderAlreadyOwned   = errors.New("заказ уже загружен этим пользователем")
	ErrOrderOwnedByAnother = errors.New("заказ загружен другим пользователем")
	ErrInsufficientFunds   = errors.New("недостаточно средств")
	ErrInvalidOrderNumber  = errors.New("неверный формат номера заказа")
)

// HTTPStatus возвращает HTTP-статус, соответствующий доменной ошибке.
// Для неизвестных ошибок возвращает 500.
func HTTPStatus(err error) int {
	switch {
	case errors.Is(err, ErrUserExists):
		return http.StatusConflict // 409
	case errors.Is(err, ErrInvalidCredentials):
		return http.StatusUnauthorized // 401
	case errors.Is(err, ErrOrderAlreadyOwned):
		return http.StatusOK // 200
	case errors.Is(err, ErrOrderOwnedByAnother):
		return http.StatusConflict // 409
	case errors.Is(err, ErrInsufficientFunds):
		return http.StatusPaymentRequired // 402
	case errors.Is(err, ErrInvalidOrderNumber):
		return http.StatusUnprocessableEntity // 422
	default:
		return http.StatusInternalServerError // 500
	}
}
