package metrics

import (
	"math"
	"testing"
	"time"
)

func newTestCollector(now *time.Time) *Collector {
	c := New()
	c.now = func() time.Time { return *now }
	return c
}

func TestHistogramQuantile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		durations []time.Duration
		q         float64
		min, max  float64
	}{
		{name: "empty histogram", q: 0.95, min: 0, max: 0},
		{name: "all fast requests", durations: []time.Duration{time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond}, q: 0.5, min: 0, max: 5},
		{name: "slow tail drives p95", durations: append(repeat(3*time.Millisecond, 90), repeat(700*time.Millisecond, 10)...), q: 0.95, min: 500, max: 1000},
		{name: "beyond last bound", durations: []time.Duration{20 * time.Second}, q: 0.5, min: 5000, max: 10000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var h histogram
			for _, d := range tt.durations {
				h.add(d)
			}
			got := h.quantile(tt.q)
			if got < tt.min || got > tt.max || math.IsNaN(got) {
				t.Fatalf("quantile(%v) = %v, want within [%v, %v]", tt.q, got, tt.min, tt.max)
			}
		})
	}
}

func repeat(d time.Duration, n int) []time.Duration {
	out := make([]time.Duration, n)
	for i := range out {
		out[i] = d
	}
	return out
}

func TestCollectorWindows(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 1, 10, 0, 30, 0, time.UTC)
	c := newTestCollector(&now)
	for i := 0; i < 10; i++ {
		c.Observe("GET /api/v1/me", 200, 10*time.Millisecond)
	}
	c.Observe("POST /api/v1/grades", 500, 40*time.Millisecond)

	now = now.Add(time.Minute) // предыдущая минута стала полной
	snapshot := c.HTTP()
	if snapshot.RequestsLastMinute != 11 || snapshot.RequestsLastHour != 11 {
		t.Fatalf("counts = %d/%d, want 11/11", snapshot.RequestsLastMinute, snapshot.RequestsLastHour)
	}
	if math.Abs(snapshot.ErrorRate-1.0/11) > 1e-9 {
		t.Fatalf("error rate = %v", snapshot.ErrorRate)
	}
	if len(snapshot.Routes) != 2 || snapshot.Routes[0].Route != "GET /api/v1/me" || snapshot.Routes[1].Errors != 1 {
		t.Fatalf("routes = %+v", snapshot.Routes)
	}

	now = now.Add(10 * time.Minute) // запросы ушли из 5-минутного окна, но остались в часовом
	snapshot = c.HTTP()
	if snapshot.RequestsLastMinute != 0 || snapshot.RequestsLastHour != 11 || snapshot.ErrorRate != 0 || len(snapshot.Routes) != 0 {
		t.Fatalf("after 10 minutes: %+v", snapshot)
	}

	now = now.Add(2 * time.Hour) // корзина кольца переиспользуется, старые данные не протекают
	c.Observe("GET /api/v1/me", 404, time.Millisecond)
	now = now.Add(time.Minute)
	snapshot = c.HTTP()
	if snapshot.RequestsLastHour != 1 || snapshot.ErrorRate != 0 {
		t.Fatalf("4xx must not count as errors and stale buckets must be dropped: %+v", snapshot)
	}
}

func TestCollectorInFlight(t *testing.T) {
	t.Parallel()
	c := New()
	c.Begin()
	c.Begin()
	c.End()
	if got := c.HTTP().InFlight; got != 1 {
		t.Fatalf("in flight = %d, want 1", got)
	}
}
