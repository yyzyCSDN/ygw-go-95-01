package meter

import "time"

type Sample struct {
	Vessel  string
	BerthID string
	KWh     float64
	KVAh    float64
	At      time.Time
	Seq     int
}
