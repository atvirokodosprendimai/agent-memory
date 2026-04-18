// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

import (
	"context"
	"fmt"

	"github.com/ipfs/boxo/bitswap/client"
	bsnet "github.com/ipfs/boxo/bitswap/network/bsnet"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p/core/host"
)

// BitSwap handles block exchange with connected peers via bitswap.
type BitSwap interface {
	// GetBlock retrieves a block by CID from any connected peer.
	// Returns err if ctx is cancelled or block not found among any connected peer.
	GetBlock(ctx context.Context, c cid.Cid) (blocks.Block, error)
	// NotifyNewBlocks announces newly stored blocks to connected peers.
	NotifyNewBlocks(ctx context.Context, blks ...blocks.Block) error
	// GetSession returns the underlying bitswap client for advanced use.
	GetSession() *client.Client
}

// bitSwap implements the BitSwap interface using boxo/bitswap client.
type bitSwap struct {
	bs    *client.Client
	store blockstore.Blockstore
}

// NewBitSwap creates a BitSwap session wired to the given host and blockstore.
// It creates a bitswap client that can exchange blocks with connected peers.
func NewBitSwap(ctx context.Context, h host.Host, bs blockstore.Blockstore) (BitSwap, error) {
	if h == nil {
		return nil, fmt.Errorf("p2p: host cannot be nil")
	}
	if bs == nil {
		return nil, fmt.Errorf("p2p: blockstore cannot be nil")
	}

	// Create bitswap network from the libp2p host
	bsNetwork := bsnet.NewFromIpfsHost(h)

	// Create bitswap client with the network and blockstore
	// The nil providerFinder disables content routing lookups
	// since we rely on bitswap for discovery among connected peers
	bsClient := client.New(ctx, bsNetwork, nil, bs)

	return &bitSwap{
		bs:    bsClient,
		store: bs,
	}, nil
}

// GetBlock retrieves a block by CID.
// It checks the local blockstore first (fast path), and if not found,
// queries connected peers via bitswap.
func (b *bitSwap) GetBlock(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	// Check blockstore first (fast path for local blocks)
	has, err := b.store.Has(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("p2p: checking blockstore: %w", err)
	}
	if has {
		return b.store.Get(ctx, c)
	}

	// Not in local blockstore, query peers via bitswap
	return b.bs.GetBlock(ctx, c)
}

// NotifyNewBlocks announces newly stored blocks to connected peers.
// This triggers want-have messages to be broadcast to peers.
func (b *bitSwap) NotifyNewBlocks(ctx context.Context, blks ...blocks.Block) error {
	return b.bs.NotifyNewBlocks(ctx, blks...)
}

// GetSession returns the underlying bitswap client for advanced use cases.
// The client can be used to create sessions for batched block requests.
func (b *bitSwap) GetSession() *client.Client {
	return b.bs
}
