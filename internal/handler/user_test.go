package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/kerpe-l/gophermart-loyalty/internal/apperrors"
	"github.com/kerpe-l/gophermart-loyalty/internal/auth"
	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

// mockUserStore — мок хранилища пользователей для тестов хендлеров.
type mockUserStore struct {
	createUserFn     func(ctx context.Context, login string, hash []byte) (*model.User, error)
	getUserByLoginFn func(ctx context.Context, login string) (*model.User, error)
}

func (m *mockUserStore) CreateUser(ctx context.Context, login string, hash []byte) (*model.User, error) {
	return m.createUserFn(ctx, login, hash)
}

func (m *mockUserStore) GetUserByLogin(ctx context.Context, login string) (*model.User, error) {
	return m.getUserByLoginFn(ctx, login)
}

func TestRegister(t *testing.T) {
	t.Parallel()

	authMgr := auth.NewManager("test-secret")

	tests := []struct {
		name       string
		body       any
		store      *mockUserStore
		wantStatus int
		wantAuth   bool
	}{
		{
			name: "успешная регистрация",
			body: credentials{Login: "user1", Password: "pass123"},
			store: &mockUserStore{
				createUserFn: func(_ context.Context, login string, hash []byte) (*model.User, error) {
					return &model.User{ID: 1, Login: login, Password: hash}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantAuth:   true,
		},
		{
			name: "логин занят",
			body: credentials{Login: "user1", Password: "pass123"},
			store: &mockUserStore{
				createUserFn: func(_ context.Context, _ string, _ []byte) (*model.User, error) {
					return nil, apperrors.ErrUserExists
				},
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "пустой логин",
			body:       credentials{Login: "", Password: "pass123"},
			store:      &mockUserStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "пустой пароль",
			body:       credentials{Login: "user1", Password: ""},
			store:      &mockUserStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "невалидный JSON",
			body:       "not json",
			store:      &mockUserStore{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var body []byte
			switch v := tc.body.(type) {
			case string:
				body = []byte(v)
			default:
				body, _ = json.Marshal(v)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/user/register", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h := NewUserHandler(tc.store, authMgr, zap.NewNop())
			h.Register(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("статус = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantAuth {
				if a := rec.Header().Get("Authorization"); a == "" {
					t.Error("ожидался заголовок Authorization")
				}
			}
		})
	}
}

func TestLogin(t *testing.T) {
	t.Parallel()

	authMgr := auth.NewManager("test-secret")
	hash, _ := authMgr.HashPassword("pass123")

	tests := []struct {
		name       string
		body       any
		store      *mockUserStore
		wantStatus int
		wantAuth   bool
	}{
		{
			name: "успешный логин",
			body: credentials{Login: "user1", Password: "pass123"},
			store: &mockUserStore{
				getUserByLoginFn: func(_ context.Context, _ string) (*model.User, error) {
					return &model.User{ID: 1, Login: "user1", Password: hash}, nil
				},
			},
			wantStatus: http.StatusOK,
			wantAuth:   true,
		},
		{
			name: "пользователь не найден",
			body: credentials{Login: "nobody", Password: "pass123"},
			store: &mockUserStore{
				getUserByLoginFn: func(_ context.Context, _ string) (*model.User, error) {
					return nil, apperrors.ErrInvalidCredentials
				},
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "неверный пароль",
			body: credentials{Login: "user1", Password: "wrong"},
			store: &mockUserStore{
				getUserByLoginFn: func(_ context.Context, _ string) (*model.User, error) {
					return &model.User{ID: 1, Login: "user1", Password: hash}, nil
				},
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "пустое тело",
			body:       credentials{},
			store:      &mockUserStore{},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body, _ := json.Marshal(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			h := NewUserHandler(tc.store, authMgr, zap.NewNop())
			h.Login(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("статус = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantAuth {
				if a := rec.Header().Get("Authorization"); a == "" {
					t.Error("ожидался заголовок Authorization")
				}
			}
		})
	}
}
