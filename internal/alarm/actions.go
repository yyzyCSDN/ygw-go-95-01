package alarm

import (
	"fmt"
	"time"

	"ygw-go-95-01/internal/grid"
	"ygw-go-95-01/internal/record"
)

type Actions struct {
	grid *grid.Controller
	rec  *record.Recorder
}

func NewActions(g *grid.Controller, rec *record.Recorder) *Actions {
	return &Actions{grid: g, rec: rec}
}

func (a *Actions) Dispatch(al Alarm) error {
	if a.rec != nil {
		_, _ = a.rec.Append(record.Entry{
			Kind:    "alarm.dispatch",
			Message: fmt.Sprintf("%s %s %s", al.Severity, al.Source, al.Message),
			At:      time.Now().UTC(),
		})
	}
	if al.Severity == SeverityCritical && a.grid != nil {
		if err := a.grid.Separate(); err != nil {
			return fmt.Errorf("separate on critical alarm: %w", err)
		}
	}
	return nil
}
