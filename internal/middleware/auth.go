package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/kerpe-l/gophermart-loyalty/internal/auth"
)

type contextKey int

const userIDKey contextKey = iota

// AuthMiddleware проверяет JWT из заголовка Authorization
// и кладёт user ID в context.
func AuthMiddleware(authMgr *auth.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(header, "Bearer ")
			if tokenStr == header {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			userID, err := authMgr.ParseToken(tokenStr)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext извлекает user ID из контекста.
// Возвращает 0, false если ID отсутствует.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey).(int64)
	return id, ok
}
