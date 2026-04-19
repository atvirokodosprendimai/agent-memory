// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

import (
	"context"
	"fmt"
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
	bs    BitSwap
	store blockstore.Blockstore

	ctx    context.Context
	cancel context.CancelFunc
}

// NewP2PClient creates a P2PClient: starts libp2p host with DHT and bitswap.
func NewP2PClient(ctx context.Context, dataDir string) (*P2PClient, error) {
	clientCtx, cancel := context.WithCancel(context.Background())

	h, err := NewHost(clientCtx, dataDir)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("creating host: %w", err)
	}

	bs, err := NewBadgerBlockstore(clientCtx, dataDir)
	if err != nil {
		h.Close()
		cancel()
		return nil, fmt.Errorf("creating blockstore: %w", err)
	}

	bitSwap, err := NewBitSwap(clientCtx, h.Host(), bs)
	if err != nil {
		bs.Close()
		h.Close()
		cancel()
		return nil, fmt.Errorf("creating bitswap: %w", err)
	}

	client := &P2PClient{
		host:   h,
		bs:     bitSwap,
		store:  bs,
		ctx:    clientCtx,
		cancel: cancel,
	}

	return client, nil
}

// Add implements StorageClient.
// It stores the block locally and announces it to bitswap peers via the public IPFS network.
func (c *P2PClient) Add(data []byte) (string, error) {
	blk, err := NewBlock(data)
	if err != nil {
		return "", fmt.Errorf("creating block: %w", err)
	}

	if err := c.store.Put(context.Background(), blk); err != nil {
		return "", fmt.Errorf("putting block to store: %w", err)
	}

	if err := c.bs.NotifyNewBlocks(context.Background(), blk); err != nil {
	}

	return blk.Cid().String(), nil
}

// Get implements StorageClient.
// It checks local blockstore first; on miss, fetches via bitswap from the public IPFS network.
func (c *P2PClient) Get(cidStr string) ([]byte, error) {
	ctx := context.Background()

	cidObj, err := cid.Decode(cidStr)
	if err != nil {
		return nil, fmt.Errorf("decoding CID: %w", err)
	}

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

	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	blk, err := c.bs.GetBlock(fetchCtx, cidObj)
	if err != nil {
		return nil, fmt.Errorf("getting block from bitswap: %w", err)
	}

	return blk.RawData(), nil
}

// PinLs implements StorageClient.
func (c *P2PClient) PinLs() (map[string]bool, error) {
	ctx := context.Background()
	result := make(map[string]bool)

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
func (c *P2PClient) PinRm(cidStr string) error {
	ctx := context.Background()

	cidObj, err := cid.Decode(cidStr)
	if err != nil {
		return fmt.Errorf("decoding CID: %w", err)
	}

	if err := c.store.DeleteBlock(ctx, cidObj); err != nil {
		return fmt.Errorf("deleting block: %w", err)
	}

	return nil
}

// Close implements StorageClient.
func (c *P2PClient) Close() error {
	var errs []error

	if c.cancel != nil {
		c.cancel()
	}

	if c.store != nil {
		if closer, ok := c.store.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, fmt.Errorf("closing blockstore: %w", err))
			}
		}
	}

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
func (c *P2PClient) ID() (string, error) {
	if c.host == nil {
		return "", fmt.Errorf("host is nil")
	}
	return c.host.PeerID().String(), nil
}

// compile-time interface compliance check
var _ StorageClient = (*P2PClient)(nil)
