package auth

import (
	"testing"
)

func TestHashAndComparePassword(t *testing.T) {
	t.Parallel()
	mgr := NewManager("test-secret")

	tests := []struct {
		name     string
		password string
	}{
		{name: "обычный пароль", password: "qwerty123"},
		{name: "длинный пароль", password: "a-very-long-password-with-special-chars!@#$%"},
		{name: "unicode пароль", password: "пароль123"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hash, err := mgr.HashPassword(tc.password)
			if err != nil {
				t.Fatalf("HashPassword(%q): %v", tc.password, err)
			}

			if err := mgr.ComparePassword(hash, tc.password); err != nil {
				t.Errorf("ComparePassword: правильный пароль не совпал: %v", err)
			}

			if err := mgr.ComparePassword(hash, tc.password+"wrong"); err == nil {
				t.Errorf("ComparePassword: неверный пароль не вернул ошибку")
			}
		})
	}
}

func TestGenerateAndParseToken(t *testing.T) {
	t.Parallel()
	mgr := NewManager("test-secret")

	tests := []struct {
		name   string
		userID int64
	}{
		{name: "обычный ID", userID: 42},
		{name: "большой ID", userID: 9999999},
		{name: "ID = 1", userID: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			token, err := mgr.GenerateToken(tc.userID)
			if err != nil {
				t.Fatalf("GenerateToken(%d): %v", tc.userID, err)
			}

			got, err := mgr.ParseToken(token)
			if err != nil {
				t.Fatalf("ParseToken: %v", err)
			}
			if got != tc.userID {
				t.Errorf("ParseToken = %d, want %d", got, tc.userID)
			}
		})
	}
}

func TestParseToken_невалидный(t *testing.T) {
	t.Parallel()
	mgr := NewManager("test-secret")

	tests := []struct {
		name  string
		token string
	}{
		{name: "пустая строка", token: ""},
		{name: "мусор", token: "not-a-jwt"},
		{name: "другой секрет", token: func() string {
			other := NewManager("other-secret")
			tok, _ := other.GenerateToken(1)
			return tok
		}()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := mgr.ParseToken(tc.token)
			if err == nil {
				t.Error("ParseToken: ожидалась ошибка для невалидного токена")
			}
		})
	}
}
