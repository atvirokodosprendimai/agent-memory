// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

import (
	"context"
	"fmt"
	"sync"
	"time"

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

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// FindPeers discovers peers advertising under the shared secret via DHT.
// It queries the DHT for the discovery key and returns their address info.
// This is exposed for diagnostics and testing.
func (c *P2PClient) FindPeers(ctx context.Context) ([]string, error) {
	peers, err := c.disc.FindPeers(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(peers))
	for i, p := range peers {
		ids[i] = p.ID.String()
	}
	return ids, nil
}

// ConnectPeers explicitly calls ConnectToPeers to try to connect to discovered peers.
// The background reconnect loop calls this periodically, but this can be called
// explicitly to force a reconnect attempt (e.g., before a Get operation).
func (c *P2PClient) ConnectPeers(ctx context.Context) error {
	return c.disc.ConnectToPeers(ctx)
}

// NewP2PClient creates a P2PClient: starts libp2p host, DHT, bitswap, and connects to peers.
func NewP2PClient(ctx context.Context, secret, dataDir string) (*P2PClient, error) {
	// Create a derived context for the client's lifecycle
	// Cancelled on Close()
	clientCtx, cancel := context.WithCancel(context.Background())

	// Create the host first
	h, err := NewHost(clientCtx, secret, dataDir)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("creating host: %w", err)
	}

	// Create discovery
	disc, err := NewDiscovery(h, secret)
	if err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("creating discovery: %w", err)
	}

	// Create blockstore
	bs, err := NewBadgerBlockstore(clientCtx, dataDir)
	if err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("creating blockstore: %w", err)
	}

	// Create bitswap
	bitSwap, err := NewBitSwap(clientCtx, h.Host(), bs)
	if err != nil {
		bs.Close()
		h.Close()
		cancel()
		return nil, fmt.Errorf("creating bitswap: %w", err)
	}

	client := &P2PClient{
		host:   h,
		disc:   disc,
		bs:     bitSwap,
		store:  bs,
		ctx:    clientCtx,
		cancel: cancel,
	}

	// Advertise to DHT with retries (DHT routing table may not be populated yet)
	var advertiseErr error
	for i := 0; i < 3; i++ {
		advertiseErr = disc.Advertise(clientCtx)
		if advertiseErr == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if advertiseErr != nil {
		// Non-fatal: client still works for local operations
		// DHT advertise will succeed once routing table is populated
	}

	// Connect to peers (initial attempt)
	if err := disc.ConnectToPeers(clientCtx); err != nil {
		// Non-fatal: peers may not be available yet
		// Background reconnect loop will keep trying
	}

	// Start background reconnect loop
	// This continuously tries to connect to peers as they become available via DHT.
	// Since both peers run this loop, they will eventually find each other.
	client.wg.Add(1)
	go func() {
		defer client.wg.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-clientCtx.Done():
				return
			case <-ticker.C:
				_ = disc.ConnectToPeers(clientCtx)
			}
		}
	}()

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
// Uses a 30-second timeout for the bitswap fetch.
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

	// Miss - fetch from peers via bitswap with timeout
	// If the client is closed (ctx cancelled) or no peers are connected,
	// the fetch will fail rather than block indefinitely.
	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	blk, err := c.bs.GetBlock(fetchCtx, cidObj)
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

	// Cancel the client's context to stop the background reconnect goroutine
	if c.cancel != nil {
		c.cancel()
	}

	// Wait for background goroutines to exit
	c.wg.Wait()

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
