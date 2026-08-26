package alarm

import (
	"context"

	"ygw-go-95-01/internal/event"
)

type DispatchLoop struct {
	bus     *event.Bus
	mgr     *Manager
	actions *Actions
	queue   chan Alarm
	stop    chan struct{}
	done    chan struct{}
}

func NewDispatchLoop(bus *event.Bus, mgr *Manager, actions *Actions) *DispatchLoop {
	return &DispatchLoop{
		bus:     bus,
		mgr:     mgr,
		actions: actions,
		queue:   make(chan Alarm, 64),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (d *DispatchLoop) Run(ctx context.Context) {
	defer close(d.done)
	topics := []string{"grid.state", "connect.failed", "capacity.reconciled", "connect.connected"}
	for _, topic := range topics {
		if err := d.bus.Subscribe(topic, d.handleEvent); err != nil {
			return
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stop:
			return
		case al := <-d.queue:
			_ = d.actions.Dispatch(al)
		}
	}
}

func (d *DispatchLoop) Stop() {
	select {
	case <-d.stop:
	default:
		close(d.stop)
	}
	<-d.done
}

func (d *DispatchLoop) handleEvent(ev event.Event) {
	raised := d.mgr.Evaluate(ev)
	for _, al := range raised {
		select {
		case d.queue <- al:
		default:
		}
	}
}
