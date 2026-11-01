package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"schooldiarysvu/internal/httpserver"
)

func writeSite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html":           "<!doctype html><title>diary</title>",
		"assets/app-abc123.js": "console.log('app')",
		"favicon.svg":          "<svg/>",
	}
	for name, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSPAHandler(t *testing.T) {
	t.Parallel()
	handler, err := httpserver.SPAHandler(writeSite(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name         string
		method       string
		target       string
		wantStatus   int
		wantBody     string
		wantCache    string
		wantNoBodyOf string
	}{
		{name: "root serves index", method: http.MethodGet, target: "/", wantStatus: http.StatusOK, wantBody: "<title>diary</title>", wantCache: "no-cache"},
		{name: "client route falls back to index", method: http.MethodGet, target: "/journal", wantStatus: http.StatusOK, wantBody: "<title>diary</title>", wantCache: "no-cache"},
		{name: "nested client route falls back to index", method: http.MethodGet, target: "/admin/users/42", wantStatus: http.StatusOK, wantBody: "<title>diary</title>"},
		{name: "fingerprinted asset is cached forever", method: http.MethodGet, target: "/assets/app-abc123.js", wantStatus: http.StatusOK, wantBody: "console.log", wantCache: "public, max-age=31536000, immutable"},
		{name: "other static file is revalidated", method: http.MethodGet, target: "/favicon.svg", wantStatus: http.StatusOK, wantBody: "<svg/>", wantCache: "no-cache"},
		{name: "missing asset is not replaced by index", method: http.MethodGet, target: "/assets/app-old999.js", wantStatus: http.StatusNotFound, wantNoBodyOf: "<title>diary</title>"},
		{name: "unknown api path is 404 not html", method: http.MethodGet, target: "/api/v1/typo", wantStatus: http.StatusNotFound, wantNoBodyOf: "<title>diary</title>"},
		{name: "health path is not a frontend route", method: http.MethodGet, target: "/health/typo", wantStatus: http.StatusNotFound, wantNoBodyOf: "<title>diary</title>"},
		{name: "directory is not listed", method: http.MethodGet, target: "/assets", wantStatus: http.StatusOK, wantBody: "<title>diary</title>", wantNoBodyOf: "app-abc123.js"},
		{name: "dot-dot traversal is rejected", method: http.MethodGet, target: "/../../etc/passwd", wantStatus: http.StatusBadRequest, wantNoBodyOf: "root:"},
		{name: "encoded traversal is rejected", method: http.MethodGet, target: "/%2e%2e/%2e%2e/etc/passwd", wantStatus: http.StatusBadRequest, wantNoBodyOf: "root:"},
		{name: "post is rejected", method: http.MethodPost, target: "/journal", wantStatus: http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(tt.method, tt.target, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", response.Code, tt.wantStatus, response.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(response.Body.String(), tt.wantBody) {
				t.Errorf("body %q does not contain %q", response.Body.String(), tt.wantBody)
			}
			if tt.wantNoBodyOf != "" && strings.Contains(response.Body.String(), tt.wantNoBodyOf) {
				t.Errorf("body unexpectedly contains %q", tt.wantNoBodyOf)
			}
			if tt.wantCache != "" {
				if got := response.Header().Get("Cache-Control"); got != tt.wantCache {
					t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
				}
			}
		})
	}
}

func TestSPAHandler_SecurityHeaders(t *testing.T) {
	t.Parallel()
	handler, err := httpserver.SPAHandler(writeSite(t))
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	for header, want := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer"} {
		if got := response.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if csp := response.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") || !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("Content-Security-Policy = %q", csp)
	}
	if hsts := response.Header().Get("Strict-Transport-Security"); hsts != "" {
		t.Errorf("HSTS must not be set in the local-http mode, got %q", hsts)
	}
}

func TestSPAHandler_RequiresIndex(t *testing.T) {
	t.Parallel()
	if _, err := httpserver.SPAHandler(t.TempDir()); err == nil {
		t.Fatal("expected an error for a directory without index.html")
	}
	if _, err := httpserver.SPAHandler(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}
