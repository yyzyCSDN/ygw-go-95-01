package grid

import (
	"context"
	"sync"
)

type ControlMode int

const (
	ControlAuto ControlMode = iota
	ControlManual
)

type Command struct {
	ID      string
	Kind    string
	Vessel  string
	BerthID string
}

type CommandQueue struct {
	mu    sync.Mutex
	items []Command
	seen  map[string]bool
}

func NewCommandQueue() *CommandQueue {
	return &CommandQueue{seen: make(map[string]bool)}
}

func (q *CommandQueue) Push(cmd Command) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if cmd.ID == "" {
		return false
	}
	if q.seen[cmd.ID] {
		return false
	}
	q.seen[cmd.ID] = true
	q.items = append(q.items, cmd)
	return true
}

func (q *CommandQueue) Pop() (Command, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return Command{}, false
	}
	cmd := q.items[0]
	q.items = q.items[1:]
	return cmd, true
}

func (q *CommandQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

func (g *Controller) SetControlMode(mode ControlMode) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.mode = mode
	return nil
}

func (g *Controller) ControlMode() ControlMode {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.mode
}

func (g *Controller) Enqueue(cmd Command) bool {
	return g.queue.Push(cmd)
}

func (g *Controller) QueueLen() int {
	return g.queue.Len()
}

func (g *Controller) AutoDispatch(ctx context.Context) (int, error) {
	g.mu.Lock()
	if g.mode == ControlManual {
		g.mu.Unlock()
		return 0, nil
	}
	g.mu.Unlock()
	dispatched := 0
	for {
		_, ok := g.queue.Pop()
		if !ok {
			break
		}
		g.mu.Lock()
		checked := g.phaseChecked
		g.mu.Unlock()
		if !checked {
			continue
		}
		if err := g.SyncAndClose(ctx, g.lastPhase, 0); err != nil {
			continue
		}
		dispatched++
	}
	return dispatched, nil
}
