// Package model содержит доменные типы системы лояльности.
package model

import "time"

// User — зарегистрированный пользователь системы.
type User struct {
	ID        int64
	Login     string
	Password  []byte // bcrypt-хеш
	CreatedAt time.Time
}

// OrderStatus — статус обработки заказа в системе начислений.
type OrderStatus string

const (
	OrderStatusNew        OrderStatus = "NEW"
	OrderStatusProcessing OrderStatus = "PROCESSING"
	OrderStatusInvalid    OrderStatus = "INVALID"
	OrderStatusProcessed  OrderStatus = "PROCESSED"
)

// IsFinal возвращает true для терминальных статусов (INVALID, PROCESSED).
func (s OrderStatus) IsFinal() bool {
	return s == OrderStatusInvalid || s == OrderStatusProcessed
}

// Order — загруженный номер заказа с информацией о начислении.
type Order struct {
	ID         int64
	UserID     int64
	Number     string
	Status     OrderStatus
	Accrual    int64 // в копейках
	UploadedAt time.Time
}

// Withdrawal — факт списания баллов в счёт оплаты заказа.
type Withdrawal struct {
	ID          int64
	UserID      int64
	OrderNumber string
	Amount      int64 // в копейках
	ProcessedAt time.Time
}

// Balance — текущее состояние счёта пользователя.
type Balance struct {
	Current   int64 // доступные баллы в копейках
	Withdrawn int64 // суммарно списано в копейках
}
