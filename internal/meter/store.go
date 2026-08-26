package meter

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/cespare/xxhash/v2"
)

type Store struct {
	mu           sync.Mutex
	samples      map[string][]Sample
	fingerprints map[string]bool
}

func NewStore() *Store {
	return &Store{
		samples:      make(map[string][]Sample),
		fingerprints: make(map[string]bool),
	}
}

func (s *Store) Add(vessel string, smp Sample) error {
	if vessel == "" || smp.Seq <= 0 {
		return fmt.Errorf("invalid sample")
	}
	fp := sampleFingerprint(smp)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fingerprints[fp] {
		return nil
	}
	s.fingerprints[fp] = true
	s.samples[vessel] = append(s.samples[vessel], smp)
	return nil
}

func (s *Store) Last(vessel string) (Sample, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.samples[vessel]
	if len(items) == 0 {
		return Sample{}, false
	}
	return items[len(items)-1], true
}

func sampleFingerprint(smp Sample) string {
	h := xxhash.New()
	_, _ = h.WriteString(smp.Vessel)
	_, _ = h.WriteString("\x00")
	_, _ = h.WriteString(smp.BerthID)
	_, _ = h.WriteString("\x00")
	_, _ = h.WriteString(strconv.Itoa(smp.Seq))
	return fmt.Sprintf("%x", h.Sum64())
}
