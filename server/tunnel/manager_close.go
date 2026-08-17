package tunnel

import (
	"errors"
	"net"
	"time"
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
	for _, timer := range m.reconnectTimers {
		timer.Stop()
	}
	m.reconnectTimers = make(map[string]*time.Timer)
	m.reconnectEpochs = make(map[string]uint64)
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
	m.broker.CloseIdleWorkConnections()
	return errors.Join(errs...)
}
