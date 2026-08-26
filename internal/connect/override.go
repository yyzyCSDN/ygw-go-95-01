package connect

import (
	"context"
	"time"

	"ygw-go-95-01/internal/grid"
)

type Override struct {
	Active bool
	Vessel string
	Since  time.Time
}

func (o Override) ActiveFor(vessel string) bool {
	if !o.Active {
		return false
	}
	return o.Vessel == "" || o.Vessel == vessel
}

func (c *Connector) SetLocalOverride(o Override) error {
	c.mu.Lock()
	c.override = o
	c.mu.Unlock()
	mode := grid.ControlAuto
	if o.Active {
		mode = grid.ControlManual
	}
	return c.grid.SetControlMode(mode)
}

func (c *Connector) Override() Override {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.override
}

func (c *Connector) AutoDispatch() (int, error) {
	c.mu.Lock()
	override := c.override
	targets := make([]string, 0)
	for id, sess := range c.sessions {
		if sess.State == StateVerifying && !override.ActiveFor(sess.Vessel) {
			targets = append(targets, id)
		}
	}
	c.mu.Unlock()
	dispatched := 0
	for _, id := range targets {
		if err := c.ConfirmVerification(id); err == nil {
			dispatched++
		}
	}
	if dispatched > 0 {
		_, _ = c.grid.AutoDispatch(context.Background())
	}
	return dispatched, nil
}
