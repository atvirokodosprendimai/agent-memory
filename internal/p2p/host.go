// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"golang.org/x/crypto/hkdf"
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

// NewHost creates a libp2p host backed by a peer ID derived from secret.
// It enables circuit relay v2 and auto-relay with public bootstrap relays.
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

	// Derive peer ID from secret using HKDF-SHA256
	privKey, err := deriveKeyFromSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("p2p: deriving key from secret: %w", err)
	}

	// Ensure data directory exists
	p2pDir := filepath.Join(dataDir, "p2p")
	if err := os.MkdirAll(p2pDir, 0700); err != nil {
		return nil, fmt.Errorf("p2p: creating p2p directory: %w", err)
	}

	// Try to load existing host key
	hostKeyPath := filepath.Join(p2pDir, "hostkey")
	privKey, err = loadOrCreateHostKey(hostKeyPath, privKey)
	if err != nil {
		return nil, fmt.Errorf("p2p: loading host key: %w", err)
	}

	// Derive peer ID from the actual key
	id, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("p2p: deriving peer ID from key: %w", err)
	}

	// Get public bootstrap relay addresses
	relayAddresses := getDefaultRelayAddresses()

	// Create libp2p host with no listen addresses (works from NAT)
	// Enable circuit relay v2 and auto-relay with public bootstrap relays
	h, err := libp2p.New(
		libp2p.Identity(privKey),
		libp2p.NoListenAddrs,
		libp2p.EnableRelay(),
		libp2p.EnableAutoRelay(
			autorelay.WithStaticRelays(relayAddresses),
			autorelay.WithNumRelays(2),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("p2p: creating libp2p host: %w", err)
	}

	// Create DHT client mode
	d, err := dht.New(ctx, h,
		dht.Mode(dht.ModeClient),
		dht.BootstrapPeers(dht.GetDefaultBootstrapPeerAddrInfos()...),
	)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("p2p: creating DHT client: %w", err)
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

// deriveKeyFromSecret derives an Ed25519 private key from the secret using HKDF-SHA256.
func deriveKeyFromSecret(secret string) (crypto.PrivKey, error) {
	salt := []byte("agent-memory-peer-id")
	info := []byte("v1")

	// HKDF to derive key material
	hkdfReader := hkdf.New(sha256.New, []byte(secret), salt, info)

	// Generate Ed25519 key pair from HKDF output
	_, privKeyBytes, err := ed25519.GenerateKey(hkdfReader)
	if err != nil {
		return nil, fmt.Errorf("ed25519 key generation: %w", err)
	}

	// Convert to libp2p crypto PrivKey
	privKey, err := crypto.UnmarshalEd25519PrivateKey(privKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling ed25519 private key: %w", err)
	}

	return privKey, nil
}

// loadOrCreateHostKey loads a host key from disk or creates a new one.
// If a key exists at path, it validates and returns it.
// If no key exists, stores the provided key and returns it.
func loadOrCreateHostKey(path string, privKey crypto.PrivKey) (crypto.PrivKey, error) {
	// Try to load existing key
	keyBytes, err := os.ReadFile(path)
	if err == nil && len(keyBytes) > 0 {
		privKey, err := crypto.UnmarshalEd25519PrivateKey(keyBytes)
		if err == nil {
			return privKey, nil
		}
		// If unmarshal fails, generate new key below
	}

	// No key exists or invalid - store the derived key
	privKeyBytes, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("marshaling private key: %w", err)
	}

	if err := os.WriteFile(path, privKeyBytes, 0600); err != nil {
		return nil, fmt.Errorf("writing host key to disk: %w", err)
	}

	return privKey, nil
}

// getDefaultRelayAddresses returns the default public libp2p circuit relay v2 addresses.
func getDefaultRelayAddresses() []peer.AddrInfo {
	// Use known public libp2p bootstrap relays
	// These are the official libp2p project public relays
	defaultRelays := []string{
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLGSQJfedviPkCMnpSWxNetLnk",
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
	}

	var addrs []peer.AddrInfo
	for _, addrStr := range defaultRelays {
		addr, err := peer.AddrInfoFromString(addrStr)
		if err == nil {
			addrs = append(addrs, *addr)
		}
	}
	return addrs
}
