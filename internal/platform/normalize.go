package platform

import (
	"net"
	"sort"
	"strings"

	"nuntius/internal/core/model"
)

func normalizeRoutes(routes []model.Route) []model.Route {
	for i := range routes {
		routes[i].Destination = normalizeDestination(routes[i].Destination)
		routes[i].Gateway = normalizeHost(routes[i].Gateway)
		routes[i].Interface = strings.TrimSpace(routes[i].Interface)
	}
	sort.SliceStable(routes, func(i, j int) bool {
		if routes[i].Destination != routes[j].Destination {
			return routes[i].Destination < routes[j].Destination
		}
		if routes[i].Metric != routes[j].Metric {
			return routes[i].Metric < routes[j].Metric
		}
		if routes[i].Gateway != routes[j].Gateway {
			return routes[i].Gateway < routes[j].Gateway
		}
		return routes[i].Interface < routes[j].Interface
	})
	return routes
}

func normalizeDestination(value string) string {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "default", "0/0":
		return "0.0.0.0/0"
	}
	if value == "::/0" {
		return value
	}
	if ip := net.ParseIP(value); ip != nil {
		if ip.To4() != nil {
			return ip.String() + "/32"
		}
		return ip.String() + "/128"
	}
	return value
}

func normalizeHost(value string) string {
	value = strings.TrimSpace(value)
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	return value
}
