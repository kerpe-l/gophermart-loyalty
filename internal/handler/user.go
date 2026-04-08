package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"github.com/kerpe-l/gophermart-loyalty/internal/apperrors"
	"github.com/kerpe-l/gophermart-loyalty/internal/auth"
	"github.com/kerpe-l/gophermart-loyalty/internal/model"
)

// UserStore — интерфейс хранилища пользователей (consumer-side).
type UserStore interface {
	CreateUser(ctx context.Context, login string, passwordHash []byte) (*model.User, error)
	GetUserByLogin(ctx context.Context, login string) (*model.User, error)
}

// UserHandler обрабатывает запросы регистрации и аутентификации.
type UserHandler struct {
	store UserStore
	auth  *auth.Manager
	log   *zap.Logger
}

// NewUserHandler создаёт хендлер пользователей.
func NewUserHandler(store UserStore, authMgr *auth.Manager, log *zap.Logger) *UserHandler {
	return &UserHandler{
		store: store,
		auth:  authMgr,
		log:   log,
	}
}

type credentials struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// Register обрабатывает POST /api/user/register.
func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if creds.Login == "" || creds.Password == "" {
		http.Error(w, "login and password required", http.StatusBadRequest)
		return
	}

	hash, err := h.auth.HashPassword(creds.Password)
	if err != nil {
		h.log.Error("хеширование пароля", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	user, err := h.store.CreateUser(r.Context(), creds.Login, hash)
	if err != nil {
		if errors.Is(err, apperrors.ErrUserExists) {
			http.Error(w, "login already taken", apperrors.HTTPStatus(err))
			return
		}
		h.log.Error("создание пользователя", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.setAuthToken(w, user.ID)
	w.WriteHeader(http.StatusOK)
}

// Login обрабатывает POST /api/user/login.
func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if creds.Login == "" || creds.Password == "" {
		http.Error(w, "login and password required", http.StatusBadRequest)
		return
	}

	user, err := h.store.GetUserByLogin(r.Context(), creds.Login)
	if err != nil {
		if errors.Is(err, apperrors.ErrInvalidCredentials) {
			http.Error(w, "invalid credentials", apperrors.HTTPStatus(err))
			return
		}
		h.log.Error("получение пользователя", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := h.auth.ComparePassword(user.Password, creds.Password); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	h.setAuthToken(w, user.ID)
	w.WriteHeader(http.StatusOK)
}

func (h *UserHandler) setAuthToken(w http.ResponseWriter, userID int64) {
	token, err := h.auth.GenerateToken(userID)
	if err != nil {
		h.log.Error("генерация токена", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Authorization", "Bearer "+token)
}
