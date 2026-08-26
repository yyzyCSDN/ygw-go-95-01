package grid

import (
	"context"
	"sync"
	"time"
)

type Phase struct {
	VoltageKV float64
	FreqHz    float64
	Degree    float64
}

func (p Phase) Valid() bool {
	return p.VoltageKV > 0 && p.FreqHz > 0
}

type Syncer struct {
	mu    sync.Mutex
	match func(Phase) bool
}

func NewSyncer(match func(Phase) bool) *Syncer {
	return &Syncer{match: match}
}

func (s *Syncer) WaitSync(ctx context.Context, want Phase, timeout time.Duration) error {
	if s.matches(want) {
		return nil
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return ErrSyncTimeout
		case <-ticker.C:
			if s.matches(want) {
				return nil
			}
		}
	}
}

func (s *Syncer) matches(want Phase) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.match == nil {
		return false
	}
	return s.match(want)
}
