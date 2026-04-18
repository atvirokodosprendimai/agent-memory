// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	libp2precord "github.com/libp2p/go-libp2p-record"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
)

// ErrNotImplemented is returned when a function is not yet implemented.
var ErrNotImplemented = errors.New("p2p: not implemented")

// Host provides the libp2p host and DHT to other P2P components.
type Host interface {
	// Host returns the underlying libp2p host.
	Host() host.Host
	// DHT returns the DHT client.
	DHT() *dht.IpfsDHT
	// PeerID returns the peer ID of this host.
	PeerID() peer.ID
	// Close shuts down the host.
	Close() error
}

// p2pHost implements the Host interface.
type p2pHost struct {
	h  host.Host
	d  *dht.IpfsDHT
	id peer.ID
}

// NewHost creates a libp2p host with a random peer ID.
// The secret is used to derive the DHT discovery key (not the peer ID itself).
// The host key persists in dataDir/p2p/hostkey across restarts.
func NewHost(ctx context.Context, secret string, dataDir string) (Host, error) {
	if secret == "" {
		return nil, errors.New("p2p: secret cannot be empty")
	}
	if dataDir == "" {
		return nil, errors.New("p2p: dataDir cannot be empty")
	}
	if ctx == nil {
		return nil, errors.New("p2p: ctx cannot be nil")
	}

	p2pDir := filepath.Join(dataDir, "p2p")
	if err := os.MkdirAll(p2pDir, 0700); err != nil {
		return nil, fmt.Errorf("p2p: creating p2p directory: %w", err)
	}

	hostKeyPath := filepath.Join(p2pDir, "hostkey")
	privKey, err := loadOrCreateHostKey(hostKeyPath)
	if err != nil {
		return nil, fmt.Errorf("p2p: loading host key: %w", err)
	}

	id, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("p2p: deriving peer ID from key: %w", err)
	}

	// Get public bootstrap relay addresses
	relayAddresses := getDefaultRelayAddresses()

	// Create libp2p host.
	// Listen on loopback only — DHT needs a local address to function.
	// External connectivity goes through circuit relay v2 auto-relay.
	// NoListenAddrs is NOT used because DHT client mode requires listen addrs
	// to connect to bootstrap peers and populate its routing table.
	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"),
		libp2p.EnableRelay(),
		libp2p.EnableAutoRelay(
			autorelay.WithStaticRelays(relayAddresses),
			autorelay.WithNumRelays(2),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("p2p: creating libp2p host: %w", err)
	}

	// Create DHT client mode with custom validator and protocol prefix for peer discovery.
	// We use a custom protocol prefix so we can use our own validator
	// This prefix should be unique to agent-memory to avoid conflicts
	// Note: SplitKey splits by '/' so /agent-memory/discovery/xxx -> namespace="agent-memory", path="discovery/xxx"
	// We register under "agent-memory" namespace and handle validation in the validator
	customValidator := libp2precord.NamespacedValidator{
		"agent-memory": &discoveryValidator{},
	}

	// Use a custom protocol prefix so we can use our own validator
	// This prefix should be unique to agent-memory to avoid conflicts
	customPrefix := protocol.ID("/agent-memory/kad/1.0.0")

	d, err := dht.New(ctx, h,
		dht.Mode(dht.ModeClient),
		dht.BootstrapPeers(getDHTBootstrapPeers()...),
		dht.Validator(customValidator),
		dht.ProtocolPrefix(customPrefix),
	)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("p2p: creating DHT client: %w", err)
	}

	// Bootstrap the DHT to connect to bootstrap peers and populate routing table.
	// This is required for the DHT client to be able to put/get values.
	if err := d.Bootstrap(ctx); err != nil {
		d.Close()
		h.Close()
		return nil, fmt.Errorf("p2p: bootstrapping DHT: %w", err)
	}

	return &p2pHost{
		h:  h,
		d:  d,
		id: id,
	}, nil
}

func (h *p2pHost) Host() host.Host {
	return h.h
}

func (h *p2pHost) DHT() *dht.IpfsDHT {
	return h.d
}

func (h *p2pHost) PeerID() peer.ID {
	return h.id
}

func (h *p2pHost) Close() error {
	var errs []error

	if h.d != nil {
		if err := h.d.Close(); err != nil {
			errs = append(errs, fmt.Errorf("p2p: closing DHT: %w", err))
		}
	}

	if h.h != nil {
		if err := h.h.Close(); err != nil {
			errs = append(errs, fmt.Errorf("p2p: closing host: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// loadOrCreateHostKey loads a host key from disk or generates a new random one.
// Each client gets a unique random peer ID, independent of the secret.
// The secret is used only for DHT discovery key derivation (see computeDiscoveryKey).
func loadOrCreateHostKey(path string) (crypto.PrivKey, error) {
	// Try to load existing key (stored as raw 64-byte Ed25519 seed)
	keyBytes, err := os.ReadFile(path)
	if err == nil && len(keyBytes) == ed25519.PrivateKeySize {
		privKey, err := crypto.UnmarshalEd25519PrivateKey(keyBytes)
		if err == nil {
			return privKey, nil
		}
	}

	// No valid key on disk — generate a fresh random Ed25519 key
	_, seed, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating ed25519 key: %w", err)
	}

	privKey, err := crypto.UnmarshalEd25519PrivateKey(seed)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling ed25519 private key: %w", err)
	}

	// Persist the raw seed so restarts use the same peer ID
	if err := os.WriteFile(path, seed, 0600); err != nil {
		return nil, fmt.Errorf("writing host key to disk: %w", err)
	}

	return privKey, nil
}

// getDefaultRelayAddresses returns the default public libp2p circuit relay v2 addresses.
// Uses regional relay hostnames that resolve via DNS (am6, ny5, sg1, sv15).
// Note: bootstrap.libp2p.io does not resolve on all networks.
func getDefaultRelayAddresses() []peer.AddrInfo {
	relayAddresses := []string{
		"/dnsaddr/am6.bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
		"/dnsaddr/ny5.bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
		"/dnsaddr/sg1.bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
		"/dnsaddr/sv15.bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	}

	var addrs []peer.AddrInfo
	for _, addrStr := range relayAddresses {
		addr, err := peer.AddrInfoFromString(addrStr)
		if err == nil {
			addrs = append(addrs, *addr)
		}
	}
	return addrs
}

// getDHTBootstrapPeers returns DHT bootstrap peers using regional hostnames.
// dht.GetDefaultBootstrapPeerAddrInfos() fails to resolve bootstrap.libp2p.io
// on some networks (Go's DNS resolver can't resolve dnsaddr TXT records).
// We use the regional hostnames which do resolve.
func getDHTBootstrapPeers() []peer.AddrInfo {
	bootstrapPeers := []string{
		"/ip4/54.38.47.166/tcp/4001/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
		"/ip4/51.81.93.51/tcp/4001/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
		"/ip4/15.235.144.210/tcp/4001/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
		"/ip4/147.135.44.132/tcp/4001/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
	}

	var peers []peer.AddrInfo
	for _, addrStr := range bootstrapPeers {
		addr, err := peer.AddrInfoFromString(addrStr)
		if err == nil {
			peers = append(peers, *addr)
		}
	}
	return peers
}

// discoveryValidator validates discovery records stored in the DHT.
// It validates that the value is a valid JSON-serialized peer.AddrInfo.
type discoveryValidator struct{}

// Validate validates a discovery record.
// For discovery records, we check that:
// 1. The key path starts with "discovery/"
// 2. The value is valid JSON
func (v *discoveryValidator) Validate(key string, value []byte) error {
	// Key format: /agent-memory/discovery/<hash>
	// After SplitKey: namespace="agent-memory", path="discovery/<hash>"
	_, path, err := libp2precord.SplitKey(key)
	if err != nil {
		return fmt.Errorf("invalid key format: %w", err)
	}
	// Check that the path starts with "discovery/"
	if len(path) < len("discovery/") || path[:len("discovery/")] != "discovery/" {
		return fmt.Errorf("key path does not match discovery format: %s", path)
	}
	if len(value) == 0 {
		return errors.New("discovery record value is empty")
	}
	// Try to parse as JSON to validate it's well-formed
	var info peer.AddrInfo
	if err := json.Unmarshal(value, &info); err != nil {
		return fmt.Errorf("discovery record is not valid JSON: %w", err)
	}
	return nil
}

// Select selects the best record from multiple records.
// For discovery, we just pick the first one (they should be identical).
func (v *discoveryValidator) Select(key string, values [][]byte) (int, error) {
	if len(values) == 0 {
		return 0, errors.New("no values to select from")
	}
	return 0, nil
}

// compile-time check that discoveryValidator implements libp2precord.Validator
var _ libp2precord.Validator = (*discoveryValidator)(nil)

// passthroughValidator is a validator that accepts any value for keys it doesn't care about.
// It implements the Validator interface.
type passthroughValidator struct{}

// Validate accepts any value (passthrough for keys we don't validate).
func (v *passthroughValidator) Validate(key string, value []byte) error {
	return nil
}

// Select picks the first value (no-op for passthrough).
func (v *passthroughValidator) Select(key string, values [][]byte) (int, error) {
	if len(values) == 0 {
		return 0, errors.New("no values to select from")
	}
	return 0, nil
}

// compile-time check that passthroughValidator implements libp2precord.Validator
var _ libp2precord.Validator = (*passthroughValidator)(nil)
