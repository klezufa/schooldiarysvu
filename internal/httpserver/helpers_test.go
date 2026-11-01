package httpserver

import (
	"net/http/httptest"
	"testing"
)

func TestPagination_BoundsOffset(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		query              string
		wantLimit, wantOff int32
	}{
		{name: "defaults", query: "", wantLimit: 50, wantOff: 0},
		{name: "limit above 100 is ignored", query: "limit=100000", wantLimit: 50, wantOff: 0},
		{name: "offset beyond int32 is capped, not wrapped negative", query: "offset=3000000000", wantLimit: 50, wantOff: maxOffset},
		{name: "negative offset is ignored", query: "offset=-5", wantLimit: 50, wantOff: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			limit, offset := pagination(httptest.NewRequest("GET", "/api/v1/users?"+tt.query, nil))
			if limit != tt.wantLimit || offset != tt.wantOff {
				t.Fatalf("pagination(%q) = %d, %d; want %d, %d", tt.query, limit, offset, tt.wantLimit, tt.wantOff)
			}
		})
	}
}

func TestQueryVersion_RejectsOverflow(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"0", "-1", "2147483648", "x"} {
		if _, err := queryVersion(httptest.NewRequest("GET", "/api/v1/grades/1?version="+value, nil)); err == nil {
			t.Errorf("version=%s must be rejected", value)
		}
	}
	if version, err := queryVersion(httptest.NewRequest("GET", "/api/v1/grades/1?version=7", nil)); err != nil || version != 7 {
		t.Fatalf("version=7: %d, %v", version, err)
	}
}
