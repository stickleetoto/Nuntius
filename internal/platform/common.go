package platform

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"nuntius/internal/core/model"
)

func collectInterfaces() ([]model.NetworkInterface, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	out := make([]model.NetworkInterface, 0, len(ifs))
	for _, iface := range ifs {
		item := model.NetworkInterface{
			Name:  iface.Name,
			Index: iface.Index,
			MAC:   iface.HardwareAddr.String(),
			MTU:   iface.MTU,
		}
		item.Flags = splitFlags(iface.Flags.String())

		addrs, err := iface.Addrs()
		if err == nil {
			for _, addr := range addrs {
				ip, ipnet := parseAddr(addr.String())
				if ip == nil {
					continue
				}
				family := "ipv6"
				if ip.To4() != nil {
					family = "ipv4"
				}
				cidr := ""
				if ipnet != nil {
					ones, _ := ipnet.Mask.Size()
					cidr = fmt.Sprintf("%s/%d", ip.String(), ones)
				}
				item.Addresses = append(item.Addresses, model.IPAddress{Address: ip.String(), Family: family, CIDR: cidr})
			}
		}
		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

func parseAddr(s string) (net.IP, *net.IPNet) {
	ip, ipnet, err := net.ParseCIDR(s)
	if err == nil {
		return ip, ipnet
	}
	return net.ParseIP(s), nil
}

func splitFlags(s string) []string {
	if s == "" || s == "0" {
		return nil
	}
	parts := strings.Split(s, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
