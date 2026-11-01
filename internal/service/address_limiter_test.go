package service

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"schooldiarysvu/internal/apperr"
)

func TestAddressLimiter_BlocksPasswordSprayingFromOneAddress(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		limiter := newAddressLimiter()
		for range addressFailureLimit - 1 {
			limiter.failed("10.0.0.5")
		}
		if err := limiter.check("10.0.0.5"); err != nil {
			t.Fatalf("below the limit must not block: %v", err)
		}
		limiter.failed("10.0.0.5")
		var blocked *apperr.Error
		if err := limiter.check("10.0.0.5"); !errors.As(err, &blocked) || blocked.Status != 429 {
			t.Fatalf("limit reached must block the address: %v", err)
		}
		if err := limiter.check("10.0.0.6"); err != nil {
			t.Fatalf("another address is not blocked: %v", err)
		}
		time.Sleep(addressBlock + time.Second)
		if err := limiter.check("10.0.0.5"); err != nil {
			t.Fatalf("block must expire: %v", err)
		}
	})
}

func TestAddressLimiter_ForgetsOldFailures(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		limiter := newAddressLimiter()
		for range addressFailureLimit - 1 {
			limiter.failed("10.0.0.7")
		}
		time.Sleep(addressWindowLength + time.Second) // ошибки разных уроков не складываются
		limiter.failed("10.0.0.7")
		if err := limiter.check("10.0.0.7"); err != nil {
			t.Fatalf("failures of an old window must not count: %v", err)
		}
	})
}
