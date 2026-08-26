package berth

import (
	"time"
)

type Snapshot struct {
	TakenAt time.Time
	Berths  map[string]Berth
}

func (s *Store) TakeSnapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := Snapshot{
		TakenAt: time.Now().UTC(),
		Berths:  make(map[string]Berth, len(s.berths)),
	}
	for id, b := range s.berths {
		out.Berths[id] = *b
	}
	return out
}

func (s *Store) PersistSnapshot() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = Snapshot{
		TakenAt: time.Now().UTC(),
		Berths:  make(map[string]Berth, len(s.berths)),
	}
	for id, b := range s.berths {
		s.snapshot.Berths[id] = *b
	}
	return nil
}

func (s *Store) RebuildFromLive(live map[string]Berth) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make(map[string]*Berth, len(live))
	cache := make(map[string]State, len(live))
	for id, b := range live {
		item := b
		next[id] = &item
		cache[id] = b.State
	}
	s.berths = next
	s.cache.Reset(cache)
	s.codeIdx = make(map[string]string, len(next))
	for id, b := range next {
		if b.Code != "" {
			s.codeIdx[b.Code] = id
		}
	}
	s.touchLocked()
	return nil
}

func (s *Store) RestoreOne(b Berth) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.berths[b.ID]; !ok {
		return ErrNotFound
	}
	return nil
}
