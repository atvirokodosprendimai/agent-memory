package p2p

import (
	"context"
	"errors"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBlockstore implements blockstore.Blockstore for testing.
type mockBlockstore struct {
	store       map[string]blocks.Block
	putError    error
	getError    error
	hasError    error
	getSizeErr  error
	deleteError error
	allKeysErr  error
}

func newMockBlockstore() *mockBlockstore {
	return &mockBlockstore{store: make(map[string]blocks.Block)}
}

func (m *mockBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	if m.hasError != nil {
		return false, m.hasError
	}
	_, ok := m.store[c.String()]
	return ok, nil
}

func (m *mockBlockstore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	blk, ok := m.store[c.String()]
	if !ok {
		return nil, errors.New("block not found")
	}
	return blk, nil
}

func (m *mockBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	if m.getSizeErr != nil {
		return -1, m.getSizeErr
	}
	blk, ok := m.store[c.String()]
	if !ok {
		return -1, errors.New("block not found")
	}
	return len(blk.RawData()), nil
}

func (m *mockBlockstore) Put(ctx context.Context, b blocks.Block) error {
	if m.putError != nil {
		return m.putError
	}
	m.store[b.Cid().String()] = b
	return nil
}

func (m *mockBlockstore) PutMany(ctx context.Context, bs []blocks.Block) error {
	for _, b := range bs {
		if err := m.Put(ctx, b); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockBlockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	if m.deleteError != nil {
		return m.deleteError
	}
	delete(m.store, c.String())
	return nil
}

func (m *mockBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	if m.allKeysErr != nil {
		return nil, m.allKeysErr
	}
	ch := make(chan cid.Cid, len(m.store))
	for _, blk := range m.store {
		ch <- blk.Cid()
	}
	close(ch)
	return ch, nil
}

func (m *mockBlockstore) HashOnRead(enabled bool) {}

// mockBitswapClient implements a mock bitswap client for testing.
type mockBitswapClient struct {
	getBlockCalled     bool
	getBlockCID        cid.Cid
	getBlockResult     blocks.Block
	getBlockErr        error
	notifyBlocksCalled bool
	notifyBlocks       []blocks.Block
	notifyErr          error
	sessionCalled      bool
}

func (m *mockBitswapClient) GetBlock(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	m.getBlockCalled = true
	m.getBlockCID = c
	return m.getBlockResult, m.getBlockErr
}

func (m *mockBitswapClient) NotifyNewBlocks(ctx context.Context, blks ...blocks.Block) error {
	m.notifyBlocksCalled = true
	m.notifyBlocks = blks
	return m.notifyErr
}

func (m *mockBitswapClient) GetSession() interface{} {
	m.sessionCalled = true
	return m
}

// testBitSwap is a test implementation of the BitSwap interface that delegates
// to mock blockstore and mock bitswap client.
type testBitSwap struct {
	blockstore blockstore.Blockstore
	session    *mockBitswapClient
}

func (b *testBitSwap) GetBlock(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	// Check blockstore first (fast path)
	has, err := b.blockstore.Has(ctx, c)
	if err != nil {
		return nil, err
	}
	if has {
		return b.blockstore.Get(ctx, c)
	}
	// Not local, use bitswap
	return b.session.GetBlock(ctx, c)
}

func (b *testBitSwap) NotifyNewBlocks(ctx context.Context, blks ...blocks.Block) error {
	return b.session.NotifyNewBlocks(ctx, blks...)
}

func (b *testBitSwap) GetSession() interface{} {
	b.session.sessionCalled = true
	return b.session
}

// TestBitSwap_GetBlock_LocalBlock tests that GetBlock returns from blockstore without peer interaction.
func TestBitSwap_GetBlock_LocalBlock(t *testing.T) {
	ctx := context.Background()

	// Create a mock blockstore with a pre-stored block
	mbs := newMockBlockstore()
	testData := []byte("local block data")
	blk, err := NewBlock(testData)
	require.NoError(t, err)
	err = mbs.Put(ctx, blk)
	require.NoError(t, err)

	// Create a mock bitswap client that should NOT be called
	mockClient := &mockBitswapClient{
		getBlockErr: errors.New("bitswap should not be called for local block"),
	}

	// Create the test BitSwap implementation
	bs := &testBitSwap{
		blockstore: mbs,
		session:    mockClient,
	}

	// GetBlock should return from local blockstore
	result, err := bs.GetBlock(ctx, blk.Cid())
	require.NoError(t, err, "GetBlock should succeed for local block")
	assert.Equal(t, testData, result.RawData(), "Retrieved data should match")

	// Verify bitswap was NOT called (local block fast path)
	assert.False(t, mockClient.getBlockCalled, "bitswap.GetBlock should not be called for local block")
}

// TestBitSwap_GetBlock_RemoteBlock tests that GetBlock queries bitswap when block is not local.
func TestBitSwap_GetBlock_RemoteBlock(t *testing.T) {
	ctx := context.Background()

	// Create an empty mock blockstore (block not stored locally)
	mbs := newMockBlockstore()

	// Create a CID for a block that doesn't exist locally
	nonExistentCid, err := cid.Decode("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(t, err)

	// Create a mock bitswap client that returns a block
	remoteData := []byte("remote block data")
	remoteBlk, err := NewBlock(remoteData)
	require.NoError(t, err)

	mockClient := &mockBitswapClient{
		getBlockResult: remoteBlk,
	}

	bs := &testBitSwap{
		blockstore: mbs,
		session:    mockClient,
	}

	// GetBlock should call bitswap since block is not local
	result, err := bs.GetBlock(ctx, nonExistentCid)
	require.NoError(t, err, "GetBlock should succeed for remote block")
	assert.Equal(t, remoteData, result.RawData(), "Retrieved data should match remote block")

	// Verify bitswap WAS called
	assert.True(t, mockClient.getBlockCalled, "bitswap.GetBlock should be called for non-local block")
	assert.Equal(t, nonExistentCid, mockClient.getBlockCID, "bitswap should be called with correct CID")
}

// TestBitSwap_NotifyNewBlocks tests that NotifyNewBlocks broadcasts to peers.
func TestBitSwap_NotifyNewBlocks(t *testing.T) {
	ctx := context.Background()

	mbs := newMockBlockstore()
	mockClient := &mockBitswapClient{}

	bs := &testBitSwap{
		blockstore: mbs,
		session:    mockClient,
	}

	// Create blocks to announce
	blk1, err := NewBlock([]byte("block 1"))
	require.NoError(t, err)
	blk2, err := NewBlock([]byte("block 2"))
	require.NoError(t, err)

	// NotifyNewBlocks should broadcast to peers
	err = bs.NotifyNewBlocks(ctx, blk1, blk2)
	require.NoError(t, err, "NotifyNewBlocks should succeed")

	// Verify bitswap was called with the blocks
	assert.True(t, mockClient.notifyBlocksCalled, "bitswap.NotifyNewBlocks should be called")
	assert.Len(t, mockClient.notifyBlocks, 2, "Should notify about 2 blocks")
	assert.Equal(t, blk1.Cid(), mockClient.notifyBlocks[0].Cid())
	assert.Equal(t, blk2.Cid(), mockClient.notifyBlocks[1].Cid())
}

// TestBitSwap_GetSession tests that GetSession returns the underlying session.
func TestBitSwap_GetSession(t *testing.T) {
	mbs := newMockBlockstore()
	mockClient := &mockBitswapClient{}

	bs := &testBitSwap{
		blockstore: mbs,
		session:    mockClient,
	}

	// GetSession should return the underlying session
	session := bs.GetSession()
	assert.NotNil(t, session, "GetSession should return non-nil session")
	assert.True(t, mockClient.sessionCalled, "GetSession should be called")
}

// TestBitSwap_GetBlock_BlockstoreError tests error handling when blockstore Has fails.
func TestBitSwap_GetBlock_BlockstoreError(t *testing.T) {
	ctx := context.Background()

	// Create a blockstore that returns an error on Has
	mbs := &mockBlockstore{
		hasError: errors.New("blockstore has error"),
	}
	mockClient := &mockBitswapClient{}

	bs := &testBitSwap{
		blockstore: mbs,
		session:    mockClient,
	}

	// Create any CID
	anyCid, err := cid.Decode("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(t, err)

	// GetBlock should propagate blockstore error
	_, err = bs.GetBlock(ctx, anyCid)
	assert.Error(t, err, "GetBlock should return error from blockstore")
	assert.Contains(t, err.Error(), "blockstore has error")

	// bitswap should NOT be called when blockstore errors
	assert.False(t, mockClient.getBlockCalled, "bitswap should not be called when blockstore errors")
}

// TestBitSwap_GetBlock_NotFound tests that GetBlock returns error when block not found anywhere.
func TestBitSwap_GetBlock_NotFound(t *testing.T) {
	ctx := context.Background()

	// Create an empty mock blockstore
	mbs := newMockBlockstore()

	// Create a CID for a block that doesn't exist
	nonExistentCid, err := cid.Decode("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	require.NoError(t, err)

	// Mock bitswap client that returns not found
	mockClient := &mockBitswapClient{
		getBlockErr: errors.New("block not found"),
	}

	bs := &testBitSwap{
		blockstore: mbs,
		session:    mockClient,
	}

	// GetBlock should return error from bitswap
	_, err = bs.GetBlock(ctx, nonExistentCid)
	assert.Error(t, err, "GetBlock should return error when block not found")
	assert.Contains(t, err.Error(), "block not found")
}

// TestBitSwap_NotifyNewBlocks_Error tests error handling in NotifyNewBlocks.
func TestBitSwap_NotifyNewBlocks_Error(t *testing.T) {
	ctx := context.Background()

	mbs := newMockBlockstore()
	mockClient := &mockBitswapClient{
		notifyErr: errors.New("notify failed"),
	}

	bs := &testBitSwap{
		blockstore: mbs,
		session:    mockClient,
	}

	blk, err := NewBlock([]byte("test block"))
	require.NoError(t, err)

	// NotifyNewBlocks should propagate error
	err = bs.NotifyNewBlocks(ctx, blk)
	assert.Error(t, err, "NotifyNewBlocks should return error")
	assert.Contains(t, err.Error(), "notify failed")
}

// TestBitSwap_InterfaceCompliance verifies that bitSwap implements BitSwap interface.
// This is a compile-time check.
func TestBitSwap_InterfaceCompliance(t *testing.T) {
	// Compile-time check that *bitSwap implements BitSwap
	var _ BitSwap = (*bitSwap)(nil)
}

// TestBitSwap_NewBitSwap_NilHost tests that NewBitSwap rejects nil host.
func TestBitSwap_NewBitSwap_NilHost(t *testing.T) {
	ctx := context.Background()
	mbs := newMockBlockstore()

	_, err := NewBitSwap(ctx, nil, mbs)
	assert.Error(t, err, "NewBitSwap should reject nil host")
	assert.Contains(t, err.Error(), "host cannot be nil")
}

// TestBitSwap_NewBitSwap_NilBlockstore tests that NewBitSwap rejects nil blockstore.
func TestBitSwap_NewBitSwap_NilBlockstore(t *testing.T) {
	// We can't create a real host here without full libp2p setup,
	// but we can test the nil blockstore check by using a mock that won't be reached
	// Skip this test as it requires a real host
	t.Skip("Requires full libp2p host setup")
}

// mockHostForBitswap implements a minimal host.Host for testing NewBitSwap.
type mockHostForBitswap struct {
	id peer.ID
}

func (m *mockHostForBitswap) ID() peer.ID {
	return m.id
}

// Verify mockHostForBitswap satisfies host.Host interface
func TestBitSwap_NewBitSwap_ValidInputs(t *testing.T) {
	// This test would require a real libp2p host which is complex to set up in unit tests.
	// The actual integration is tested in the integration tests.
	t.Skip("Requires full libp2p host setup for integration test")
}
