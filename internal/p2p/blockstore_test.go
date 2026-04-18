package p2p

import (
	"context"
	"testing"

	"github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBlockstore_PutGetRoundtrip tests that Put + Get returns identical bytes.
func TestBlockstore_PutGetRoundtrip(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bs, err := NewBadgerBlockstore(ctx, tmpDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, bs.Close())
	}()

	// Create a test block with known content using NewBlock (CIDv1, raw)
	testData := []byte("hello world this is a test block")
	blk, err := NewBlock(testData)
	require.NoError(t, err)

	// Put the block
	err = bs.Put(ctx, blk)
	require.NoError(t, err, "Put should succeed")

	// Get the block back
	retrieved, err := bs.Get(ctx, blk.Cid())
	require.NoError(t, err, "Get should succeed")

	// Verify the data is identical
	assert.Equal(t, testData, retrieved.RawData(), "Retrieved data should match original")
}

// TestBlockstore_AllKeysChan tests that AllKeysChan lists all stored CIDs.
func TestBlockstore_AllKeysChan(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bs, err := NewBadgerBlockstore(ctx, tmpDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, bs.Close())
	}()

	// Store multiple blocks using NewBlock to create CIDv1 (raw) blocks
	testBlocks := [][]byte{
		[]byte("block one"),
		[]byte("block two"),
		[]byte("block three"),
	}

	cids := make([]cid.Cid, len(testBlocks))
	for i, data := range testBlocks {
		blk, err := NewBlock(data)
		require.NoError(t, err)
		err = bs.Put(ctx, blk)
		require.NoError(t, err)
		cids[i] = blk.Cid()
	}

	// Collect all CIDs via AllKeysChan
	keysCh, err := bs.AllKeysChan(ctx)
	require.NoError(t, err, "AllKeysChan should succeed")

	var storedCIDs []cid.Cid
	for c := range keysCh {
		storedCIDs = append(storedCIDs, c)
	}

	// Verify all stored CIDs are returned
	// Note: BadgerDS/AllKeysChan may return duplicates, so we use >= instead of ==
	assert.GreaterOrEqual(t, len(storedCIDs), len(testBlocks), "AllKeysChan should return at least the number of stored CIDs")

	// Verify each CID is present (using a map to track since AllKeysChan may return duplicates)
	found := make(map[string]bool)
	for _, storedCid := range storedCIDs {
		found[storedCid.String()] = true
	}
	for _, expectedCid := range cids {
		assert.True(t, found[expectedCid.String()], "AllKeysChan should include CID %v", expectedCid)
	}
}

// TestBlockstore_Has tests that Has returns true for stored blocks.
func TestBlockstore_Has(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bs, err := NewBadgerBlockstore(ctx, tmpDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, bs.Close())
	}()

	testData := []byte("test data for has")
	blk, err := NewBlock(testData)
	require.NoError(t, err)

	// Should not exist before Put
	has, err := bs.Has(ctx, blk.Cid())
	require.NoError(t, err)
	assert.False(t, has, "Has should return false before Put")

	// Put the block
	err = bs.Put(ctx, blk)
	require.NoError(t, err)

	// Should exist after Put
	has, err = bs.Has(ctx, blk.Cid())
	require.NoError(t, err)
	assert.True(t, has, "Has should return true after Put")
}

// TestBlockstore_GetSize tests that GetSize returns correct size.
func TestBlockstore_GetSize(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bs, err := NewBadgerBlockstore(ctx, tmpDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, bs.Close())
	}()

	testData := []byte("test data size")
	blk := blocks.NewBlock(testData)

	err = bs.Put(ctx, blk)
	require.NoError(t, err)

	size, err := bs.GetSize(ctx, blk.Cid())
	require.NoError(t, err)
	assert.Equal(t, len(testData), size, "GetSize should return correct size")
}

// TestBlockstore_DeleteBlock tests that DeleteBlock removes a block.
func TestBlockstore_DeleteBlock(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bs, err := NewBadgerBlockstore(ctx, tmpDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, bs.Close())
	}()

	testData := []byte("block to delete")
	blk := blocks.NewBlock(testData)

	// Put the block
	err = bs.Put(ctx, blk)
	require.NoError(t, err)

	// Verify it exists
	has, err := bs.Has(ctx, blk.Cid())
	require.NoError(t, err)
	assert.True(t, has, "Block should exist before Delete")

	// Delete the block
	err = bs.DeleteBlock(ctx, blk.Cid())
	require.NoError(t, err, "DeleteBlock should succeed")

	// Verify it's gone
	has, err = bs.Has(ctx, blk.Cid())
	require.NoError(t, err)
	assert.False(t, has, "Block should not exist after Delete")
}

// TestBlockstore_PutMany tests PutMany stores multiple blocks.
func TestBlockstore_PutMany(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bs, err := NewBadgerBlockstore(ctx, tmpDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, bs.Close())
	}()

	blks := []blocks.Block{
		blocks.NewBlock([]byte("first block")),
		blocks.NewBlock([]byte("second block")),
		blocks.NewBlock([]byte("third block")),
	}

	err = bs.PutMany(ctx, blks)
	require.NoError(t, err, "PutMany should succeed")

	// Verify all blocks are retrievable
	for _, blk := range blks {
		has, err := bs.Has(ctx, blk.Cid())
		require.NoError(t, err)
		assert.True(t, has, "PutMany should store all blocks")
	}
}

// TestBlockstore_InterfaceCompliance verifies the returned type satisfies blockstore.Blockstore.
func TestBlockstore_InterfaceCompliance(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bs, err := NewBadgerBlockstore(ctx, tmpDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, bs.Close())
	}()

	// Verify it satisfies blockstore.Blockstore interface
	var _ blockstore.Blockstore = bs
}

// TestBlockstore_CIDFormatKuboCompatible tests that CID format matches Kubo output.
// Kubo uses CIDv1 with sha2-256 multihash (0x12) and raw multicodec (0x55).
// The NewBlock helper creates CIDv1 (raw) to match Kubo's format.
func TestBlockstore_CIDFormatKuboCompatible(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bs, err := NewBadgerBlockstore(ctx, tmpDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, bs.Close())
	}()

	// Create a block with CIDv1 (raw multicodec) to match Kubo format
	testData := []byte("test data for cid format")
	blk, err := NewBlock(testData)
	require.NoError(t, err)

	err = bs.Put(ctx, blk)
	require.NoError(t, err)

	c := blk.Cid()
	mh := c.Hash()

	// Verify the multihash is sha2-256 (0x12 is the multihash code for sha2-256)
	assert.Equal(t, uint8(0x12), mh[0], "First byte of multihash should be sha2-256 code (0x12)")

	// Verify the multihash hash length is 32 bytes (sha2-256 produces 32-byte hash)
	// Note: multihash encoding adds 2 bytes (code + length prefix), so total is 34 bytes
	assert.Equal(t, uint8(32), mh[1], "Second byte of multihash should be hash length (32)")

	// Verify CID is version 1
	assert.Equal(t, 1, int(c.Version()), "CID should be version 1")

	// Verify CID uses raw multicodec (0x55)
	assert.Equal(t, uint64(0x55), c.Type(), "CID should use raw multicodec (0x55)")

	t.Logf("CID: %v, multihash: %x, version: %d, codec: 0x%x", c, mh, c.Version(), c.Type())
}

// TestBlockstore_Persistence tests that blocks persist across blockstore restarts.
func TestBlockstore_Persistence(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	testData := []byte("persistent data across restarts")
	blk := blocks.NewBlock(testData)

	// First instance: store the block
	{
		bs1, err := NewBadgerBlockstore(ctx, tmpDir)
		require.NoError(t, err)

		err = bs1.Put(ctx, blk)
		require.NoError(t, err)
		require.NoError(t, bs1.Close())
	}

	// Second instance: retrieve the block
	{
		bs2, err := NewBadgerBlockstore(ctx, tmpDir)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, bs2.Close())
		}()

		has, err := bs2.Has(ctx, blk.Cid())
		require.NoError(t, err)
		assert.True(t, has, "Block should persist across restarts")

		retrieved, err := bs2.Get(ctx, blk.Cid())
		require.NoError(t, err)
		assert.Equal(t, testData, retrieved.RawData(), "Retrieved data should match")
	}
}

// TestBlockstore_NonExistentBlock tests that Get returns error for non-existent CIDs.
func TestBlockstore_NonExistentBlock(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bs, err := NewBadgerBlockstore(ctx, tmpDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, bs.Close())
	}()

	// Use a valid CID that definitely doesn't exist in our blockstore
	// This is a randomly generated CID with sha2-256 hash
	nonExistentCid, err := cid.Decode("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(t, err)
	_, err = bs.Get(ctx, nonExistentCid)
	assert.Error(t, err, "Get should return error for non-existent CID")
}

// TestBlockstore_DoublePut tests that Put of same block is idempotent.
func TestBlockstore_DoublePut(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bs, err := NewBadgerBlockstore(ctx, tmpDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, bs.Close())
	}()

	blk := blocks.NewBlock([]byte("idempotent test"))

	// Put twice
	err = bs.Put(ctx, blk)
	require.NoError(t, err)

	err = bs.Put(ctx, blk)
	// Idempotent - should succeed or be a no-op
	t.Logf("Second Put result: %v", err)

	// Should still be able to get it
	has, err := bs.Has(ctx, blk.Cid())
	require.NoError(t, err)
	assert.True(t, has, "Block should exist after double Put")
}
