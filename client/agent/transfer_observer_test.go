package agent

import (
	"sync"
	"testing"
	"time"

	"github.com/nrytex/nrynet/internal/model"
)

func TestNotifyTransferBatchesObserverCallbacks(t *testing.T) {
	observer := &transferObserver{done: make(chan struct{})}
	agent := &Agent{options: Options{Observer: observer}, transferPending: make(map[string]*transferCounters)}
	agent.startTransferFlushLoop()
	agent.notifyTransfer("tun-1", DirectionUpload, 10)
	agent.notifyTransfer("tun-1", DirectionUpload, 20)
	if got := observer.total(); got != 0 {
		t.Fatalf("transfer was reported before the batch timer: %d", got)
	}
	select {
	case <-observer.done:
	case <-time.After(time.Second):
		t.Fatal("batched transfer was not reported")
	}
	if got := observer.total(); got != 30 {
		t.Fatalf("reported=%d want=30", got)
	}
}

func TestFlushTransfersReportsPendingBytes(t *testing.T) {
	observer := &transferObserver{done: make(chan struct{})}
	agent := &Agent{options: Options{Observer: observer}, transferPending: map[string]*transferCounters{
		"tun-1": &transferCounters{upload: 12, download: 8},
	}}
	agent.flushTransfers()
	if got := observer.total(); got != 20 {
		t.Fatalf("reported=%d want=20", got)
	}
}

func TestTransferSnapshotCopiesPendingCounters(t *testing.T) {
	agent := &Agent{transferPending: map[string]*transferCounters{
		"tun-1": &transferCounters{upload: 12, download: 8},
	}}
	snapshot := agent.transferSnapshot()
	snapshot["tun-1"] = transferCounters{}
	if got := agent.transferPending["tun-1"].upload; got != 12 {
		t.Fatalf("pending counters were aliased: %d", got)
	}
}

type transferObserver struct {
	mu         sync.Mutex
	totalBytes int64
	done       chan struct{}
}

func (o *transferObserver) SessionStarted()               {}
func (o *transferObserver) SessionEnded(error)            {}
func (o *transferObserver) TunnelSnapshot([]model.Tunnel) {}
func (o *transferObserver) Transfer(_ string, _ string, bytes int64) {
	o.mu.Lock()
	o.totalBytes += bytes
	select {
	case <-o.done:
	default:
		close(o.done)
	}
	o.mu.Unlock()
}

func (o *transferObserver) total() int64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.totalBytes
}
