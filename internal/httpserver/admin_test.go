package httpserver

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		trustProxy bool
		remote     string
		realIP     string
		want       string
	}{
		{name: "direct client", remote: "192.168.1.20:5000", want: "192.168.1.20"},
		{name: "spoofed header from LAN is ignored by default", remote: "192.168.1.20:5000", realIP: "10.9.9.9", want: "192.168.1.20"},
		{name: "spoofed header from loopback is ignored by default", remote: "127.0.0.1:5000", realIP: "10.9.9.9", want: "127.0.0.1"},
		{name: "trusted proxy forwards client address", trustProxy: true, remote: "172.18.0.5:5000", realIP: "192.168.1.20", want: "192.168.1.20"},
		{name: "trusted proxy with garbage header", trustProxy: true, remote: "172.18.0.5:5000", realIP: "not-an-ip", want: "172.18.0.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest("GET", "/api/v1/invitations/lookup", nil)
			request.RemoteAddr = tt.remote
			if tt.realIP != "" {
				request.Header.Set("X-Real-IP", tt.realIP)
			}
			if got := (&API{trustProxy: tt.trustProxy}).clientIP(request); got != tt.want {
				t.Fatalf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
