// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

// ErrNotImplemented is used as a placeholder until the real implementation is added.

// Discovery handles DHT-based peer discovery keyed by shared secret.
type Discovery interface {
	// Advertise announces this peer's address info to the DHT under the secret key.
	Advertise(ctx interface{}) error
	// FindPeers returns AddrInfo for all peers advertising under the shared secret.
	FindPeers(ctx interface{}) ([]interface{}, error) // TODO: replace with []peer.AddrInfo
	// ConnectToPeers finds and connects to all discovered peers via circuit relay.
	ConnectToPeers(ctx interface{}) error
}

// NewDiscovery creates a Discovery instance bound to the given Host and secret.
func NewDiscovery(h Host, secret string) (Discovery, error) {
	return nil, ErrNotImplemented
}
