package record

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
)

type Store struct {
	mu      sync.RWMutex
	entries []Entry
	dedup   *dedupSet
}

func NewStore() *Store {
	return &Store{dedup: newDedupSet()}
}

func (s *Store) Resume(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		s.mu.Lock()
		s.entries = append(s.entries, e)
		s.dedup.Mark(Fingerprint(e))
		s.mu.Unlock()
	}
	return scanner.Err()
}

func (s *Store) Record(fp string, e Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
	s.dedup.Mark(fp)
}

func (s *Store) Seen(fp string) bool {
	return s.dedup.Seen(fp)
}
