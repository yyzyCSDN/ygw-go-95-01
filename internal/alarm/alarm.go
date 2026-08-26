package alarm

import (
	"fmt"
	"sync"
	"time"

	"ygw-go-95-01/internal/berth"
	"ygw-go-95-01/internal/grid"
)

type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

type Alarm struct {
	ID       string
	Severity Severity
	Source   string
	Message  string
	At       time.Time
	Ack      bool
}

type Manager struct {
	mu      sync.Mutex
	grid    *grid.Controller
	berths  *berth.Store
	actions *Actions
	alarms  []Alarm
	seq     int
}

func NewManager(g *grid.Controller, b *berth.Store, actions *Actions) *Manager {
	return &Manager{
		grid:    g,
		berths:  b,
		actions: actions,
	}
}

func (m *Manager) Raise(source, message string, sev Severity) Alarm {
	m.mu.Lock()
	m.seq++
	al := Alarm{
		ID:       fmt.Sprintf("al-%03d", m.seq),
		Severity: sev,
		Source:   source,
		Message:  message,
		At:       time.Now().UTC(),
	}
	m.alarms = append(m.alarms, al)
	m.mu.Unlock()
	if m.actions != nil {
		_ = m.actions.Dispatch(al)
	}
	return al
}

func (m *Manager) Alarms() []Alarm {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Alarm, len(m.alarms))
	copy(out, m.alarms)
	return out
}

func (m *Manager) Ack(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.alarms {
		if m.alarms[i].ID == id {
			m.alarms[i].Ack = true
			return nil
		}
	}
	return fmt.Errorf("alarm %s not found", id)
}
