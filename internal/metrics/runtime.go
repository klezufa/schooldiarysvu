package metrics

import (
	"os"
	"runtime"
	rtmetrics "runtime/metrics"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

// RuntimeStats — состояние Go-процесса.
type RuntimeStats struct {
	Goroutines     int     `json:"goroutines"`
	HeapAllocBytes uint64  `json:"heap_alloc_bytes"`
	HeapSysBytes   uint64  `json:"heap_sys_bytes"`
	GCCycles       uint32  `json:"gc_cycles"`
	GCPauseTotalMs float64 `json:"gc_pause_total_ms"`
	CPUPercent     float64 `json:"cpu_percent"`
	UptimeSeconds  int64   `json:"uptime_seconds"`
}

// CPUMeter считает долю процессорного времени, которое Go-процесс тратит на пользовательский код,
// относительно доступного ему (GOMAXPROCS × время). Это загрузка именно приложения, а не всего хоста.
type CPUMeter struct {
	mu          sync.Mutex
	started     time.Time
	prevUser    float64
	prevTotal   float64
	lastPercent float64
}

func NewCPUMeter() *CPUMeter {
	m := &CPUMeter{started: time.Now()}
	m.prevUser, m.prevTotal = readCPUSeconds()
	return m
}

func readCPUSeconds() (user, total float64) {
	samples := []rtmetrics.Sample{{Name: "/cpu/classes/user:cpu-seconds"}, {Name: "/cpu/classes/total:cpu-seconds"}}
	rtmetrics.Read(samples)
	if samples[0].Value.Kind() == rtmetrics.KindFloat64 {
		user = samples[0].Value.Float64()
	}
	if samples[1].Value.Kind() == rtmetrics.KindFloat64 {
		total = samples[1].Value.Float64()
	}
	return user, total
}

// Percent возвращает загрузку с прошлого вызова (0–100).
func (m *CPUMeter) Percent() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	user, total := readCPUSeconds()
	if delta := total - m.prevTotal; delta > 0.05 {
		m.lastPercent = (user - m.prevUser) / delta * 100
		m.prevUser, m.prevTotal = user, total
	}
	if m.lastPercent < 0 {
		m.lastPercent = 0
	}
	if m.lastPercent > 100 {
		m.lastPercent = 100
	}
	return m.lastPercent
}

// Runtime снимает показатели процесса.
func Runtime(meter *CPUMeter, started time.Time) RuntimeStats {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	result := RuntimeStats{Goroutines: runtime.NumGoroutine(), HeapAllocBytes: stats.HeapAlloc, HeapSysBytes: stats.HeapSys,
		GCCycles: stats.NumGC, GCPauseTotalMs: float64(stats.PauseTotalNs) / 1e6, UptimeSeconds: int64(time.Since(started).Seconds())}
	if meter != nil {
		result.CPUPercent = meter.Percent()
	}
	return result
}

// HostStats — показатели машины (или контейнера); поля nil, если ОС их не отдала.
type HostStats struct {
	CPUPercent  *float64
	MemoryTotal *int64
	MemoryUsed  *int64
	DiskPath    string
	DiskTotal   *int64
	DiskUsed    *int64
}

var hostCache struct {
	sync.Mutex
	at    time.Time
	path  string
	stats HostStats
}

// hostCacheTTL: cpu.Percent(0) считает загрузку с прошлого вызова, поэтому частые вызовы из разных мест
// (репортёр, открытый дашборд) дают шумные значения. Кэш делает интервал замера не короче 5 секунд.
const hostCacheTTL = 5 * time.Second

// Host читает загрузку процессора и памяти хоста и занятость диска по указанному пути.
func Host(diskPath string) HostStats {
	hostCache.Lock()
	defer hostCache.Unlock()
	if hostCache.path == diskPath && time.Since(hostCache.at) < hostCacheTTL {
		return hostCache.stats
	}
	stats := readHost(diskPath)
	hostCache.at, hostCache.path, hostCache.stats = time.Now(), diskPath, stats
	return stats
}

func readHost(diskPath string) HostStats {
	var result HostStats
	if values, err := cpu.Percent(0, false); err == nil && len(values) == 1 {
		result.CPUPercent = &values[0]
	}
	if memory, err := mem.VirtualMemory(); err == nil {
		total, used := int64(memory.Total), int64(memory.Used)
		result.MemoryTotal, result.MemoryUsed = &total, &used
	}
	if diskPath == "" {
		diskPath, _ = os.Getwd()
	}
	if usage, err := disk.Usage(diskPath); err == nil {
		total, used := int64(usage.Total), int64(usage.Used)
		result.DiskPath, result.DiskTotal, result.DiskUsed = diskPath, &total, &used
	}
	return result
}

// PrimeHost делает первый (пустой) замер процессора хоста, чтобы следующие вызовы возвращали среднее за интервал.
func PrimeHost() { _, _ = cpu.Percent(0, false) }
