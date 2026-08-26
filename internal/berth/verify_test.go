package berth

import "testing"

func TestRecoveryRebuildsFromLiveState(t *testing.T) {
	store := NewStore([]Berth{
		{ID: "B1", Code: "B1", VoltageKV: 10, CapacityKVA: 1000, State: StateIdle},
	})
	if err := store.MarkOccupied("B1", "shipA"); err != nil {
		t.Fatal(err)
	}
	if err := store.PersistSnapshot(); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkIdle("B1"); err != nil {
		t.Fatal(err)
	}
	live := make(map[string]Berth)
	for _, b := range store.List() {
		live[b.ID] = b
	}
	if err := store.RebuildFromLive(live); err != nil {
		t.Fatal(err)
	}
	st, ok := store.State("B1")
	if !ok {
		t.Fatalf("berth B1 missing after recovery")
	}
	if st != StateIdle {
		t.Fatalf("recovery must rebuild from live state: berth B1 is %s", st)
	}
}
