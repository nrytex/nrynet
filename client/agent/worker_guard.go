package agent

import (
	"fmt"
	"runtime/debug"
)

func (a *Agent) goWorker(name string, work func()) {
	go func() {
		defer a.recoverWorker(name)
		work()
	}()
}

func (a *Agent) runWorker(name string, work func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			a.logWorkerPanic(name, recovered)
			err = fmt.Errorf("%s worker panicked: %v", name, recovered)
		}
	}()
	return work()
}

func (a *Agent) recoverWorker(name string) {
	if recovered := recover(); recovered != nil {
		a.logWorkerPanic(name, recovered)
	}
}

func (a *Agent) logWorkerPanic(name string, recovered any) {
	if a.logger == nil {
		return
	}
	a.logger.Error("agent worker panicked", "worker", name, "panic", recovered, "stack", string(debug.Stack()))
}
