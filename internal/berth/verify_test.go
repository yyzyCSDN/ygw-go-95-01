package berth

import "testing"

func TestAllocUsesFreshBerthState(t *testing.T) {
	store := NewStore([]Berth{
		{ID: "B1", Code: "B1", VoltageKV: 10, CapacityKVA: 1000, State: StateIdle},
	})
	if err := store.MarkOccupied("B1", "shipA"); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkIdle("B1"); err != nil {
		t.Fatal(err)
	}
	st, ok := store.CachedState("B1")
	if !ok {
		t.Fatalf("berth B1 must stay in the allocation cache after going idle")
	}
	if st != StateIdle {
		t.Fatalf("allocation must read fresh berth state: got %s want idle", st)
	}
	if !store.CanAccept("B1", 500) {
		t.Fatalf("idle berth must accept a new allocation")
	}
}
