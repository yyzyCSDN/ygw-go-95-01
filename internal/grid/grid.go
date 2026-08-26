package grid

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"ygw-go-95-01/internal/berth"
	"ygw-go-95-01/internal/event"
	"ygw-go-95-01/internal/record"
)

type GridState string

const (
	StateOff        GridState = "off"
	StateSyncing    GridState = "syncing"
	StateOnGrid     GridState = "on-grid"
	StateSeparating GridState = "separating"
)

var (
	ErrPhaseNotChecked = errors.New("phase check not completed")
	ErrSyncTimeout     = errors.New("phase sync timeout")
	ErrAlreadyOnGrid   = errors.New("grid already on-grid")
	ErrNotOnGrid       = errors.New("grid not on-grid")
)

type Controller struct {
	mu           sync.Mutex
	breaker      Breaker
	syncer       *Syncer
	berths       *berth.Store
	rec          *record.Recorder
	bus          *event.Bus
	state        GridState
	phaseChecked bool
	lastPhase    Phase
	mode         ControlMode
	queue        *CommandQueue
	execCount    int
}

func NewController(
	breaker Breaker,
	syncer *Syncer,
	berths *berth.Store,
	rec *record.Recorder,
	bus *event.Bus,
) *Controller {
	return &Controller{
		breaker: breaker,
		syncer:  syncer,
		berths:  berths,
		rec:     rec,
		bus:     bus,
		state:   StateOff,
		queue:   NewCommandQueue(),
	}
}

func (g *Controller) PhaseCheck(phase Phase) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.state == StateOnGrid {
		return ErrAlreadyOnGrid
	}
	g.phaseChecked = true
	g.lastPhase = phase
	return nil
}

func (g *Controller) SyncAndClose(ctx context.Context, phase Phase, timeout time.Duration) error {
	g.mu.Lock()
	if g.state == StateOnGrid {
		g.mu.Unlock()
		return ErrAlreadyOnGrid
	}
	if !g.phaseChecked {
		g.mu.Unlock()
		return ErrPhaseNotChecked
	}
	g.mu.Unlock()
	if err := g.syncer.WaitSync(ctx, phase, timeout); err != nil {
		g.recordEvent("sync.failed", phase, err.Error())
		return err
	}
	if err := g.breaker.Close(); err != nil {
		g.recordEvent("breaker.failed", phase, err.Error())
		return fmt.Errorf("breaker close: %w", err)
	}
	g.mu.Lock()
	g.state = StateOnGrid
	g.execCount++
	g.mu.Unlock()
	g.recordEvent("grid.on", phase, "")
	return nil
}

func (g *Controller) Separate() error {
	g.mu.Lock()
	if g.state != StateOnGrid {
		g.mu.Unlock()
		return ErrNotOnGrid
	}
	g.state = StateSeparating
	g.mu.Unlock()
	if err := g.breaker.Open(); err != nil {
		g.mu.Lock()
		g.state = StateOnGrid
		g.mu.Unlock()
		return fmt.Errorf("breaker open: %w", err)
	}
	g.mu.Lock()
	g.state = StateOff
	g.phaseChecked = false
	g.lastPhase = Phase{}
	g.mu.Unlock()
	g.recordEvent("grid.off", Phase{}, "")
	return nil
}

func (g *Controller) State() GridState {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state
}

func (g *Controller) PhaseChecked() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.phaseChecked
}

func (g *Controller) BreakerClosed() bool {
	return g.breaker.IsClosed()
}

func (g *Controller) ExecutionCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.execCount
}

func (g *Controller) recordEvent(kind string, phase Phase, message string) {
	if g.rec == nil {
		return
	}
	if err := g.rec.Lease(); err != nil {
		return
	}
	defer g.rec.Release()
	_, _ = g.rec.Append(record.Entry{
		Kind:    kind,
		Message: fmt.Sprintf("%s %.2f/%.2f/%.2f %s", kind, phase.VoltageKV, phase.FreqHz, phase.Degree, message),
		At:      time.Now().UTC(),
	})
	if g.bus != nil {
		g.bus.Publish(event.Event{
			Topic: "grid.state",
			ID:    kind,
			Payload: map[string]string{
				"state":   string(g.State()),
				"breaker": fmt.Sprintf("%v", g.BreakerClosed()),
			},
		})
	}
}
