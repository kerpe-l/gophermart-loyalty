// Package middleware содержит HTTP middleware для сервиса.
package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// gzipResponseWriter оборачивает http.ResponseWriter для сжатия ответа.
type gzipResponseWriter struct {
	http.ResponseWriter
	writer io.Writer
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.writer.Write(b)
}

// GzipMiddleware сжимает ответы и распаковывает запросы с Content-Encoding: gzip.
func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Распаковка входящего запроса.
		if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
			gr, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "failed to decompress request", http.StatusBadRequest)
				return
			}
			defer gr.Close()
			r.Body = gr
		}

		// Сжатие ответа, если клиент поддерживает.
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		defer gz.Close()

		w.Header().Set("Content-Encoding", "gzip")
		// Удаляем Content-Length после сжатия.
		w.Header().Del("Content-Length")

		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, writer: gz}, r)
	})
}
