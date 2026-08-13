package relay

import (
	"context"
	"sync"
	"time"

	"github.com/nrytex/nrynet/internal/auth"
	"github.com/nrytex/nrynet/internal/model"
	"github.com/nrytex/nrynet/internal/storage"
)

const relayAuthCacheTTL = 5 * time.Second

// Data-channel handshakes are short-lived and repeat for every visitor. The
// cache removes that hot-path SQLite lookup without making credential changes
// permanent: admin mutations explicitly invalidate it and the TTL is short.

type relayAuthCache struct {
	auth  *auth.Service
	store *storage.Store
	ttl   time.Duration

	mu        sync.Mutex
	byToken   map[string]relayTokenEntry
	byDevice  map[string]relayTokenEntry
	byTokenID map[string]relayTokenEntry
	loading   map[string]*relayAuthLoad
}

type relayTokenEntry struct {
	token    model.Token
	client   model.Client
	expires  time.Time
	verified bool
}

type relayAuthLoad struct {
	done chan struct{}
}

func newRelayAuthCache(authService *auth.Service, store *storage.Store) *relayAuthCache {
	return &relayAuthCache{
		auth:      authService,
		store:     store,
		ttl:       relayAuthCacheTTL,
		byToken:   make(map[string]relayTokenEntry),
		byDevice:  make(map[string]relayTokenEntry),
		byTokenID: make(map[string]relayTokenEntry),
		loading:   make(map[string]*relayAuthLoad),
	}
}

func (c *relayAuthCache) authenticate(ctx context.Context, value string) (model.Token, error) {
	if entry, ok := c.cachedToken(value); ok {
		return entry.token, nil
	}
	load, owner := c.beginLoad("token:" + value)
	if !owner {
		select {
		case <-load.done:
			if entry, ok := c.cachedToken(value); ok {
				return entry.token, nil
			}
		case <-ctx.Done():
			return model.Token{}, ctx.Err()
		}
	}
	defer c.finishLoad("token:"+value, load)
	token, err := c.auth.AuthenticateAgent(ctx, value)
	if err != nil {
		return model.Token{}, err
	}
	c.mu.Lock()
	c.byToken[value] = relayTokenEntry{token: token, expires: time.Now().Add(c.ttl), verified: true}
	c.mu.Unlock()
	return token, nil
}

func (c *relayAuthCache) clientByDevice(ctx context.Context, deviceID string) (model.Client, error) {
	if entry, ok := c.cachedClient(deviceID); ok {
		return entry.client, nil
	}
	key := "device:" + deviceID
	load, owner := c.beginLoad(key)
	if !owner {
		select {
		case <-load.done:
			if entry, ok := c.cachedClient(deviceID); ok {
				return entry.client, nil
			}
		case <-ctx.Done():
			return model.Client{}, ctx.Err()
		}
	}
	defer c.finishLoad(key, load)
	client, err := c.store.GetClientByDevice(ctx, deviceID)
	if err != nil {
		return model.Client{}, err
	}
	c.mu.Lock()
	entry := c.byDevice[deviceID]
	entry.client = client
	entry.expires = time.Now().Add(c.ttl)
	entry.verified = true
	c.byDevice[deviceID] = entry
	c.mu.Unlock()
	return client, nil
}

func (c *relayAuthCache) tokenByID(ctx context.Context, tokenID string) (model.Token, error) {
	if entry, ok := c.cachedTokenID(tokenID); ok {
		return entry.token, nil
	}
	key := "token-id:" + tokenID
	load, owner := c.beginLoad(key)
	if !owner {
		select {
		case <-load.done:
			if entry, ok := c.cachedTokenID(tokenID); ok {
				return entry.token, nil
			}
		case <-ctx.Done():
			return model.Token{}, ctx.Err()
		}
	}
	defer c.finishLoad(key, load)
	token, err := c.store.GetToken(ctx, tokenID)
	if err != nil {
		return model.Token{}, err
	}
	c.mu.Lock()
	entry := c.byTokenID[tokenID]
	entry.token = token
	entry.expires = time.Now().Add(c.ttl)
	entry.verified = true
	c.byTokenID[tokenID] = entry
	c.mu.Unlock()
	return token, nil
}

func (c *relayAuthCache) invalidateAll() {
	c.mu.Lock()
	clear(c.byToken)
	clear(c.byDevice)
	clear(c.byTokenID)
	c.mu.Unlock()
}

func (c *relayAuthCache) beginLoad(key string) (*relayAuthLoad, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if load := c.loading[key]; load != nil {
		return load, false
	}
	load := &relayAuthLoad{done: make(chan struct{})}
	c.loading[key] = load
	return load, true
}

func (c *relayAuthCache) finishLoad(key string, load *relayAuthLoad) {
	c.mu.Lock()
	if c.loading[key] == load {
		delete(c.loading, key)
		close(load.done)
	}
	c.mu.Unlock()
}

func (c *relayAuthCache) invalidateClient(clientID string) {
	// A client mutation can affect both the device lookup and the token bound
	// to it. Clearing all short-lived entries avoids retaining either side when
	// one of the related entries has already expired.
	c.invalidateAll()
}

func (c *relayAuthCache) invalidateToken(tokenID string) {
	c.mu.Lock()
	for value, entry := range c.byToken {
		if entry.token.ID == tokenID {
			delete(c.byToken, value)
		}
	}
	delete(c.byTokenID, tokenID)
	c.mu.Unlock()
}

func (c *relayAuthCache) invalidateDevice(deviceID string) {
	c.invalidateAll()
}

func (c *relayAuthCache) cachedToken(value string) (relayTokenEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.byToken[value]
	if !ok || time.Now().After(entry.expires) {
		delete(c.byToken, value)
		return relayTokenEntry{}, false
	}
	return entry, entry.verified && !entry.token.Disabled
}

func (c *relayAuthCache) cachedClient(deviceID string) (relayTokenEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.byDevice[deviceID]
	if !ok || time.Now().After(entry.expires) {
		delete(c.byDevice, deviceID)
		return relayTokenEntry{}, false
	}
	return entry, entry.verified && !entry.client.Disabled
}

func (c *relayAuthCache) cachedTokenID(tokenID string) (relayTokenEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.byTokenID[tokenID]
	if !ok || time.Now().After(entry.expires) {
		delete(c.byTokenID, tokenID)
		return relayTokenEntry{}, false
	}
	return entry, entry.verified && !entry.token.Disabled
}
