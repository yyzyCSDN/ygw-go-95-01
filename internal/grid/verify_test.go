package grid

import (
	"testing"

	"ygw-go-95-01/internal/berth"
)

func TestSwitchStateUpdateAtomic(t *testing.T) {
	store := berth.NewStore([]berth.Berth{
		{ID: "B1", Code: "B1", VoltageKV: 10, CapacityKVA: 1000, State: berth.StateIdle},
	})
	breaker := NewSimpleBreaker()
	breaker.SetCloseFault(true)
	ctrl := NewController(breaker, NewSyncer(func(p Phase) bool { return true }), store, nil, nil)
	seq := Sequence{
		ID: "sw1",
		Steps: []SequenceStep{
			{Kind: StepSetGridState, GridState: StateOnGrid},
			{Kind: StepSetBerthState, BerthID: "B1", BerthState: berth.StateSettled, Vessel: "shipA"},
			{Kind: StepCloseBreaker},
		},
	}
	if err := ctrl.ApplySequence(seq); err == nil {
		t.Fatalf("sequence must fail when the breaker cannot close")
	}
	if ctrl.State() == StateOnGrid {
		t.Fatalf("grid state must be rolled back after failed sequence")
	}
	st, ok := store.State("B1")
	if !ok {
		t.Fatalf("berth B1 missing")
	}
	if st != berth.StateIdle {
		t.Fatalf("berth state must be rolled back after failed sequence: got %s", st)
	}
	if cache, ok := store.CachedState("B1"); !ok || cache != berth.StateIdle {
		t.Fatalf("berth cache must be rolled back after failed sequence: got %s", cache)
	}
}
