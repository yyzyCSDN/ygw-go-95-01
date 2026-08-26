package connect

import (
	"fmt"
	"time"
)

func (c *Connector) WaitVerification(id string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = c.verifyTimeout
	}
	c.mu.Lock()
	sess, ok := c.sessions[id]
	if !ok {
		c.mu.Unlock()
		return ErrUnknownSession
	}
	if sess.State != StateVerifying {
		c.mu.Unlock()
		return fmt.Errorf("%w: current %s", ErrNotVerifying, sess.State)
	}
	ch := c.pending[id]
	c.mu.Unlock()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ch:
		return nil
	case <-timer.C:
		c.timeout(id, timeout)
		return ErrVerifyTimeout
	}
}

func (c *Connector) timeout(id string, timeout time.Duration) {
	c.mu.Lock()
	sess, ok := c.sessions[id]
	if ok {
		sess.State = StateIdle
		sess.Reason = fmt.Sprintf("verification wait timed out after %s", timeout)
	}
	ch := c.pending[id]
	delete(c.pending, id)
	berthID := ""
	if sess != nil {
		berthID = sess.BerthID
	}
	c.mu.Unlock()
	if ch != nil {
		close(ch)
	}
	if berthID != "" {
		_ = c.berths.ResetState(berthID)
		_ = c.alloc.Release(berthID)
	}
	c.publish("connect.timeout", sess)
}
