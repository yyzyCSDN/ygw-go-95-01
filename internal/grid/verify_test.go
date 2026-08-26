package grid

import (
	"context"
	"testing"
	"time"
)

func TestBerthPowerSwitchErrorNotSwallowed(t *testing.T) {
	breaker := NewSimpleBreaker()
	breaker.SetCloseFault(true)
	ctrl := NewController(breaker, NewSyncer(func(p Phase) bool { return true }), nil, nil, nil)
	phase := Phase{VoltageKV: 10, FreqHz: 50, Degree: 0}
	if err := ctrl.PhaseCheck(phase); err != nil {
		t.Fatal(err)
	}
	err := ctrl.SyncAndClose(context.Background(), phase, time.Second)
	if err == nil {
		t.Fatalf("breaker close error must not be swallowed")
	}
	if ctrl.State() == StateOnGrid {
		t.Fatalf("grid must not report on-grid after failed close")
	}
	if ctrl.BreakerClosed() {
		t.Fatalf("breaker must stay open after failed close")
	}
}
