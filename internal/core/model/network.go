package model

import "time"

type IPAddress struct {
	Address string `json:"address"`
	Family  string `json:"family"`
	CIDR    string `json:"cidr,omitempty"`
}

type NetworkInterface struct {
	Name      string      `json:"name"`
	Index     int         `json:"index"`
	MAC       string      `json:"mac,omitempty"`
	MTU       int         `json:"mtu"`
	Flags     []string    `json:"flags,omitempty"`
	Addresses []IPAddress `json:"addresses,omitempty"`
}

type Route struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway,omitempty"`
	Interface   string `json:"interface,omitempty"`
	Metric      int    `json:"metric,omitempty"`
	Raw         string `json:"raw,omitempty"`
}

type DNSConfig struct {
	Servers []string `json:"servers,omitempty"`
	Search  []string `json:"search,omitempty"`
	Source  string   `json:"source,omitempty"`
}

type Listener struct {
	Protocol string `json:"protocol"`
	Local    string `json:"local"`
	PID      int    `json:"pid,omitempty"`
	Process  string `json:"process,omitempty"`
}

type Connection struct {
	Protocol string `json:"protocol"`
	Local    string `json:"local"`
	Remote   string `json:"remote"`
	State    string `json:"state,omitempty"`
	PID      int    `json:"pid,omitempty"`
	Process  string `json:"process,omitempty"`
}

type NetworkState struct {
	CapturedAt  time.Time          `json:"captured_at"`
	Hostname    string             `json:"hostname"`
	OS          string             `json:"os"`
	Arch        string             `json:"arch"`
	Interfaces  []NetworkInterface `json:"interfaces"`
	Routes      []Route            `json:"routes,omitempty"`
	DNS         DNSConfig          `json:"dns"`
	Listeners   []Listener         `json:"listeners,omitempty"`
	Connections []Connection       `json:"connections,omitempty"`
	Warnings    []string           `json:"warnings,omitempty"`
}

type Snapshot struct {
	Name      string       `json:"name"`
	CreatedAt time.Time    `json:"created_at"`
	Version   int          `json:"schema_version"`
	State     NetworkState `json:"state"`
}
