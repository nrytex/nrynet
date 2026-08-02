package api

import (
	"context"
	"errors"
)

type Runtime interface {
	StartTunnel(context.Context, string) error
	StopTunnel(context.Context, string) error
	SyncClient(context.Context, string) error
	DisconnectClient(string)
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
