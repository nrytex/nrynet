package relay

import (
	"context"
	"errors"
)

func (b *Broker) authenticateDataClient(tokenValue, deviceID string) (string, uint64, error) {
	client, err := b.authCache.clientByDevice(context.Background(), deviceID)
	if err != nil {
		return "", 0, err
	}
	token, err := b.authCache.authenticate(context.Background(), tokenValue)
	if err != nil {
		return "", 0, err
	}
	if client.Disabled || client.TokenID != token.ID {
		return "", 0, errors.New("client is not authorized")
	}
	return client.ID, b.connections.generationFor(client.ID), nil
}
