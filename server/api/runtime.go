package api

import (
	"context"
	"errors"
	"time"
)

type Runtime interface {
	StartTunnel(context.Context, string) error
	StopTunnel(context.Context, string) error
	SyncClient(context.Context, string) error
	DisconnectClient(string)
	ClientConnectedAt(string) (time.Time, bool)
}

type unavailableRuntime struct{}

func (unavailableRuntime) StartTunnel(context.Context, string) error {
	return errors.New("tunnel runtime unavailable")
}

func (unavailableRuntime) StopTunnel(context.Context, string) error {
	return errors.New("tunnel runtime unavailable")
}

func (unavailableRuntime) SyncClient(context.Context, string) error { return nil }
func (unavailableRuntime) DisconnectClient(string)                  {}
func (unavailableRuntime) ClientConnectedAt(string) (time.Time, bool) {
	return time.Time{}, false
}
