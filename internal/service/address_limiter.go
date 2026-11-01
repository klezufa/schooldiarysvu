package service

import (
	"sync"
	"time"

	"schooldiarysvu/internal/apperr"
)

// addressLimiter считает неудачные входы с одного адреса по всем логинам. Счётчик пары «адрес — логин» не видит
// password spraying — один распространённый пароль по сотням логинов (пентест: 80 попыток подряд без блокировки).
// Порог выше, чем у одного логина: за общим NAT компьютерного класса ошибаются многие, это не должно блокировать
// всех. Успешный вход счётчик не сбрасывает — иначе атакующий перемежал бы перебор своим настоящим входом.
type addressLimiter struct {
	mu      sync.Mutex
	windows map[string]addressWindow
}

type addressWindow struct {
	started      time.Time
	failures     int
	blockedUntil time.Time
}

const (
	addressFailureLimit = 50
	addressWindowLength = 15 * time.Minute
	addressBlock        = 15 * time.Minute
	addressPruneSize    = 10_000
)

func newAddressLimiter() *addressLimiter {
	return &addressLimiter{windows: make(map[string]addressWindow)}
}

func (l *addressLimiter) check(address string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if wait := time.Until(l.windows[address].blockedUntil); wait > 0 {
		return apperr.TooManyAttempts(wait)
	}
	return nil
}

func (l *addressLimiter) failed(address string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.windows) >= addressPruneSize {
		l.prune(now)
	}
	window := l.windows[address]
	if now.Sub(window.started) > addressWindowLength {
		window = addressWindow{started: now, blockedUntil: window.blockedUntil}
	}
	window.failures++
	if window.failures >= addressFailureLimit {
		window = addressWindow{started: now, blockedUntil: now.Add(addressBlock)}
	}
	l.windows[address] = window
}

// prune удаляет окна, которые уже не блокируют и не копят неудачи, чтобы карта не росла от множества адресов.
func (l *addressLimiter) prune(now time.Time) {
	for address, window := range l.windows {
		if now.After(window.blockedUntil) && now.Sub(window.started) > addressWindowLength {
			delete(l.windows, address)
		}
	}
}
