// Package auth реализует генерацию/валидацию JWT-токенов и хеширование паролей.
package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const tokenTTL = 24 * time.Hour

// Manager управляет JWT-токенами и хешированием паролей.
type Manager struct {
	secretKey []byte
}

// NewManager создаёт менеджер аутентификации с указанным секретным ключом.
func NewManager(secret string) *Manager {
	return &Manager{secretKey: []byte(secret)}
}

// HashPassword возвращает bcrypt-хеш пароля.
func (m *Manager) HashPassword(password string) ([]byte, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("хеширование пароля: %w", err)
	}
	return hash, nil
}

// ComparePassword сравнивает пароль с его bcrypt-хешем.
// Возвращает nil при совпадении.
func (m *Manager) ComparePassword(hash []byte, password string) error {
	return bcrypt.CompareHashAndPassword(hash, []byte(password))
}

// GenerateToken создаёт подписанный JWT с user ID в claims.
func (m *Manager) GenerateToken(userID int64) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   fmt.Sprintf("%d", userID),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString(m.secretKey)
	if err != nil {
		return "", fmt.Errorf("подпись токена: %w", err)
	}
	return signed, nil
}

// ParseToken валидирует JWT и возвращает user ID из claims.
func (m *Manager) ParseToken(tokenStr string) (int64, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("неожиданный метод подписи: %v", t.Header["alg"])
		}
		return m.secretKey, nil
	})
	if err != nil {
		return 0, fmt.Errorf("невалидный токен: %w", err)
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok {
		return 0, fmt.Errorf("невалидные claims токена")
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("невалидный subject токена: %w", err)
	}

	return userID, nil
}
