package model

type TraceHopAddressChange struct {
	Hop           int    `json:"hop"`
	BeforeAddress string `json:"before_address,omitempty"`
	AfterAddress  string `json:"after_address,omitempty"`
}

type TraceComparison struct {
	RemovedHops    []TraceHop              `json:"removed_hops,omitempty"`
	AddedHops      []TraceHop              `json:"added_hops,omitempty"`
	AddressChanges []TraceHopAddressChange `json:"address_changes,omitempty"`
	ReachedChanged bool                    `json:"reached_changed"`
	OldReached     bool                    `json:"old_reached"`
	NewReached     bool                    `json:"new_reached"`
}
