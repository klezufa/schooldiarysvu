package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"schooldiarysvu/internal/apperr"
)

func TestWithPasswordSlot_BoundsConcurrentHashes(t *testing.T) {
	t.Parallel()
	s := &Service{passwordSlots: newPasswordSlots(2)}
	var running, peak atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			err := s.withPasswordSlot(t.Context(), func() {
				now := running.Add(1)
				for {
					old := peak.Load()
					if now <= old || peak.CompareAndSwap(old, now) {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
				running.Add(-1)
			})
			if err != nil {
				t.Errorf("slot: %v", err)
			}
		})
	}
	wg.Wait()
	if peak.Load() != 2 {
		t.Fatalf("peak concurrent hashes = %d, want 2", peak.Load())
	}
}

func TestWithPasswordSlot_BusyAfterQueueWait(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		s := &Service{passwordSlots: newPasswordSlots(1)}
		s.passwordSlots <- struct{}{} // слот занят долгим хешем
		err := s.withPasswordSlot(t.Context(), func() { t.Error("work must not run without a slot") })
		var busy *apperr.Error
		if !errors.As(err, &busy) || busy.Status != 503 || busy.RetryAfter != passwordRetryAfter {
			t.Fatalf("want 503 with Retry-After, got %v", err)
		}
	})
}

func TestWithPasswordSlot_StopsWhenRequestIsCanceled(t *testing.T) {
	t.Parallel()
	s := &Service{passwordSlots: newPasswordSlots(1)}
	s.passwordSlots <- struct{}{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.withPasswordSlot(ctx, func() {}); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}
