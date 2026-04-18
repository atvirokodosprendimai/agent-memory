// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

// ErrNotImplemented is used as a placeholder until the real implementation is added.

// StorageClient is the interface that both KuboClient and P2PClient implement.
// All store operations go through this interface.
type StorageClient interface {
	Add(data []byte) (string, error)
	Get(cid string) ([]byte, error)
	PinLs() (map[string]bool, error)
	PinRm(cid string) error
	Close() error
	Ping() error
	ID() (string, error)
}

// P2PClient is the P2P transport implementation of StorageClient.
type P2PClient struct {
	host Host
	disc Discovery
	bs   BitSwap
}

// NewP2PClient creates a P2PClient: starts libp2p host, DHT, bitswap, and connects to peers.
func NewP2PClient(ctx interface{}, secret, dataDir string) (*P2PClient, error) { // TODO: context.Context
	return nil, ErrNotImplemented
}

// Add implements StorageClient.
func (c *P2PClient) Add(data []byte) (string, error) {
	return "", ErrNotImplemented
}

// Get implements StorageClient.
func (c *P2PClient) Get(cid string) ([]byte, error) {
	return nil, ErrNotImplemented
}

// PinLs implements StorageClient.
func (c *P2PClient) PinLs() (map[string]bool, error) {
	return nil, ErrNotImplemented
}

// PinRm implements StorageClient.
func (c *P2PClient) PinRm(cid string) error {
	return ErrNotImplemented
}

// Close implements StorageClient.
func (c *P2PClient) Close() error {
	return ErrNotImplemented
}

// Ping implements StorageClient.
func (c *P2PClient) Ping() error {
	return ErrNotImplemented
}

// ID implements StorageClient.
func (c *P2PClient) ID() (string, error) {
	return "", ErrNotImplemented
}
