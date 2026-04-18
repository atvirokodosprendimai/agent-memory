// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ds-badger"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	mh "github.com/multiformats/go-multihash"
)

// NewBlock creates a block with CIDv1 using sha2-256 and raw multicodec,
// matching the CID format used by Kubo. This ensures interoperability
// between P2PClient and Kubo clients.
func NewBlock(data []byte) (*blocks.BasicBlock, error) {
	// Hash the data using sha2-256
	h, err := mh.Sum(data, mh.SHA2_256, 32)
	if err != nil {
		return nil, fmt.Errorf("computing sha2-256 hash: %w", err)
	}
	// Create CIDv1 with raw multicodec (0x55)
	c := cid.NewCidV1(cid.Raw, h)
	return blocks.NewBlockWithCid(data, c)
}

// badgerBlockstore wraps a boxo/go-ipfs-blockstore Blockstore with a BadgerDS datastore
// to provide a Close method for cleanup.
type badgerBlockstore struct {
	blockstore.Blockstore
	ds *badger.Datastore
}

// Close closes the underlying BadgerDS datastore.
func (b *badgerBlockstore) Close() error {
	return b.ds.Close()
}

// NewBadgerBlockstore creates a blockstore backed by go-ds-badger.
// It creates a BadgerDS datastore at dataDir/p2p/badgerds/ and wraps it
// with the blockstore interface using blockstore.NoPrefix() to ensure
// CID format matches Kubo (sha2-256, raw multicodec).
func NewBadgerBlockstore(ctx context.Context, dataDir string) (*badgerBlockstore, error) {
	// Create the BadgerDS directory
	badgerPath := filepath.Join(dataDir, "p2p", "badgerds")
	if err := os.MkdirAll(badgerPath, 0755); err != nil {
		return nil, fmt.Errorf("creating badgerds directory: %w", err)
	}

	// Create BadgerDS datastore with default options
	opts := badger.DefaultOptions
	ds, err := badger.NewDatastore(badgerPath, &opts)
	if err != nil {
		return nil, fmt.Errorf("creating badger datastore: %w", err)
	}

	// Wrap with boxo blockstore using NoPrefix to match Kubo CID format
	bs := blockstore.NewBlockstoreNoPrefix(ds)

	// Enable hash verification on read to ensure data integrity
	bs.HashOnRead(true)

	return &badgerBlockstore{
		Blockstore: bs,
		ds:         ds,
	}, nil
}
