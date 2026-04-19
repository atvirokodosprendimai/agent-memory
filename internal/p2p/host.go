// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
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
)

// ErrNotImplemented is returned when a function is not yet implemented.
var ErrNotImplemented = errors.New("p2p: not implemented")

// Host provides the libp2p host and DHT to other P2P components.
type Host interface {
	// Host returns the underlying libp2p host.
	Host() host.Host
	// DHT returns the DHT client (nil if DHT is disabled).
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

// NewHost creates a libp2p host connected to the public IPFS DHT network.
// The host key persists in dataDir/p2p/hostkey across restarts.
func NewHost(ctx context.Context, dataDir string) (Host, error) {
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

	relayAddresses := getDefaultRelayAddresses()

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

	d, err := dht.New(ctx, h,
		dht.Mode(dht.ModeClient),
		dht.BootstrapPeers(dht.GetDefaultBootstrapPeerAddrInfos()...),
	)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("p2p: creating DHT client: %w", err)
	}

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
func loadOrCreateHostKey(path string) (crypto.PrivKey, error) {
	keyBytes, err := os.ReadFile(path)
	if err == nil && len(keyBytes) == ed25519.PrivateKeySize {
		privKey, err := crypto.UnmarshalEd25519PrivateKey(keyBytes)
		if err == nil {
			return privKey, nil
		}
	}

	_, seed, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating ed25519 key: %w", err)
	}

	privKey, err := crypto.UnmarshalEd25519PrivateKey(seed)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling ed25519 private key: %w", err)
	}

	if err := os.WriteFile(path, seed, 0600); err != nil {
		return nil, fmt.Errorf("writing host key to disk: %w", err)
	}

	return privKey, nil
}

// getDefaultRelayAddresses returns the default public libp2p circuit relay v2 addresses.
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
