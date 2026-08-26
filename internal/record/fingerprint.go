package record

import (
	"strconv"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
)

type Entry struct {
	ID      string
	Kind    string
	Vessel  string
	BerthID string
	Message string
	At      time.Time
}

func Fingerprint(e Entry) string {
	h := xxhash.New()
	_, _ = h.WriteString(e.Kind)
	_, _ = h.WriteString("\x00")
	_, _ = h.WriteString(e.Vessel)
	_, _ = h.WriteString("\x00")
	_, _ = h.WriteString(e.BerthID)
	_, _ = h.WriteString("\x00")
	_, _ = h.WriteString(e.Message)
	_, _ = h.WriteString("\x00")
	_, _ = h.WriteString(e.At.UTC().Format(time.RFC3339Nano))
	return strconv.FormatUint(h.Sum64(), 16)
}

type dedupSet struct {
	mu    sync.RWMutex
	items map[string]bool
}

func newDedupSet() *dedupSet {
	return &dedupSet{items: make(map[string]bool)}
}

func (d *dedupSet) Seen(fp string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.items[fp]
}

func (d *dedupSet) Mark(fp string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.items[fp] = true
}
