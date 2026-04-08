package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// statusResponseWriter запоминает код ответа и количество записанных байт.
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

func LoggingMiddleware(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			sw := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(sw, r)

			log.Info("HTTP-запрос",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", sw.statusCode),
				zap.Int("bytes", sw.bytes),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}
