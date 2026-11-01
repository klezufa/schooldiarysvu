package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"schooldiarysvu/internal/service"
)

type routerSettings struct {
	static     http.Handler
	trustProxy bool
}

// RouterOption настраивает необязательные части маршрутизатора.
type RouterOption func(*routerSettings)

// WithStaticHandler подключает раздачу frontend для всех путей, которые не относятся к API и healthcheck.
func WithStaticHandler(handler http.Handler) RouterOption {
	return func(settings *routerSettings) { settings.static = handler }
}

// WithTrustedProxy включает доверие к X-Real-IP (адрес клиента для ограничения попыток входа).
func WithTrustedProxy(enabled bool) RouterOption {
	return func(settings *routerSettings) { settings.trustProxy = enabled }
}

func NewRouter(pool *pgxpool.Pool, app *service.Service, cookieSecure bool, options ...RouterOption) http.Handler {
	var settings routerSettings
	for _, option := range options {
		option(&settings)
	}
	router := chi.NewRouter()
	api := &API{service: app, cookieSecure: cookieSecure, trustProxy: settings.trustProxy}
	router.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			WriteError(w, r, &Error{Status: http.StatusServiceUnavailable, Code: "database_unavailable", Message: "База данных недоступна"})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	router.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", api.login)
		r.With(api.authenticate).Get("/me", api.me)
		r.With(api.authenticate, api.requireCSRF).Post("/auth/logout", api.logout)
		r.With(api.authenticate, api.requireCSRF).Post("/auth/password", api.changePassword)
		r.Group(api.registerAcademicRoutes)
	})
	if settings.static != nil {
		router.NotFound(settings.static.ServeHTTP)
	}
	return router
}
