// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
)

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
	host  Host
	disc  Discovery
	bs    BitSwap
	store blockstore.Blockstore
}

// NewP2PClient creates a P2PClient: starts libp2p host, DHT, bitswap, and connects to peers.
func NewP2PClient(ctx context.Context, secret, dataDir string) (*P2PClient, error) {
	// Create the host first
	h, err := NewHost(ctx, secret, dataDir)
	if err != nil {
		return nil, fmt.Errorf("creating host: %w", err)
	}

	// Create discovery
	disc, err := NewDiscovery(h, secret)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("creating discovery: %w", err)
	}

	// Create blockstore
	bs, err := NewBadgerBlockstore(ctx, dataDir)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("creating blockstore: %w", err)
	}

	// Create bitswap
	bitSwap, err := NewBitSwap(ctx, h.Host(), bs)
	if err != nil {
		bs.Close()
		h.Close()
		return nil, fmt.Errorf("creating bitswap: %w", err)
	}

	client := &P2PClient{
		host:  h,
		disc:  disc,
		bs:    bitSwap,
		store: bs,
	}

	// Advertise to DHT
	if err := disc.Advertise(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("advertising to DHT: %w", err)
	}

	// Connect to peers
	if err := disc.ConnectToPeers(ctx); err != nil {
		// Non-fatal: peers may not be available yet
		// Log but don't fail - other peers can connect to us
	}

	return client, nil
}

// Add implements StorageClient.
// It stores the block locally, notifies bitswap peers, and re-advertises to DHT.
func (c *P2PClient) Add(data []byte) (string, error) {
	// Create block with CIDv1 (sha2-256, raw) to match Kubo format
	blk, err := NewBlock(data)
	if err != nil {
		return "", fmt.Errorf("creating block: %w", err)
	}

	// Store in local blockstore
	if err := c.store.Put(context.Background(), blk); err != nil {
		return "", fmt.Errorf("putting block to store: %w", err)
	}

	// Notify bitswap peers about the new block
	if err := c.bs.NotifyNewBlocks(context.Background(), blk); err != nil {
		// Log but don't fail - block is stored locally
	}

	// Re-advertise to DHT with updated content
	if err := c.disc.Advertise(context.Background()); err != nil {
		// Log but don't fail
	}

	return blk.Cid().String(), nil
}

// Get implements StorageClient.
// It checks local blockstore first; on miss, fetches via bitswap from peers.
func (c *P2PClient) Get(cidStr string) ([]byte, error) {
	ctx := context.Background()

	// Parse the CID
	cidObj, err := cid.Decode(cidStr)
	if err != nil {
		return nil, fmt.Errorf("decoding CID: %w", err)
	}

	// Check local blockstore first (fast path)
	has, err := c.store.Has(ctx, cidObj)
	if err != nil {
		return nil, fmt.Errorf("checking blockstore: %w", err)
	}
	if has {
		blk, err := c.store.Get(ctx, cidObj)
		if err != nil {
			return nil, fmt.Errorf("getting block from store: %w", err)
		}
		return blk.RawData(), nil
	}

	// Miss - fetch from peers via bitswap
	blk, err := c.bs.GetBlock(ctx, cidObj)
	if err != nil {
		return nil, fmt.Errorf("getting block from bitswap: %w", err)
	}

	return blk.RawData(), nil
}

// PinLs implements StorageClient.
// It lists all CIDs stored in the local blockstore.
func (c *P2PClient) PinLs() (map[string]bool, error) {
	ctx := context.Background()
	result := make(map[string]bool)

	// Get all keys from blockstore
	keysCh, err := c.store.AllKeysChan(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting all keys: %w", err)
	}

	for keyCID := range keysCh {
		result[keyCID.String()] = true
	}

	return result, nil
}

// PinRm implements StorageClient.
// It removes the block from the local blockstore.
func (c *P2PClient) PinRm(cidStr string) error {
	ctx := context.Background()

	// Parse the CID
	cidObj, err := cid.Decode(cidStr)
	if err != nil {
		return fmt.Errorf("decoding CID: %w", err)
	}

	// Delete from blockstore
	if err := c.store.DeleteBlock(ctx, cidObj); err != nil {
		return fmt.Errorf("deleting block: %w", err)
	}

	return nil
}

// Close implements StorageClient.
// It shuts down all P2P components.
func (c *P2PClient) Close() error {
	var errs []error

	// Close blockstore
	if c.store != nil {
		if closer, ok := c.store.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("closing blockstore: %w", err))
			}
		}
	}

	// Close host (which closes DHT and connections)
	if c.host != nil {
		if err := c.host.Close(); err != nil {
			errs = append(errs, fmt.Errorf("closing host: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during close: %v", errs)
	}
	return nil
}

// Ping implements StorageClient.
// It checks if the host is running by verifying the peer ID is non-empty.
func (c *P2PClient) Ping() error {
	if c.host == nil {
		return fmt.Errorf("host is nil")
	}
	peerID := c.host.PeerID()
	if peerID == "" {
		return fmt.Errorf("host is not running (empty peer ID)")
	}
	return nil
}

// ID implements StorageClient.
// It returns the libp2p peer ID of this node.
func (c *P2PClient) ID() (string, error) {
	if c.host == nil {
		return "", fmt.Errorf("host is nil")
	}
	return c.host.PeerID().String(), nil
}

// compile-time interface compliance check
var _ StorageClient = (*P2PClient)(nil)
