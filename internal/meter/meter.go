package meter

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"ygw-go-95-01/internal/event"
	"ygw-go-95-01/internal/record"
)

type MeterState string

const (
	NotMetering MeterState = "not_metering"
	Metering    MeterState = "metering"
	Suspended   MeterState = "suspended"
	Settled     MeterState = "settled"
)

var (
	ErrAlreadyMetering = errors.New("meter already running for vessel")
	ErrNotMetering     = errors.New("meter not running for vessel")
	ErrNoSamples       = errors.New("no samples for vessel")
)

type Run struct {
	Vessel    string
	BerthID   string
	StartedAt time.Time
	State     MeterState
	Samples   int
}

type Meter struct {
	mu     sync.Mutex
	rec    *record.Recorder
	bus    *event.Bus
	store  *Store
	active map[string]*Run
}

func NewMeter(rec *record.Recorder, bus *event.Bus, store *Store) *Meter {
	return &Meter{
		rec:    rec,
		bus:    bus,
		store:  store,
		active: make(map[string]*Run),
	}
}

func (m *Meter) Start(vessel, berthID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if vessel == "" || berthID == "" {
		return fmt.Errorf("vessel and berth are required")
	}
	if _, ok := m.active[vessel]; ok {
		return ErrAlreadyMetering
	}
	m.active[vessel] = &Run{
		Vessel:    vessel,
		BerthID:   berthID,
		StartedAt: time.Now().UTC(),
		State:     Metering,
	}
	return nil
}

func (m *Meter) Sample(vessel string) (Sample, error) {
	m.mu.Lock()
	run, ok := m.active[vessel]
	if !ok {
		m.mu.Unlock()
		return Sample{}, ErrNotMetering
	}
	if run.State != Metering {
		m.mu.Unlock()
		return Sample{}, ErrNotMetering
	}
	run.Samples++
	smp := Sample{
		Vessel:  vessel,
		BerthID: run.BerthID,
		KWh:     float64(run.Samples) * 0.10,
		KVAh:    float64(run.Samples) * 0.08,
		At:      time.Now().UTC(),
		Seq:     run.Samples,
	}
	m.mu.Unlock()
	if err := m.store.Add(vessel, smp); err != nil {
		return Sample{}, err
	}
	if m.rec != nil {
		_, _ = m.rec.Append(record.Entry{
			Kind:    "meter.sample",
			Vessel:  vessel,
			BerthID: run.BerthID,
			Message: fmt.Sprintf("sample seq %d kwh %.2f", smp.Seq, smp.KWh),
			At:      smp.At,
		})
	}
	if m.bus != nil {
		m.bus.Publish(event.Event{
			Topic: "meter.sampled",
			ID:    vessel,
			Payload: map[string]string{
				"seq":   fmt.Sprintf("%d", smp.Seq),
				"kwh":   fmt.Sprintf("%.2f", smp.KWh),
				"berth": run.BerthID,
			},
		})
	}
	return smp, nil
}

func (m *Meter) Resume(vessel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.active[vessel]
	if !ok {
		return ErrNotMetering
	}
	if run.State != Suspended {
		return fmt.Errorf("meter is not suspended")
	}
	run.State = Metering
	return nil
}

func (m *Meter) Suspend(vessel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.active[vessel]
	if !ok {
		return ErrNotMetering
	}
	if run.State != Metering {
		return ErrNotMetering
	}
	run.State = Suspended
	return nil
}

func (m *Meter) Stop(vessel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.active[vessel]
	if !ok {
		return ErrNotMetering
	}
	run.State = Settled
	return nil
}

func (m *Meter) StateOf(vessel string) MeterState {
	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.active[vessel]
	if !ok {
		return NotMetering
	}
	return run.State
}

func (m *Meter) Runs() []Run {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Run, 0, len(m.active))
	for _, run := range m.active {
		out = append(out, *run)
	}
	return out
}
