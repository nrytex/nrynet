package certbothelper

import "time"

type Request struct {
	Action  string `json:"action"`
	Domain  string `json:"domain,omitempty"`
	Email   string `json:"email,omitempty"`
	Staging bool   `json:"staging,omitempty"`
}

type Status struct {
	Action   string    `json:"action"`
	Domain   string    `json:"domain,omitempty"`
	Email    string    `json:"email,omitempty"`
	State    string    `json:"state"`
	Message  string    `json:"message,omitempty"`
	Output   string    `json:"output,omitempty"`
	Updated  time.Time `json:"updated"`
	CertFile string    `json:"cert_file,omitempty"`
	KeyFile  string    `json:"key_file,omitempty"`
}

type ManagedState struct {
	Domain  string    `json:"domain"`
	Email   string    `json:"email,omitempty"`
	Updated time.Time `json:"updated"`
}
