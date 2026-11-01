// Package metrics собирает показатели нагрузки процесса для дашборда системного администратора.
// Данные хранятся в памяти (кольцо поминутных корзин) и периодически сбрасываются в БД репортёром.
package metrics

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Границы гистограммы задержек, мс. Последняя корзина — всё, что дольше последней границы.
var latencyBounds = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}

const (
	bucketCount = 60 // поминутные корзины за последний час
	topRoutes   = 8
)

type histogram [11]int

func (h *histogram) add(duration time.Duration) {
	ms := float64(duration) / float64(time.Millisecond)
	index := sort.SearchFloat64s(latencyBounds, ms)
	h[index]++
}

func (h histogram) merge(other histogram) histogram {
	for i := range h {
		h[i] += other[i]
	}
	return h
}

// quantile оценивает квантиль по гистограмме с линейной интерполяцией внутри корзины (мс).
func (h histogram) quantile(q float64) float64 {
	total := 0
	for _, count := range h {
		total += count
	}
	if total == 0 {
		return 0
	}
	target := q * float64(total)
	cumulative := 0.0
	for index, count := range h {
		next := cumulative + float64(count)
		if next >= target && count > 0 {
			lower := 0.0
			if index > 0 {
				lower = latencyBounds[index-1]
			}
			upper := latencyBounds[len(latencyBounds)-1] * 2
			if index < len(latencyBounds) {
				upper = latencyBounds[index]
			}
			return lower + (upper-lower)*(target-cumulative)/float64(count)
		}
		cumulative = next
	}
	return latencyBounds[len(latencyBounds)-1]
}

type routeBucket struct {
	count, errors int
	hist          histogram
}

type bucket struct {
	minute int64 // unix-минута; 0 — корзина не использовалась
	count  int
	errors int
	hist   histogram
	routes map[string]*routeBucket
}

// Collector безопасен для одновременного использования.
type Collector struct {
	mu       sync.Mutex
	started  time.Time
	buckets  [bucketCount]bucket
	inFlight atomic.Int64
	now      func() time.Time
}

func New() *Collector { return &Collector{started: time.Now(), now: time.Now} }

// Started возвращает момент запуска процесса.
func (c *Collector) Started() time.Time { return c.started }

// Begin и End считают запросы, выполняющиеся прямо сейчас.
func (c *Collector) Begin() { c.inFlight.Add(1) }
func (c *Collector) End()   { c.inFlight.Add(-1) }

// Observe учитывает завершённый запрос. Ошибкой считается только ответ 5xx: 4xx — штатная реакция на неверный ввод.
func (c *Collector) Observe(route string, status int, duration time.Duration) {
	minute := c.now().Unix() / 60
	c.mu.Lock()
	defer c.mu.Unlock()
	slot := &c.buckets[minute%bucketCount]
	if slot.minute != minute {
		*slot = bucket{minute: minute, routes: map[string]*routeBucket{}}
	}
	slot.count++
	slot.hist.add(duration)
	failed := status >= 500 && status != 0
	if failed {
		slot.errors++
	}
	stat := slot.routes[route]
	if stat == nil {
		stat = &routeBucket{}
		slot.routes[route] = stat
	}
	stat.count++
	stat.hist.add(duration)
	if failed {
		stat.errors++
	}
}

// RouteSnapshot — итог по маршруту за окно.
type RouteSnapshot struct {
	Route  string  `json:"route"`
	Count  int     `json:"count"`
	Errors int     `json:"errors"`
	P95Ms  float64 `json:"p95_ms"`
}

// HTTPSnapshot — состояние HTTP-нагрузки. Задержки и доля ошибок считаются за последние 5 минут.
type HTTPSnapshot struct {
	RequestsLastMinute int             `json:"requests_last_minute"`
	RequestsLastHour   int             `json:"requests_last_hour"`
	ErrorRate          float64         `json:"error_rate"`
	P50Ms              float64         `json:"p50_ms"`
	P95Ms              float64         `json:"p95_ms"`
	InFlight           int             `json:"in_flight"`
	Routes             []RouteSnapshot `json:"routes"`
}

// HTTP собирает сводку. Текущая (неполная) минута в «последней минуте» не участвует: берётся предыдущая, полная.
func (c *Collector) HTTP() HTTPSnapshot {
	current := c.now().Unix() / 60
	c.mu.Lock()
	defer c.mu.Unlock()
	var hist histogram
	total, errors := 0, 0
	snapshot := HTTPSnapshot{InFlight: int(c.inFlight.Load())}
	routes := map[string]*routeBucket{}
	for i := range c.buckets {
		slot := &c.buckets[i]
		if slot.minute == 0 || slot.minute > current || current-slot.minute >= bucketCount {
			continue
		}
		snapshot.RequestsLastHour += slot.count
		if slot.minute == current-1 {
			snapshot.RequestsLastMinute = slot.count
		}
		if current-slot.minute < 5 {
			hist = hist.merge(slot.hist)
			total += slot.count
			errors += slot.errors
			for route, stat := range slot.routes {
				merged := routes[route]
				if merged == nil {
					merged = &routeBucket{}
					routes[route] = merged
				}
				merged.count += stat.count
				merged.errors += stat.errors
				merged.hist = merged.hist.merge(stat.hist)
			}
		}
	}
	if total > 0 {
		snapshot.ErrorRate = float64(errors) / float64(total)
		snapshot.P50Ms = hist.quantile(0.5)
		snapshot.P95Ms = hist.quantile(0.95)
	}
	for route, stat := range routes {
		snapshot.Routes = append(snapshot.Routes, RouteSnapshot{Route: route, Count: stat.count, Errors: stat.errors, P95Ms: stat.hist.quantile(0.95)})
	}
	sort.Slice(snapshot.Routes, func(i, j int) bool {
		if snapshot.Routes[i].Count != snapshot.Routes[j].Count {
			return snapshot.Routes[i].Count > snapshot.Routes[j].Count
		}
		return snapshot.Routes[i].Route < snapshot.Routes[j].Route
	})
	if len(snapshot.Routes) > topRoutes {
		snapshot.Routes = snapshot.Routes[:topRoutes]
	}
	if snapshot.Routes == nil {
		snapshot.Routes = []RouteSnapshot{}
	}
	return snapshot
}
