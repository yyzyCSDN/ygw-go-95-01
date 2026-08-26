package meter

type Tier string

const (
	TierPeak   Tier = "peak"
	TierFlat   Tier = "flat"
	TierValley Tier = "valley"
)

type Rate struct {
	peakStart    int
	peakEnd      int
	valleyStart  int
	valleyEnd    int
	energyPrice  float64
	servicePrice float64
}

func NewRate(peakStart, peakEnd, valleyStart, valleyEnd int, energyPrice, servicePrice float64) *Rate {
	return &Rate{
		peakStart:    peakStart,
		peakEnd:      peakEnd,
		valleyStart:  valleyStart,
		valleyEnd:    valleyEnd,
		energyPrice:  energyPrice,
		servicePrice: servicePrice,
	}
}

func (r *Rate) Tier(hour int) Tier {
	if r.inRange(hour, r.peakStart, r.peakEnd) {
		return TierPeak
	}
	if r.inRange(hour, r.valleyStart, r.valleyEnd) {
		return TierValley
	}
	return TierFlat
}

func (r *Rate) inRange(hour, start, end int) bool {
	if start == end {
		return hour == start
	}
	if start < end {
		return hour >= start && hour < end
	}
	return hour >= start || hour < end
}

func (r *Rate) EnergyPrice(t Tier) float64 {
	switch t {
	case TierPeak:
		return r.energyPrice * 1.5
	case TierValley:
		return r.energyPrice * 0.5
	default:
		return r.energyPrice
	}
}

func (r *Rate) ServicePrice(t Tier) float64 {
	switch t {
	case TierPeak:
		return r.servicePrice * 1.2
	case TierValley:
		return r.servicePrice * 0.8
	default:
		return r.servicePrice
	}
}

func (r *Rate) Prices() map[Tier]float64 {
	return map[Tier]float64{
		TierPeak:   r.EnergyPrice(TierPeak),
		TierFlat:   r.EnergyPrice(TierFlat),
		TierValley: r.EnergyPrice(TierValley),
	}
}
