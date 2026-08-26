package capacity

import (
	"sort"

	"ygw-go-95-01/internal/berth"
)

type PlanItem struct {
	BerthID      string
	Code         string
	VoltageKV    float64
	CapacityKVA  float64
	State        berth.State
	Vessel       string
	AllocatedKVA float64
}

type Plan struct {
	ShoreKVA  float64
	UsedKVA   float64
	RemainKVA float64
	Items     []PlanItem
}

func BuildPlan(store *berth.Store, alloc *Allocator) Plan {
	plan := Plan{
		ShoreKVA:  alloc.Shore(),
		UsedKVA:   alloc.UsedTotal(),
		RemainKVA: alloc.Remain(),
	}
	berths := store.List()
	sort.Slice(berths, func(i, j int) bool {
		return berths[i].Code < berths[j].Code
	})
	for _, b := range berths {
		item := PlanItem{
			BerthID:     b.ID,
			Code:        b.Code,
			VoltageKV:   b.VoltageKV,
			CapacityKVA: b.CapacityKVA,
			State:       b.State,
			Vessel:      b.Vessel,
		}
		if alloc, ok := alloc.AllocationOf(b.ID); ok {
			item.AllocatedKVA = alloc.KVA
		}
		plan.Items = append(plan.Items, item)
	}
	return plan
}
