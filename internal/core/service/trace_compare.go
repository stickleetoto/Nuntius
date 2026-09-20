package service

import (
	"sort"

	"nuntius/internal/core/model"
)

func CompareTraces(oldTrace, newTrace model.TraceResult) model.TraceComparison {
	result := model.TraceComparison{
		ReachedChanged: oldTrace.Reached != newTrace.Reached,
		OldReached:     oldTrace.Reached,
		NewReached:     newTrace.Reached,
	}

	oldByHop := make(map[int]model.TraceHop, len(oldTrace.Hops))
	newByHop := make(map[int]model.TraceHop, len(newTrace.Hops))
	hopNumbers := make(map[int]struct{}, len(oldTrace.Hops)+len(newTrace.Hops))

	for _, hop := range oldTrace.Hops {
		oldByHop[hop.Hop] = hop
		hopNumbers[hop.Hop] = struct{}{}
	}
	for _, hop := range newTrace.Hops {
		newByHop[hop.Hop] = hop
		hopNumbers[hop.Hop] = struct{}{}
	}

	orderedHops := make([]int, 0, len(hopNumbers))
	for hop := range hopNumbers {
		orderedHops = append(orderedHops, hop)
	}
	sort.Ints(orderedHops)

	for _, hopNumber := range orderedHops {
		oldHop, oldOK := oldByHop[hopNumber]
		newHop, newOK := newByHop[hopNumber]

		switch {
		case oldOK && !newOK:
			result.RemovedHops = append(result.RemovedHops, oldHop)
		case !oldOK && newOK:
			result.AddedHops = append(result.AddedHops, newHop)
		case oldOK && newOK && oldHop.Address != newHop.Address:
			result.AddressChanges = append(result.AddressChanges, model.TraceHopAddressChange{
				Hop:           hopNumber,
				BeforeAddress: oldHop.Address,
				AfterAddress:  newHop.Address,
			})
		}
	}

	return result
}
