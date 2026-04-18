// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

import "errors"

var ErrNotImplemented = errors.New("p2p: not implemented")

// Host provides the libp2p host and DHT to other P2P components.
type Host interface {
	// Host returns the underlying libp2p host.
	Host() interface{} // TODO: replace with host.Host when libp2p is imported
	// DHT returns the DHT client.
	DHT() interface{} // TODO: replace with *dht.IpfsDHT when imported
	// PeerID returns the peer ID of this host.
	PeerID() string
	// Close shuts down the host.
	Close() error
}

// NewHost creates a libp2p host backed by a peer ID derived from secret.
// It enables circuit relay v2 and auto-relay with public bootstrap relays.
func NewHost(ctx interface{}, secret, dataDir string) (Host, error) {
	return nil, ErrNotImplemented
}
