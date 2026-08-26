package berth

import (
	"fmt"
	"time"
)

func (s *Store) MarkOccupied(id, vessel string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.berths[id]
	if !ok {
		return ErrNotFound
	}
	if b.State != StateIdle && b.State != StateAllocating {
		return fmt.Errorf("%w: current %s", ErrBusy, b.State)
	}
	if vessel == "" {
		return fmt.Errorf("vessel is required")
	}
	b.State = StateSettled
	b.Vessel = vessel
	b.OccupiedAt = time.Now().UTC()
	b.Fingerprint = fmt.Sprintf("%s@%d", id, b.OccupiedAt.UnixNano())
	s.cache.Put(id, StateSettled)
	s.touchLocked()
	return nil
}

func (s *Store) MarkIdle(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.berths[id]
	if !ok {
		return ErrNotFound
	}
	if b.State == StateIdle {
		return nil
	}
	b.State = StateIdle
	b.Vessel = ""
	b.OccupiedAt = time.Time{}
	s.touchLocked()
	return nil
}

func (s *Store) MarkAllocating(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.berths[id]
	if !ok {
		return ErrNotFound
	}
	if b.State != StateIdle {
		return fmt.Errorf("%w: current %s", ErrBusy, b.State)
	}
	b.State = StateAllocating
	s.cache.Put(id, StateAllocating)
	s.touchLocked()
	return nil
}

func (s *Store) MarkReleasing(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.berths[id]
	if !ok {
		return ErrNotFound
	}
	if b.State != StateSettled {
		return fmt.Errorf("%w: berth %s is %s", ErrBadState, id, b.State)
	}
	b.State = StateReleasing
	s.cache.Put(id, StateReleasing)
	s.touchLocked()
	return nil
}

func (s *Store) CheckReady(id string, needKVA float64) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.berths[id]
	if !ok {
		return ErrNotFound
	}
	if b.State != StateIdle {
		return fmt.Errorf("%w: berth %s is %s", ErrBusy, id, b.State)
	}
	if needKVA <= 0 {
		return fmt.Errorf("demand must be positive")
	}
	if needKVA > b.CapacityKVA {
		return fmt.Errorf("demand %.0f exceeds berth capacity %.0f", needKVA, b.CapacityKVA)
	}
	return nil
}

func (s *Store) CanAccept(id string, needKVA float64) bool {
	return s.CheckReady(id, needKVA) == nil
}

func (s *Store) ResetState(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.berths[id]
	if !ok {
		return ErrNotFound
	}
	if b.State == StateIdle {
		return nil
	}
	b.State = StateIdle
	b.Vessel = ""
	b.OccupiedAt = time.Time{}
	s.cache.Put(id, StateIdle)
	s.touchLocked()
	return nil
}

func (s *Store) OccupiedBy(id string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.berths[id]
	if !ok {
		return "", false
	}
	return b.Vessel, b.Vessel != ""
}
