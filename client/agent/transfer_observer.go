package agent

import "time"

const transferReportInterval = 250 * time.Millisecond

type transferCounters struct {
	upload   int64
	download int64
}

func (a *Agent) notifyTransfer(tunnelID, direction string, bytes int64) {
	if a.options.Observer == nil || bytes <= 0 {
		return
	}
	a.transferMu.Lock()
	if a.transferPending == nil {
		a.transferPending = make(map[string]*transferCounters)
	}
	pending := a.transferPending[tunnelID]
	if pending == nil {
		pending = &transferCounters{}
		a.transferPending[tunnelID] = pending
	}
	if direction == DirectionUpload {
		pending.upload += bytes
	} else if direction == DirectionDownload {
		pending.download += bytes
	} else {
		a.transferMu.Unlock()
		return
	}
	a.transferMu.Unlock()
}

func (a *Agent) reportTransfer(tunnelID string, upload, download int64) {
	if upload > 0 {
		a.options.Observer.Transfer(tunnelID, DirectionUpload, upload)
	}
	if download > 0 {
		a.options.Observer.Transfer(tunnelID, DirectionDownload, download)
	}
}

func (a *Agent) flushTransfer(tunnelID string) {
	a.transferMu.Lock()
	pending := a.transferPending[tunnelID]
	if pending == nil {
		a.transferMu.Unlock()
		return
	}
	upload, download := pending.upload, pending.download
	delete(a.transferPending, tunnelID)
	a.transferMu.Unlock()
	a.reportTransfer(tunnelID, upload, download)
}

func (a *Agent) transferSnapshot() map[string]transferCounters {
	a.transferMu.Lock()
	defer a.transferMu.Unlock()
	snapshot := make(map[string]transferCounters, len(a.transferPending))
	for tunnelID, pending := range a.transferPending {
		if pending != nil {
			snapshot[tunnelID] = *pending
		}
	}
	return snapshot
}
