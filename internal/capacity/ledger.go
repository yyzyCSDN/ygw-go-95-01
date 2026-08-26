package capacity

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
)

type LedgerRecord struct {
	Allocation  Allocation
	Fingerprint string
	ClosedAt    time.Time
}

type Ledger struct {
	mu    sync.Mutex
	items []LedgerRecord
}

func NewLedger() *Ledger {
	return &Ledger{}
}

func (l *Ledger) Append(a Allocation) string {
	fp := ledgerFingerprint(a)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = append(l.items, LedgerRecord{Allocation: a, Fingerprint: fp})
	return fp
}

func (l *Ledger) Close(a Allocation) string {
	fp := ledgerFingerprint(a)
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.items {
		if l.items[i].Fingerprint == fp && l.items[i].ClosedAt.IsZero() {
			l.items[i].ClosedAt = time.Now().UTC()
			break
		}
	}
	return fp
}

func ledgerFingerprint(a Allocation) string {
	h := xxhash.New()
	_, _ = h.WriteString(a.BerthID)
	_, _ = h.WriteString("\x00")
	_, _ = h.WriteString(a.Vessel)
	_, _ = h.WriteString("\x00")
	_, _ = h.WriteString(strconv.FormatFloat(a.KVA, 'f', 2, 64))
	_, _ = h.WriteString("\x00")
	_, _ = h.WriteString(a.At.UTC().Format(time.RFC3339Nano))
	return fmt.Sprintf("%x", h.Sum64())
}
