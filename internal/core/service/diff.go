package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"nuntius/internal/core/model"
	"nuntius/internal/core/port"
)

type DiffService struct{ Storage port.SnapshotStorage }

func (s DiffService) Compare(ctx context.Context, fromName, toName string) (model.DiffResult, error) {
	from, err := s.Storage.Load(ctx, fromName)
	if err != nil {
		return model.DiffResult{}, err
	}
	to, err := s.Storage.Load(ctx, toName)
	if err != nil {
		return model.DiffResult{}, err
	}
	return CompareSnapshots(from, to), nil
}

func CompareSnapshots(from, to model.Snapshot) model.DiffResult {
	result := model.DiffResult{From: from.Name, To: to.Name}
	compareScalar(&result, "host.hostname", from.State.Hostname, to.State.Hostname)
	compareScalar(&result, "host.os", from.State.OS, to.State.OS)
	compareScalar(&result, "host.arch", from.State.Arch, to.State.Arch)
	compareSet(&result, "dns.server", from.State.DNS.Servers, to.State.DNS.Servers)
	compareSet(&result, "dns.search", from.State.DNS.Search, to.State.DNS.Search)
	compareSet(&result, "interface", interfaceKeys(from.State.Interfaces), interfaceKeys(to.State.Interfaces))
	compareSet(&result, "route", routeKeys(from.State.Routes), routeKeys(to.State.Routes))
	compareSet(&result, "listener", listenerKeys(from.State.Listeners), listenerKeys(to.State.Listeners))
	compareSet(&result, "connection", connectionKeys(from.State.Connections), connectionKeys(to.State.Connections))
	sort.Slice(result.Changes, func(i, j int) bool {
		if result.Changes[i].Kind == result.Changes[j].Kind {
			return result.Changes[i].Key < result.Changes[j].Key
		}
		return result.Changes[i].Kind < result.Changes[j].Kind
	})
	return result
}

func compareScalar(r *model.DiffResult, key, before, after string) {
	if before != after {
		r.Changes = append(r.Changes, model.Change{Kind: "changed", Key: key, Before: before, After: after})
	}
}

func compareSet(r *model.DiffResult, key string, before, after []string) {
	b := make(map[string]struct{}, len(before))
	a := make(map[string]struct{}, len(after))
	for _, v := range before {
		b[v] = struct{}{}
	}
	for _, v := range after {
		a[v] = struct{}{}
	}
	for v := range b {
		if _, ok := a[v]; !ok {
			r.Changes = append(r.Changes, model.Change{Kind: "removed", Key: key, Before: v})
		}
	}
	for v := range a {
		if _, ok := b[v]; !ok {
			r.Changes = append(r.Changes, model.Change{Kind: "added", Key: key, After: v})
		}
	}
}

func interfaceKeys(values []model.NetworkInterface) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		addrs := make([]string, 0, len(v.Addresses))
		for _, a := range v.Addresses {
			addrs = append(addrs, a.CIDR)
		}
		sort.Strings(addrs)
		flags := append([]string(nil), v.Flags...)
		sort.Strings(flags)
		out = append(out, fmt.Sprintf("%s|mac=%s|mtu=%d|flags=%s|addr=%s", v.Name, v.MAC, v.MTU, strings.Join(flags, ","), strings.Join(addrs, ",")))
	}
	return out
}
func routeKeys(values []model.Route) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, fmt.Sprintf("%s|via=%s|if=%s|metric=%d", v.Destination, v.Gateway, v.Interface, v.Metric))
	}
	return out
}
func listenerKeys(values []model.Listener) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, fmt.Sprintf("%s|%s|pid=%d|proc=%s", v.Protocol, v.Local, v.PID, v.Process))
	}
	return out
}
func connectionKeys(values []model.Connection) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, fmt.Sprintf("%s|%s->%s|%s|pid=%d|proc=%s", v.Protocol, v.Local, v.Remote, v.State, v.PID, v.Process))
	}
	return out
}

func PrettyJSON(v any) string { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }
