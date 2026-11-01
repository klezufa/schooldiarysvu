package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"schooldiarysvu/internal/metrics"
)

// WithMetrics полагается на то, что chi использует заранее положенный в контекст RouteContext.
// Тест фиксирует это поведение: при обновлении chi маршруты должны по-прежнему учитываться по шаблону.
func TestWithMetrics(t *testing.T) {
	t.Parallel()
	router := chi.NewRouter()
	router.Route("/api/v1", func(r chi.Router) {
		r.Get("/users/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
		r.Get("/broken", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	})
	router.NotFound(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	collector := metrics.New()
	handler := WithMetrics(collector, router)

	tests := []struct {
		name   string
		path   string
		status int
	}{
		{name: "templated route", path: "/api/v1/users/7d4c1a2e", status: http.StatusNoContent},
		{name: "server error", path: "/api/v1/broken", status: http.StatusInternalServerError},
		{name: "static fallback", path: "/today", status: http.StatusOK},
	}
	for _, tt := range tests {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.path, nil))
		if recorder.Code != tt.status {
			t.Fatalf("%s: status %d, want %d", tt.name, recorder.Code, tt.status)
		}
	}

	routes := map[string]int{}
	errors := 0
	for _, route := range collector.HTTP().Routes {
		routes[route.Route] = route.Count
		errors += route.Errors
	}
	// Учитываем запросы текущей минуты через окно 5 минут: HTTP() включает и её.
	for _, want := range []string{"GET /api/v1/users/{id}", "GET /api/v1/broken", "GET (статические файлы)"} {
		if routes[want] != 1 {
			t.Fatalf("route %q counted %d times, routes: %v", want, routes[want], routes)
		}
	}
	if errors != 1 {
		t.Fatalf("5xx counted %d times, want 1", errors)
	}
	if _, leaked := routes["GET /api/v1/users/7d4c1a2e"]; leaked {
		t.Fatal("concrete identifiers must not become separate routes")
	}
}
