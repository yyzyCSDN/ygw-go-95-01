package capacity

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"ygw-go-95-01/internal/berth"
	"ygw-go-95-01/internal/event"
)

type Report struct {
	CheckedAt   time.Time
	RemainKVA   float64
	Mismatches  int
	Released    int
	LastMessage string
}

type Reconciler struct {
	alloc    *Allocator
	store    *berth.Store
	bus      *event.Bus
	interval time.Duration
	mu       sync.Mutex
	last     Report
	runs     atomic.Int64
	stop     chan struct{}
	done     chan struct{}
}

func NewReconciler(alloc *Allocator, store *berth.Store, bus *event.Bus, interval time.Duration) *Reconciler {
	return &Reconciler{
		alloc:    alloc,
		store:    store,
		bus:      bus,
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (r *Reconciler) Run(ctx context.Context) {
	defer close(r.done)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stop:
			return
		case <-ticker.C:
			_, _ = r.Reconcile()
		}
	}
}

func (r *Reconciler) Stop() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
	<-r.done
}

func (r *Reconciler) Reconcile() (Report, error) {
	r.runs.Add(1)
	report := Report{CheckedAt: time.Now().UTC(), RemainKVA: r.alloc.Remain()}
	released, err := r.alloc.SyncFromStore()
	if err != nil {
		report.LastMessage = err.Error()
		return report, err
	}
	report.Released = released
	for _, b := range r.store.List() {
		_, allocated := r.alloc.AllocationOf(b.ID)
		if b.State == berth.StateSettled && !allocated {
			report.Mismatches++
		}
	}
	if report.Released > 0 {
		report.Mismatches += report.Released
	}
	if report.Mismatches > 0 {
		report.LastMessage = "capacity state drift detected"
		if r.bus != nil {
			r.bus.Publish(event.Event{
				Topic: "capacity.reconciled",
				ID:    time.Now().UTC().Format("150405"),
				Payload: map[string]string{
					"mismatches": itoa(report.Mismatches),
					"released":   itoa(report.Released),
				},
			})
		}
	}
	r.mu.Lock()
	r.last = report
	r.mu.Unlock()
	return report, nil
}

func (r *Reconciler) Last() Report {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}

func (r *Reconciler) Runs() int64 {
	return r.runs.Load()
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
