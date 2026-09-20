package service

import (
	"reflect"
	"testing"

	"nuntius/internal/core/model"
)

func TestCompareTraces(t *testing.T) {
	tests := []struct {
		name string
		old  model.TraceResult
		new  model.TraceResult
		want model.TraceComparison
	}{
		{
			name: "unchanged trace",
			old: model.TraceResult{
				Reached: true,
				Hops: []model.TraceHop{
					{Hop: 1, Address: "192.0.2.1"},
					{Hop: 2, Address: "192.0.2.2"},
				},
			},
			new: model.TraceResult{
				Reached: true,
				Hops: []model.TraceHop{
					{Hop: 1, Address: "192.0.2.1"},
					{Hop: 2, Address: "192.0.2.2"},
				},
			},
			want: model.TraceComparison{
				OldReached: true,
				NewReached: true,
			},
		},
		{
			name: "address changed at same hop",
			old: model.TraceResult{
				Hops: []model.TraceHop{{Hop: 2, Address: "192.0.2.2"}},
			},
			new: model.TraceResult{
				Hops: []model.TraceHop{{Hop: 2, Address: "198.51.100.2"}},
			},
			want: model.TraceComparison{
				AddressChanges: []model.TraceHopAddressChange{
					{
						Hop:           2,
						BeforeAddress: "192.0.2.2",
						AfterAddress:  "198.51.100.2",
					},
				},
			},
		},
		{
			name: "added and removed hops",
			old: model.TraceResult{
				Hops: []model.TraceHop{
					{Hop: 1, Address: "192.0.2.1"},
					{Hop: 3, Address: "192.0.2.3"},
				},
			},
			new: model.TraceResult{
				Hops: []model.TraceHop{
					{Hop: 2, Address: "198.51.100.2"},
					{Hop: 3, Address: "192.0.2.3"},
				},
			},
			want: model.TraceComparison{
				RemovedHops: []model.TraceHop{{Hop: 1, Address: "192.0.2.1"}},
				AddedHops:   []model.TraceHop{{Hop: 2, Address: "198.51.100.2"}},
			},
		},
		{
			name: "reachability changed",
			old:  model.TraceResult{Reached: false},
			new:  model.TraceResult{Reached: true},
			want: model.TraceComparison{
				ReachedChanged: true,
				OldReached:     false,
				NewReached:     true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareTraces(tt.old, tt.new)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("CompareTraces() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
