package grid

import (
	"context"
	"testing"
	"time"
)

func TestPhaseCheckRequiredBeforeClose(t *testing.T) {
	ctrl := NewController(NewSimpleBreaker(), NewSyncer(func(p Phase) bool { return true }), nil, nil, nil)
	phase := Phase{VoltageKV: 10, FreqHz: 50, Degree: 0}
	err := ctrl.SyncAndClose(context.Background(), phase, time.Second)
	if err == nil {
		t.Fatalf("phase check must be required before closing")
	}
	if ctrl.BreakerClosed() {
		t.Fatalf("breaker must stay open when phase check is missing")
	}
	if ctrl.State() != StateOff {
		t.Fatalf("grid must stay off when phase check is missing")
	}
}
