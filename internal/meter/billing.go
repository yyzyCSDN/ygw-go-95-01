package meter

import (
	"fmt"
	"sync"
	"time"

	"ygw-go-95-01/internal/record"
)

type Bill struct {
	ID            string
	Vessel        string
	BerthID       string
	KWh           float64
	EnergyAmount  float64
	ServiceAmount float64
	TotalAmount   float64
	Tier          Tier
	SettledAt     time.Time
}

type Billing struct {
	mu      sync.Mutex
	meter   *Meter
	rate    *Rate
	rec     *record.Recorder
	records map[string][]Bill
	seq     int
}

func NewBilling(meter *Meter, rate *Rate, rec *record.Recorder) *Billing {
	return &Billing{
		meter:   meter,
		rate:    rate,
		rec:     rec,
		records: make(map[string][]Bill),
	}
}

func (b *Billing) Settle(vessel string) (Bill, error) {
	last, ok := b.meter.store.Last(vessel)
	if !ok {
		return Bill{}, ErrNoSamples
	}
	tier := b.rate.Tier(time.Now().Hour())
	energy := last.KWh * b.rate.EnergyPrice(tier)
	service := last.KWh * b.rate.ServicePrice(tier)
	b.mu.Lock()
	b.seq++
	bill := Bill{
		ID:            fmt.Sprintf("bill-%s-%03d", vessel, b.seq),
		Vessel:        vessel,
		BerthID:       last.BerthID,
		KWh:           last.KWh,
		EnergyAmount:  energy,
		ServiceAmount: service,
		TotalAmount:   energy + service,
		Tier:          tier,
		SettledAt:     time.Now().UTC(),
	}
	b.records[vessel] = append(b.records[vessel], bill)
	b.mu.Unlock()
	_ = b.meter.Stop(vessel)
	if b.rec != nil {
		_, _ = b.rec.Append(record.Entry{
			Kind:    "meter.settle",
			Vessel:  vessel,
			BerthID: last.BerthID,
			Message: fmt.Sprintf("kwh %.2f total %.2f tier %s", bill.KWh, bill.TotalAmount, tier),
			At:      bill.SettledAt,
		})
	}
	return bill, nil
}
