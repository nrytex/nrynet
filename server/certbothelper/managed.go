package certbothelper

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func readManagedState(path string) (ManagedState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ManagedState{}, err
	}
	var state ManagedState
	if err := json.Unmarshal(data, &state); err != nil {
		return ManagedState{}, fmt.Errorf("parse managed certbot state: %w", err)
	}
	state.Domain = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(state.Domain), "."))
	state.Email = strings.TrimSpace(state.Email)
	if err := ValidateDomain(state.Domain); err != nil {
		return ManagedState{}, fmt.Errorf("managed domain: %w", err)
	}
	if state.Email != "" {
		if err := ValidateEmail(state.Email); err != nil {
			return ManagedState{}, fmt.Errorf("managed email: %w", err)
		}
	}
	return state, nil
}

func writeManagedState(options Options, request Request) error {
	state := ManagedState{
		Domain: request.Domain, Email: request.Email, Updated: time.Now(),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(options.ManagedPath), 0o700); err != nil {
		return err
	}
	return writeAtomic(options.ManagedPath, append(data, '\n'), 0o600)
}

func renewRequestFromManagedState(options Options) (Request, error) {
	state, err := readManagedState(options.ManagedPath)
	if err != nil {
		return Request{}, err
	}
	return Request{Action: "renew", Domain: state.Domain, Email: state.Email}, nil
}
