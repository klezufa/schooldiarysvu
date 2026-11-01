package httpserver_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"schooldiarysvu/internal/apperr"
	"schooldiarysvu/internal/httpserver"
)

func TestWriteError_ClassifiesUnhandledErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "client canceled request is not a server failure", err: fmt.Errorf("query: %w", context.Canceled), wantStatus: httpserver.StatusClientClosedRequest, wantCode: "request_canceled"},
		{name: "deadline exceeded is a gateway timeout", err: fmt.Errorf("query: %w", context.DeadlineExceeded), wantStatus: http.StatusGatewayTimeout, wantCode: "timeout"},
		{name: "unknown error is an opaque internal error", err: errors.New("secret detail: password=hunter2"), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
		{name: "typed error keeps its status", err: &httpserver.Error{Status: http.StatusConflict, Code: "conflict", Message: "x"}, wantStatus: http.StatusConflict, wantCode: "conflict"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			httpserver.WriteError(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/x", nil), tt.err)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			body := recorder.Body.String()
			if !strings.Contains(body, `"code":"`+tt.wantCode+`"`) {
				t.Errorf("body %q lacks code %q", body, tt.wantCode)
			}
			if strings.Contains(body, "hunter2") || strings.Contains(body, "secret detail") {
				t.Errorf("internal error detail leaked to the client: %q", body)
			}
		})
	}
}

func TestWriteError_RetryAfter(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	httpserver.WriteError(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil), apperr.TooManyAttempts(14*time.Minute+30*time.Second))
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") != "870" {
		t.Fatalf("status %d, Retry-After %q", recorder.Code, recorder.Header().Get("Retry-After"))
	}
	if !strings.Contains(recorder.Body.String(), "через 15 мин") {
		t.Fatalf("message must say how long to wait: %s", recorder.Body.String())
	}
	plain := httptest.NewRecorder()
	httpserver.WriteError(plain, httptest.NewRequest(http.MethodGet, "/", nil), apperr.Conflict("x"))
	if plain.Header().Get("Retry-After") != "" {
		t.Fatal("Retry-After must be sent only for rate limiting")
	}
}
