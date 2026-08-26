package capacity

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"ygw-go-95-01/internal/berth"
)

var (
	ErrCapacityShort = errors.New("shore capacity insufficient")
	ErrNoAllocation  = errors.New("no allocation for berth")
	ErrNotIdle       = errors.New("berth is not idle")
)

type Allocation struct {
	ID      string
	BerthID string
	Vessel  string
	KVA     float64
	At      time.Time
}

type Allocator struct {
	mu     sync.Mutex
	store  *berth.Store
	shore  float64
	used   map[string]float64
	allocs map[string]Allocation
	ledger *Ledger
}

func NewAllocator(store *berth.Store, shore float64) *Allocator {
	return &Allocator{
		store:  store,
		shore:  shore,
		used:   make(map[string]float64),
		allocs: make(map[string]Allocation),
		ledger: NewLedger(),
	}
}

func (a *Allocator) Allocate(berthID, vessel string, needKVA float64) (Allocation, error) {
	if err := a.Refresh(berthID); err != nil {
		return Allocation{}, err
	}
	st, ok := a.store.CachedState(berthID)
	if !ok {
		return Allocation{}, berth.ErrNotFound
	}
	if st != berth.StateIdle {
		return Allocation{}, fmt.Errorf("%w: berth %s is %s", ErrNotIdle, berthID, st)
	}
	if needKVA <= 0 {
		return Allocation{}, fmt.Errorf("demand must be positive")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if existing, ok := a.allocs[berthID]; ok {
		return existing, nil
	}
	if needKVA > a.remainingLocked() {
		return Allocation{}, fmt.Errorf("%w: need %.0f remain %.0f", ErrCapacityShort, needKVA, a.remainingLocked())
	}
	alloc := Allocation{
		ID:      allocID(berthID, vessel, needKVA),
		BerthID: berthID,
		Vessel:  vessel,
		KVA:     needKVA,
		At:      time.Now().UTC(),
	}
	a.used[berthID] = needKVA
	a.allocs[berthID] = alloc
	a.ledger.Append(alloc)
	return alloc, nil
}

func (a *Allocator) Release(berthID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	alloc, ok := a.allocs[berthID]
	if !ok {
		return ErrNoAllocation
	}
	delete(a.used, berthID)
	delete(a.allocs, berthID)
	a.ledger.Close(alloc)
	return nil
}

func (a *Allocator) Refresh(berthID string) error {
	st, ok := a.store.State(berthID)
	if !ok {
		return berth.ErrNotFound
	}
	if st != berth.StateIdle {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if alloc, ok := a.allocs[berthID]; ok {
		delete(a.used, berthID)
		delete(a.allocs, berthID)
		a.ledger.Close(alloc)
	}
	return nil
}

func (a *Allocator) Remain() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.remainingLocked()
}

func (a *Allocator) UsedTotal() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	var total float64
	for _, v := range a.used {
		total += v
	}
	return total
}

func (a *Allocator) remainingLocked() float64 {
	var total float64
	for _, v := range a.used {
		total += v
	}
	return a.shore - total
}

func (a *Allocator) AllocationOf(berthID string) (Allocation, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	alloc, ok := a.allocs[berthID]
	return alloc, ok
}

func (a *Allocator) HasAllocation(berthID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.allocs[berthID]
	return ok
}

func (a *Allocator) SyncFromStore() (int, error) {
	released := 0
	for _, b := range a.store.List() {
		if b.State != berth.StateIdle {
			continue
		}
		if !a.HasAllocation(b.ID) {
			continue
		}
		if err := a.Release(b.ID); err == nil {
			released++
		}
	}
	return released, nil
}

func (a *Allocator) Allocations() []Allocation {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Allocation, 0, len(a.allocs))
	for _, alloc := range a.allocs {
		out = append(out, alloc)
	}
	return out
}

func (a *Allocator) Shore() float64 {
	return a.shore
}

func allocID(berthID, vessel string, kva float64) string {
	return fmt.Sprintf("all-%s-%s-%.0f", berthID, vessel, kva)
}
