package platform

import "nuntius/internal/core/port"

func NewCollector() port.Collector {
	return newNativeCollector()
}
