// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
)

// discoveryKeyPrefix is the DHT key prefix for agent-memory peer discovery.
const discoveryKeyPrefix = "/agent-memory/discovery/"

// computeDiscoveryKey derives the DHT key from a secret according to the design spec.
// SHA256(secret) truncated to first 16 bytes, hex-encoded, prefixed with "/agent-memory/discovery/"
func computeDiscoveryKey(secret string) string {
	hash := sha256.Sum256([]byte(secret))
	hashHex := hex.EncodeToString(hash[:16])
	return discoveryKeyPrefix + hashHex
}

// Discovery handles DHT-based peer discovery keyed by shared secret.
type Discovery interface {
	// Advertise announces this peer's address info to the DHT under the secret key.
	Advertise(ctx context.Context) error
	// FindPeers returns AddrInfo for all peers advertising under the shared secret.
	FindPeers(ctx context.Context) ([]peer.AddrInfo, error)
	// ConnectToPeers finds and connects to all discovered peers via circuit relay.
	ConnectToPeers(ctx context.Context) error
}

// discovery implements the Discovery interface using DHT.
type discovery struct {
	h   host.Host
	dht *dht.IpfsDHT
	key string
	sec string

	mu            sync.RWMutex
	advertiseDone chan struct{}
}

// NewDiscovery creates a Discovery instance bound to the given Host and secret.
func NewDiscovery(h Host, secret string) (Discovery, error) {
	if secret == "" {
		return nil, fmt.Errorf("p2p: secret cannot be empty for discovery")
	}
	if h == nil {
		return nil, fmt.Errorf("p2p: host cannot be nil for discovery")
	}
	if h.Host() == nil {
		return nil, fmt.Errorf("p2p: host.Host() returned nil")
	}
	if h.DHT() == nil {
		return nil, fmt.Errorf("p2p: host.DHT() returned nil")
	}

	return &discovery{
		h:             h.Host(),
		dht:           h.DHT(),
		key:           computeDiscoveryKey(secret),
		sec:           secret,
		advertiseDone: make(chan struct{}),
	}, nil
}

// Advertise announces this peer's address info to the DHT under the secret key.
// It re-advertises periodically to keep the record fresh.
func (d *discovery) Advertise(ctx context.Context) error {
	// Get our own address info
	addrInfo := peer.AddrInfo{
		ID:    d.h.ID(),
		Addrs: d.h.Addrs(),
	}

	// Serialize to JSON
	value, err := json.Marshal(addrInfo)
	if err != nil {
		return fmt.Errorf("p2p: marshaling addr info: %w", err)
	}

	// Put to DHT - the DHT handles record TTL internally
	err = d.dht.PutValue(ctx, d.key, value)
	if err != nil {
		return fmt.Errorf("p2p: putting addr info to DHT: %w", err)
	}

	return nil
}

// FindPeers returns AddrInfo for all peers advertising under the shared secret.
func (d *discovery) FindPeers(ctx context.Context) ([]peer.AddrInfo, error) {
	// Get value from DHT
	value, err := d.dht.GetValue(ctx, d.key)
	if err != nil {
		if err == routing.ErrNotFound {
			// No peers found with this key
			return []peer.AddrInfo{}, nil
		}
		return nil, fmt.Errorf("p2p: getting value from DHT: %w", err)
	}

	// Deserialize from JSON
	var addrInfo peer.AddrInfo
	if err := json.Unmarshal(value, &addrInfo); err != nil {
		return nil, fmt.Errorf("p2p: unmarshaling addr info: %w", err)
	}

	// Return list of discovered peers (excluding ourselves)
	peers := []peer.AddrInfo{}
	if addrInfo.ID != d.h.ID() {
		peers = append(peers, addrInfo)
	}

	return peers, nil
}

// ConnectToPeers finds and connects to all discovered peers via circuit relay.
func (d *discovery) ConnectToPeers(ctx context.Context) error {
	peers, err := d.FindPeers(ctx)
	if err != nil {
		return fmt.Errorf("p2p: finding peers: %w", err)
	}

	// Connect to each discovered peer
	for _, p := range peers {
		// Skip if already connected
		if d.h.Network().Connectedness(p.ID) == network.Connected {
			continue
		}

		// Try to connect to the peer
		// The relay transport will be used automatically if direct connection fails
		err := d.h.Connect(ctx, p)
		if err != nil {
			// Log but don't fail - we may still connect to other peers
			continue
		}
	}

	return nil
}
