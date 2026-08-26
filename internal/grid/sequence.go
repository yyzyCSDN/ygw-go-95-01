package grid

import (
	"fmt"

	"ygw-go-95-01/internal/berth"
)

type StepKind int

const (
	StepSetGridState StepKind = iota
	StepSetBerthState
	StepCloseBreaker
	StepOpenBreaker
)

type SequenceStep struct {
	Kind       StepKind
	GridState  GridState
	BerthID    string
	BerthState berth.State
	Vessel     string
}

type Sequence struct {
	ID    string
	Steps []SequenceStep
}

func (g *Controller) ApplySequence(seq Sequence) error {
	g.mu.Lock()
	if g.state == StateOnGrid {
		g.mu.Unlock()
		return ErrAlreadyOnGrid
	}
	if len(seq.Steps) == 0 {
		g.mu.Unlock()
		return fmt.Errorf("empty sequence")
	}
	previousGrid := g.state
	g.mu.Unlock()
	appliedBerths := make(map[string]berth.Berth)
	for index, step := range seq.Steps {
		switch step.Kind {
		case StepSetGridState:
			g.mu.Lock()
			g.state = step.GridState
			g.mu.Unlock()
		case StepCloseBreaker:
			if err := g.breaker.Close(); err != nil {
				g.rollback(previousGrid, appliedBerths)
				return fmt.Errorf("sequence %s step %d: %w", seq.ID, index, err)
			}
		case StepOpenBreaker:
			if err := g.breaker.Open(); err != nil {
				g.rollback(previousGrid, appliedBerths)
				return fmt.Errorf("sequence %s step %d: %w", seq.ID, index, err)
			}
		case StepSetBerthState:
			if g.berths == nil {
				g.rollback(previousGrid, appliedBerths)
				return fmt.Errorf("sequence %s step %d: no berth store", seq.ID, index)
			}
			before, ok := g.berths.Get(step.BerthID)
			if !ok {
				g.rollback(previousGrid, appliedBerths)
				return fmt.Errorf("sequence %s step %d: %w", seq.ID, index, berth.ErrNotFound)
			}
			appliedBerths[step.BerthID] = *before
			if step.BerthState == berth.StateSettled {
				if err := g.berths.MarkOccupied(step.BerthID, step.Vessel); err != nil {
					g.rollback(previousGrid, appliedBerths)
					return fmt.Errorf("sequence %s step %d: %w", seq.ID, index, err)
				}
			} else if step.BerthState == berth.StateIdle {
				if err := g.berths.MarkIdle(step.BerthID); err != nil {
					g.rollback(previousGrid, appliedBerths)
					return fmt.Errorf("sequence %s step %d: %w", seq.ID, index, err)
				}
			} else {
				g.rollback(previousGrid, appliedBerths)
				return fmt.Errorf("sequence %s step %d: unsupported berth state %s", seq.ID, index, step.BerthState)
			}
		default:
			g.rollback(previousGrid, appliedBerths)
			return fmt.Errorf("sequence %s step %d: unsupported step kind", seq.ID, index)
		}
	}
	return nil
}

func (g *Controller) rollback(gridState GridState, applied map[string]berth.Berth) {
	for _, snapshot := range applied {
		if g.berths != nil {
			_ = g.berths.RestoreOne(snapshot)
		}
	}
	g.mu.Lock()
	g.state = gridState
	g.mu.Unlock()
}
