// Package middleware содержит HTTP middleware для сервиса.
package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"
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
func GzipMiddleware(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Распаковка входящего запроса.
			if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
				gr, err := gzip.NewReader(r.Body)
				if err != nil {
					http.Error(w, "failed to decompress request", http.StatusBadRequest)
					return
				}
				defer func() {
					// Close у gzip.Reader валидирует CRC-checksum; тело уже прочитано
					// хендлером, на HTTP-статус ошибка не влияет, но сигнализирует
					// о битом gzip от клиента.
					if cerr := gr.Close(); cerr != nil {
						log.Warn("закрытие gzip reader", zap.Error(cerr))
					}
				}()
				r.Body = gr
			}

			// Vary: Accept-Encoding — ответ зависит от этого заголовка запроса.
			w.Header().Add("Vary", "Accept-Encoding")

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
			defer func() {
				// Close у gzip.Writer дописывает финальный блок и футер;
				// ошибка здесь означает, что клиенту ушёл обрезанный gzip-поток.
				if cerr := gz.Close(); cerr != nil {
					log.Warn("закрытие gzip writer", zap.Error(cerr))
				}
			}()

			w.Header().Set("Content-Encoding", "gzip")
			// Удаляем Content-Length после сжатия.
			w.Header().Del("Content-Length")

			next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, writer: gz}, r)
		})
	}
}
