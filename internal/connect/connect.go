package connect

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"ygw-go-95-01/internal/berth"
	"ygw-go-95-01/internal/capacity"
	"ygw-go-95-01/internal/event"
	"ygw-go-95-01/internal/grid"
	"ygw-go-95-01/internal/meter"
)

type SessionState string

const (
	StateIdle       SessionState = "idle"
	StateVerifying  SessionState = "verifying"
	StateConnecting SessionState = "connecting"
	StateConnected  SessionState = "connected"
	StateFailed     SessionState = "failed"
	StateCancelled  SessionState = "cancelled"
)

var (
	ErrUnknownSession = errors.New("unknown session")
	ErrNotVerifying   = errors.New("session is not verifying")
	ErrVerifyTimeout  = errors.New("verification wait timed out")
	ErrAlreadyClosed  = errors.New("session already closed")
)

type ApplyRequest struct {
	RequestID string
	Vessel    string
	BerthID   string
	NeedKVA   float64
	Phase     grid.Phase
}

type Session struct {
	ID         string
	RequestID  string
	Vessel     string
	BerthID    string
	Phase      grid.Phase
	State      SessionState
	Progress   []Step
	Reason     string
	At         time.Time
	Allocation capacity.Allocation
}

type Connector struct {
	mu            sync.Mutex
	grid          *grid.Controller
	alloc         *capacity.Allocator
	berths        *berth.Store
	meter         *meter.Meter
	bus           *event.Bus
	sessions      map[string]*Session
	pending       map[string]chan struct{}
	executed      map[string]int
	override      Override
	verifyTimeout time.Duration
	syncTimeout   time.Duration
	seq           int
}

func NewConnector(
	g *grid.Controller,
	a *capacity.Allocator,
	b *berth.Store,
	m *meter.Meter,
	bus *event.Bus,
	verifyTimeout time.Duration,
	syncTimeout time.Duration,
) *Connector {
	return &Connector{
		grid:          g,
		alloc:         a,
		berths:        b,
		meter:         m,
		bus:           bus,
		sessions:      make(map[string]*Session),
		pending:       make(map[string]chan struct{}),
		executed:      make(map[string]int),
		verifyTimeout: verifyTimeout,
		syncTimeout:   syncTimeout,
	}
}

func (c *Connector) Apply(req ApplyRequest) (Session, error) {
	c.mu.Lock()
	if req.RequestID != "" {
		for _, sess := range c.sessions {
			if sess.RequestID == req.RequestID {
				cp := *sess
				c.mu.Unlock()
				return cp, nil
			}
		}
	}
	c.mu.Unlock()
	if req.Vessel == "" || req.BerthID == "" {
		return Session{}, fmt.Errorf("vessel and berth are required")
	}
	if !req.Phase.Valid() {
		return Session{}, fmt.Errorf("invalid shore phase parameters")
	}
	if !c.berths.CanAccept(req.BerthID, req.NeedKVA) {
		if err := c.berths.CheckReady(req.BerthID, req.NeedKVA); err != nil {
			return Session{}, err
		}
		return Session{}, fmt.Errorf("berth cannot accept demand %.0f kVA", req.NeedKVA)
	}
	alloc, err := c.alloc.Allocate(req.BerthID, req.Vessel, req.NeedKVA)
	if err != nil {
		return Session{}, err
	}
	if err := c.berths.MarkAllocating(req.BerthID); err != nil {
		_ = c.alloc.Release(req.BerthID)
		return Session{}, err
	}
	c.mu.Lock()
	c.seq++
	id := fmt.Sprintf("s-%03d", c.seq)
	sess := &Session{
		ID:         id,
		RequestID:  req.RequestID,
		Vessel:     req.Vessel,
		BerthID:    req.BerthID,
		Phase:      req.Phase,
		State:      StateVerifying,
		Progress:   []Step{StepVerify},
		At:         time.Now().UTC(),
		Allocation: alloc,
	}
	c.sessions[id] = sess
	c.pending[id] = make(chan struct{})
	c.mu.Unlock()
	c.publish("connect.applied", sess)
	return *sess, nil
}

func (c *Connector) ConfirmVerification(id string) error {
	c.mu.Lock()
	sess, ok := c.sessions[id]
	if !ok {
		c.mu.Unlock()
		return ErrUnknownSession
	}
	if sess.State == StateConnected {
		c.mu.Unlock()
		return nil
	}
	if sess.State != StateVerifying {
		c.mu.Unlock()
		return fmt.Errorf("%w: current %s", ErrNotVerifying, sess.State)
	}
	sess.State = StateConnecting
	sess.Progress = append(sess.Progress, StepPhaseCheck, StepLink)
	phase := sess.Phase
	requestID := sess.RequestID
	vessel := sess.Vessel
	berthID := sess.BerthID
	c.mu.Unlock()
	if err := c.grid.SyncAndClose(context.Background(), phase, c.syncTimeout); err != nil {
		c.fail(id, err)
		return err
	}
	if err := c.berths.MarkOccupied(berthID, vessel); err != nil {
		c.fail(id, err)
		return err
	}
	if err := c.meter.Start(vessel, berthID); err != nil {
		c.fail(id, err)
		return err
	}
	c.mu.Lock()
	sess.State = StateConnected
	sess.Progress = append(sess.Progress, StepClose)
	c.executed[requestID]++
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch != nil {
		close(ch)
	}
	c.publish("connect.connected", sess)
	return nil
}

func (c *Connector) Cancel(id string) error {
	c.mu.Lock()
	sess, ok := c.sessions[id]
	if !ok {
		c.mu.Unlock()
		return ErrUnknownSession
	}
	if sess.State == StateConnected {
		c.mu.Unlock()
		return ErrAlreadyClosed
	}
	sess.State = StateCancelled
	sess.Reason = "cancelled by operator"
	berthID := sess.BerthID
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch != nil {
		close(ch)
	}
	_ = c.berths.ResetState(berthID)
	_ = c.alloc.Release(berthID)
	c.publish("connect.cancelled", sess)
	return nil
}

func (c *Connector) Session(id string) (Session, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sess, ok := c.sessions[id]
	if !ok {
		return Session{}, false
	}
	return *sess, true
}

func (c *Connector) Sessions() []Session {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Session, 0, len(c.sessions))
	for _, sess := range c.sessions {
		out = append(out, *sess)
	}
	return out
}

func (c *Connector) ExecutionCount(requestID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.executed[requestID]
}

func (c *Connector) VerifyTimeout() time.Duration {
	return c.verifyTimeout
}

func (c *Connector) fail(id string, err error) {
	c.mu.Lock()
	sess, ok := c.sessions[id]
	if ok {
		sess.State = StateFailed
		sess.Reason = err.Error()
	}
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch != nil {
		close(ch)
	}
	c.publish("connect.failed", sess)
}

func (c *Connector) publish(topic string, sess *Session) {
	if c.bus == nil || sess == nil {
		return
	}
	c.bus.Publish(event.Event{
		Topic: topic,
		ID:    sess.ID,
		Payload: map[string]string{
			"vessel":  sess.Vessel,
			"berth":   sess.BerthID,
			"state":   string(sess.State),
			"request": sess.RequestID,
		},
	})
}
