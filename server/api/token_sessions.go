package api

import (
	"context"

	"github.com/nat-link/nat-link/internal/model"
	"github.com/nat-link/nat-link/internal/storage"
)

func clientsUsingToken(ctx context.Context, store *storage.Store, tokenID string) ([]model.Client, error) {
	return store.ListClientsByToken(ctx, tokenID)
}

func disconnectClients(runtime Runtime, clients []model.Client) {
	for _, client := range clients {
		runtime.DisconnectClient(client.ID)
	}
}

func stopClientTunnels(ctx context.Context, store *storage.Store, runtime Runtime, clients []model.Client) error {
	for _, client := range clients {
		tunnels, err := store.ListClientTunnels(ctx, client.ID)
		if err != nil {
			return err
		}
		for _, tunnel := range tunnels {
			if tunnel.Status != "running" {
				continue
			}
			if err := runtime.StopTunnel(ctx, tunnel.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
