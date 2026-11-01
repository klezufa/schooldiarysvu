package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"schooldiarysvu/internal/metrics"
)

type contextKey string

const requestIDKey contextKey = "request_id"

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func Middleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := r.Header.Get("X-Request-ID")
		if len(requestID) < 8 || len(requestID) > 128 {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		// Обёртка запоминает код ответа для журнала, сохраняя Flusher/Hijacker исходного ResponseWriter.
		wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(wrapped, r.WithContext(ctx))
		status := wrapped.Status()
		if status == 0 {
			status = http.StatusOK
		}
		// Одна строка на запрос: без query-строки и тела (в них могут быть персональные данные), с кодом ответа.
		level := slog.LevelInfo
		switch {
		case status == StatusClientClosedRequest:
			level = slog.LevelInfo
		case status >= http.StatusInternalServerError:
			level = slog.LevelError
		case status >= http.StatusBadRequest:
			level = slog.LevelWarn
		}
		logger.Log(ctx, level, "http request completed", "request_id", requestID, "method", r.Method, "path", r.URL.Path,
			"status", status, "bytes", wrapped.BytesWritten(), "duration_ms", time.Since(started).Milliseconds())
	})
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(value[:])
}

// WithMetrics учитывает каждый запрос в Collector: маршрут (по шаблону chi, без идентификаторов), код ответа и длительность.
// Контекст маршрутизации создаётся здесь заранее, чтобы после обработки прочитать совпавший шаблон.
func WithMetrics(collector *metrics.Collector, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		routeContext := chi.NewRouteContext()
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeContext))
		collector.Begin()
		defer collector.End()
		wrapped := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(wrapped, r)
		route := routeContext.RoutePattern()
		if route == "" {
			route = "(статические файлы)"
		}
		collector.Observe(r.Method+" "+route, wrapped.Status(), time.Since(started))
	})
}
