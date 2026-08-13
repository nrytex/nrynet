package tunnel

import (
	"errors"
	"net"
)

func (m *Manager) Close() error {
	m.mu.Lock()
	listeners := make([]net.Listener, 0, len(m.listeners))
	for _, listener := range m.listeners {
		listeners = append(listeners, listener)
	}
	udpRuntimes := make([]*udpRuntime, 0, len(m.udpRuntimes))
	for _, runtime := range m.udpRuntimes {
		udpRuntimes = append(udpRuntimes, runtime)
	}
	m.listeners = make(map[string]net.Listener)
	m.udpRuntimes = make(map[string]*udpRuntime)
	relayBinds := make([]RelayBinding, 0, len(m.relayBinds))
	for _, binding := range m.relayBinds {
		relayBinds = append(relayBinds, binding)
	}
	m.relayBinds = make(map[string]RelayBinding)
	m.mu.Unlock()
	var errs []error
	for _, listener := range listeners {
		errs = append(errs, listener.Close())
	}
	for _, runtime := range udpRuntimes {
		errs = append(errs, runtime.close())
	}
	for _, binding := range relayBinds {
		errs = append(errs, binding.Close())
	}
	if m.traffic != nil {
		m.traffic.close()
	}
	return errors.Join(errs...)
}
