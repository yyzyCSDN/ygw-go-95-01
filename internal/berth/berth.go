package berth

import (
	"errors"
	"sync"
	"time"
)

type State string

const (
	StateIdle       State = "idle"
	StateAllocating State = "allocating"
	StateSettled    State = "settled"
	StateReleasing  State = "releasing"
)

var (
	ErrNotFound = errors.New("berth not found")
	ErrBusy     = errors.New("berth is not idle")
	ErrBadState = errors.New("invalid berth state transition")
)

type Berth struct {
	ID          string
	Code        string
	VoltageKV   float64
	CapacityKVA float64
	State       State
	Vessel      string
	OccupiedAt  time.Time
	Fingerprint string
}

type Store struct {
	mu       sync.RWMutex
	berths   map[string]*Berth
	codeIdx  map[string]string
	cache    *Cache
	snapshot Snapshot
	version  int
}

func NewStore(seed []Berth) *Store {
	s := &Store{
		berths:  make(map[string]*Berth),
		codeIdx: make(map[string]string),
		cache:   NewCache(),
	}
	for _, b := range seed {
		item := b
		s.berths[b.ID] = &item
		if b.Code != "" {
			s.codeIdx[b.Code] = b.ID
		}
		s.cache.Put(b.ID, b.State)
	}
	return s
}

func (s *Store) Get(id string) (*Berth, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.berths[id]
	if !ok {
		return nil, false
	}
	cp := *b
	return &cp, true
}

func (s *Store) ByCode(code string) (*Berth, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.codeIdx[code]
	if !ok {
		return nil, false
	}
	b, ok := s.berths[id]
	if !ok {
		return nil, false
	}
	cp := *b
	return &cp, true
}

func (s *Store) List() []Berth {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Berth, 0, len(s.berths))
	for _, b := range s.berths {
		out = append(out, *b)
	}
	return out
}

func (s *Store) State(id string) (State, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.berths[id]
	if !ok {
		return "", false
	}
	return b.State, true
}

func (s *Store) CachedState(id string) (State, bool) {
	return s.cache.Get(id)
}

func (s *Store) touchLocked() {
	s.version++
}
