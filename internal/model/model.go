package model

import "time"

type Client struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	DeviceID   string    `json:"device_id"`
	TokenID    string    `json:"token_id"`
	Status     string    `json:"status"`
	Disabled   bool      `json:"disabled"`
	IP         string    `json:"ip"`
	OS         string    `json:"os"`
	Version    string    `json:"version"`
	LastOnline time.Time `json:"last_online"`
	CreatedAt  time.Time `json:"created_at"`
}

type Token struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	Disabled  bool       `json:"disabled"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type Tunnel struct {
	ID          string    `json:"id"`
	ClientID    string    `json:"client_id"`
	Name        string    `json:"name"`
	Protocol    string    `json:"protocol"`
	LocalHost   string    `json:"local_host"`
	LocalPort   int       `json:"local_port"`
	RemotePort  int       `json:"remote_port"`
	Domain      string    `json:"domain"`
	Status      string    `json:"status"`
	IPAllowlist []string  `json:"ip_allowlist"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Traffic struct {
	TunnelID string    `json:"tunnel_id"`
	Upload   int64     `json:"upload"`
	Download int64     `json:"download"`
	At       time.Time `json:"created_at"`
}

type TrafficSummary struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}

type TrafficTarget struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
}

type Event struct {
	ID        int64          `json:"id"`
	Level     string         `json:"level"`
	Event     string         `json:"event"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields"`
	CreatedAt time.Time      `json:"created_at"`
}
