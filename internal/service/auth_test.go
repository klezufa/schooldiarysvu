package service

import (
	"errors"
	"testing"
	"time"

	"schooldiarysvu/internal/apperr"
)

func TestLoginLimiter_ReportsRemainingBlock(t *testing.T) {
	t.Parallel()
	limiter := newLoginLimiter()
	for range 4 {
		limiter.failed("ip\x00login")
	}
	if err := limiter.check("ip\x00login"); err != nil {
		t.Fatalf("four failures must not block: %v", err)
	}
	limiter.failed("ip\x00login")
	var blocked *apperr.Error
	if err := limiter.check("ip\x00login"); !errors.As(err, &blocked) || blocked.Status != 429 {
		t.Fatalf("fifth failure must block: %v", err)
	}
	if blocked.RetryAfter <= 14*time.Minute || blocked.RetryAfter > 15*time.Minute {
		t.Fatalf("remaining block: %v", blocked.RetryAfter)
	}
	if err := limiter.check("ip\x00other"); err != nil {
		t.Fatalf("another login from the same address is not blocked: %v", err)
	}
}
