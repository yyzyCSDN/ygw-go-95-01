package alarm

import (
	"fmt"

	"ygw-go-95-01/internal/berth"
	"ygw-go-95-01/internal/event"
	"ygw-go-95-01/internal/grid"
)

func (m *Manager) Evaluate(ev event.Event) []Alarm {
	raised := make([]Alarm, 0)
	switch ev.Topic {
	case "grid.state":
		state := ev.Payload["state"]
		if state == string(grid.StateOnGrid) {
			raised = append(raised, m.Raise("grid", "shore power grid connected", SeverityInfo))
		}
		if state == string(grid.StateOff) {
			raised = append(raised, m.Raise("grid", "shore power grid separated", SeverityInfo))
		}
	case "connect.failed":
		raised = append(raised, m.Raise(
			"connect",
			fmt.Sprintf("shore power connection failed for %s", ev.Payload["vessel"]),
			SeverityWarning,
		))
	case "capacity.reconciled":
		raised = append(raised, m.Raise(
			"capacity",
			fmt.Sprintf("capacity drift detected, mismatches %s", ev.Payload["mismatches"]),
			SeverityWarning,
		))
	}
	raised = append(raised, m.driftChecks()...)
	return raised
}

func (m *Manager) driftChecks() []Alarm {
	out := make([]Alarm, 0)
	if m.berths == nil || m.grid == nil {
		return out
	}
	onGrid := m.grid.State() == grid.StateOnGrid
	for _, b := range m.berths.List() {
		if b.State == berth.StateSettled && !onGrid {
			out = append(out, m.Raise(
				"berth",
				fmt.Sprintf("berth %s settled but grid is off", b.Code),
				SeverityWarning,
			))
		}
	}
	return out
}
