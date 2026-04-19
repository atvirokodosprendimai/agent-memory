package p2p

import (
	"context"
	"errors"
	"testing"

	"github.com/ipfs/boxo/bitswap/client"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockP2PHost struct {
	idVal   peer.ID
	dhtVal  *dht.IpfsDHT
	hostVal host.Host
	closeFn func() error
}

func (m *mockP2PHost) ID() peer.ID       { return m.idVal }
func (m *mockP2PHost) Host() host.Host   { return m.hostVal }
func (m *mockP2PHost) DHT() *dht.IpfsDHT { return m.dhtVal }
func (m *mockP2PHost) PeerID() peer.ID   { return m.idVal }
func (m *mockP2PHost) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

type mockP2PBitSwap struct {
	getBlockFn        func(ctx context.Context, c cid.Cid) (blocks.Block, error)
	notifyNewBlocksFn func(ctx context.Context, blks ...blocks.Block) error
}

func (m *mockP2PBitSwap) GetBlock(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	if m.getBlockFn != nil {
		return m.getBlockFn(ctx, c)
	}
	return nil, errors.New("not implemented")
}
func (m *mockP2PBitSwap) NotifyNewBlocks(ctx context.Context, blks ...blocks.Block) error {
	if m.notifyNewBlocksFn != nil {
		return m.notifyNewBlocksFn(ctx, blks...)
	}
	return nil
}
func (m *mockP2PBitSwap) GetSession() *client.Client { return nil }

type mockP2PBlockstore struct {
	hasFn     func(ctx context.Context, c cid.Cid) (bool, error)
	getFn     func(ctx context.Context, c cid.Cid) (blocks.Block, error)
	putFn     func(ctx context.Context, b blocks.Block) error
	deleteFn  func(ctx context.Context, c cid.Cid) error
	allKeysFn func(ctx context.Context) (<-chan cid.Cid, error)
	getSizeFn func(ctx context.Context, c cid.Cid) (int, error)
}

func (m *mockP2PBlockstore) Has(ctx context.Context, c cid.Cid) (bool, error) {
	if m.hasFn != nil {
		return m.hasFn(ctx, c)
	}
	return false, nil
}
func (m *mockP2PBlockstore) Get(ctx context.Context, c cid.Cid) (blocks.Block, error) {
	if m.getFn != nil {
		return m.getFn(ctx, c)
	}
	return nil, errors.New("not implemented")
}
func (m *mockP2PBlockstore) Put(ctx context.Context, b blocks.Block) error {
	if m.putFn != nil {
		return m.putFn(ctx, b)
	}
	return nil
}
func (m *mockP2PBlockstore) PutMany(ctx context.Context, b []blocks.Block) error { return nil }
func (m *mockP2PBlockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, c)
	}
	return nil
}
func (m *mockP2PBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	if m.allKeysFn != nil {
		return m.allKeysFn(ctx)
	}
	ch := make(chan cid.Cid)
	close(ch)
	return ch, nil
}
func (m *mockP2PBlockstore) HashOnRead(enabled bool) {}
func (m *mockP2PBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	if m.getSizeFn != nil {
		return m.getSizeFn(ctx, c)
	}
	return 0, nil
}

var _ blockstore.Blockstore = (*mockP2PBlockstore)(nil)
var _ StorageClient = (*P2PClient)(nil)

func TestP2PClient_StorageClientInterface(t *testing.T) {
	var _ StorageClient = (*P2PClient)(nil)
}

func TestP2PClient_Add_PutAndNotifyBitswap(t *testing.T) {
	var putBlock blocks.Block
	var notifyCalled bool

	mockedBS := &mockP2PBlockstore{
		putFn: func(ctx context.Context, b blocks.Block) error {
			putBlock = b
			return nil
		},
	}

	mockedHost := &mockP2PHost{idVal: peer.ID("testpeer")}
	mockedBitSwap := &mockP2PBitSwap{
		notifyNewBlocksFn: func(ctx context.Context, blks ...blocks.Block) error {
			notifyCalled = true
			return nil
		},
	}

	c := &P2PClient{
		host:  mockedHost,
		bs:    mockedBitSwap,
		store: mockedBS,
	}

	testData := []byte("test data for add")
	cidStr, err := c.Add(testData)

	require.NoError(t, err, "Add should succeed")
	assert.NotEmpty(t, cidStr, "Add should return a CID")
	assert.NotNil(t, putBlock, "Put should have been called on blockstore")
	assert.Equal(t, testData, putBlock.RawData(), "Block data should match")
	assert.True(t, notifyCalled, "bitswap.NotifyNewBlocks should have been called")
}

func TestP2PClient_Get_BlockstoreHit(t *testing.T) {
	testData := []byte("data only in blockstore")
	blk, err := NewBlock(testData)
	require.NoError(t, err)

	mockedBS := &mockP2PBlockstore{
		hasFn: func(ctx context.Context, c cid.Cid) (bool, error) {
			return c.String() == blk.Cid().String(), nil
		},
		getFn: func(ctx context.Context, c cid.Cid) (blocks.Block, error) {
			if c.String() == blk.Cid().String() {
				return blk, nil
			}
			return nil, errors.New("not found")
		},
	}

	mockedBitSwap := &mockP2PBitSwap{}
	mockedHost := &mockP2PHost{idVal: peer.ID("testpeer")}

	c := &P2PClient{
		host:  mockedHost,
		bs:    mockedBitSwap,
		store: mockedBS,
	}

	data, err := c.Get(blk.Cid().String())

	require.NoError(t, err, "Get should succeed for blockstore hit")
	assert.Equal(t, testData, data, "Get should return block data")
}

func TestP2PClient_Get_BlockstoreMissBitswapFetch(t *testing.T) {
	testData := []byte("data from bitswap peer")
	blk, err := NewBlock(testData)
	require.NoError(t, err)

	var bitswapGetCalled bool

	mockedBS := &mockP2PBlockstore{
		hasFn: func(ctx context.Context, c cid.Cid) (bool, error) {
			return false, nil
		},
	}

	mockedBitSwap := &mockP2PBitSwap{
		getBlockFn: func(ctx context.Context, c cid.Cid) (blocks.Block, error) {
			bitswapGetCalled = true
			return blk, nil
		},
	}

	mockedHost := &mockP2PHost{idVal: peer.ID("testpeer")}

	c := &P2PClient{
		host:  mockedHost,
		bs:    mockedBitSwap,
		store: mockedBS,
	}

	data, err := c.Get(blk.Cid().String())

	require.NoError(t, err, "Get should succeed via bitswap")
	assert.Equal(t, testData, data, "Get should return block data from bitswap")
	assert.True(t, bitswapGetCalled, "bitswap.GetBlock should have been called on miss")
}

func TestP2PClient_PinLs_ReturnsAllCIDs(t *testing.T) {
	blk1, _ := NewBlock([]byte("block 1"))
	blk2, _ := NewBlock([]byte("block 2"))
	blk3, _ := NewBlock([]byte("block 3"))

	expectedCIDs := map[string]bool{
		blk1.Cid().String(): true,
		blk2.Cid().String(): true,
		blk3.Cid().String(): true,
	}

	mockedBS := &mockP2PBlockstore{
		allKeysFn: func(ctx context.Context) (<-chan cid.Cid, error) {
			ch := make(chan cid.Cid, 3)
			go func() {
				defer close(ch)
				ch <- blk1.Cid()
				ch <- blk2.Cid()
				ch <- blk3.Cid()
			}()
			return ch, nil
		},
	}

	mockedBitSwap := &mockP2PBitSwap{}
	mockedHost := &mockP2PHost{idVal: peer.ID("testpeer")}

	c := &P2PClient{
		host:  mockedHost,
		bs:    mockedBitSwap,
		store: mockedBS,
	}

	result, err := c.PinLs()

	require.NoError(t, err, "PinLs should succeed")
	assert.Equal(t, len(expectedCIDs), len(result), "PinLs should return all CIDs")
	for k, v := range expectedCIDs {
		assert.True(t, result[k] == v, "PinLs should contain CID %s", k)
	}
}

func TestP2PClient_PinRm_DeletesBlock(t *testing.T) {
	blk, _ := NewBlock([]byte("block to delete"))
	var deletedCID cid.Cid

	mockedBS := &mockP2PBlockstore{
		deleteFn: func(ctx context.Context, c cid.Cid) error {
			deletedCID = c
			return nil
		},
	}

	mockedBitSwap := &mockP2PBitSwap{}
	mockedHost := &mockP2PHost{idVal: peer.ID("testpeer")}

	c := &P2PClient{
		host:  mockedHost,
		bs:    mockedBitSwap,
		store: mockedBS,
	}

	err := c.PinRm(blk.Cid().String())

	require.NoError(t, err, "PinRm should succeed")
	assert.Equal(t, blk.Cid().String(), deletedCID.String(), "PinRm should delete correct CID")
}

func TestP2PClient_Ping_HostRunning(t *testing.T) {
	mockedHost := &mockP2PHost{idVal: peer.ID("testpeer")}
	mockedBitSwap := &mockP2PBitSwap{}
	mockedBS := &mockP2PBlockstore{}

	c := &P2PClient{
		host:  mockedHost,
		bs:    mockedBitSwap,
		store: mockedBS,
	}

	err := c.Ping()
	assert.NoError(t, err, "Ping should succeed when host is running")
}

func TestP2PClient_Ping_HostClosed(t *testing.T) {
	mockedBitSwap := &mockP2PBitSwap{}
	mockedBS := &mockP2PBlockstore{}

	c := &P2PClient{
		host:  &mockP2PHost{idVal: peer.ID("")},
		bs:    mockedBitSwap,
		store: mockedBS,
	}

	err := c.Ping()
	assert.Error(t, err, "Ping should fail when host is closed")
}

func TestP2PClient_ID_ReturnsPeerID(t *testing.T) {
	expectedID := peer.ID("QmTestPeerID12345")
	mockedHost := &mockP2PHost{idVal: expectedID}
	mockedBitSwap := &mockP2PBitSwap{}
	mockedBS := &mockP2PBlockstore{}

	c := &P2PClient{
		host:  mockedHost,
		bs:    mockedBitSwap,
		store: mockedBS,
	}

	idStr, err := c.ID()

	require.NoError(t, err, "ID should succeed")
	assert.Equal(t, expectedID.String(), idStr, "ID should return peer ID string")
}

func TestP2PClient_Get_BlockstoreError(t *testing.T) {
	mockedBS := &mockP2PBlockstore{
		hasFn: func(ctx context.Context, c cid.Cid) (bool, error) {
			return false, errors.New("blockstore error")
		},
	}

	mockedBitSwap := &mockP2PBitSwap{}
	mockedHost := &mockP2PHost{idVal: peer.ID("testpeer")}

	c := &P2PClient{
		host:  mockedHost,
		bs:    mockedBitSwap,
		store: mockedBS,
	}

	_, err := c.Get("QmTestCID")
	assert.Error(t, err, "Get should return error when blockstore fails")
}

func TestP2PClient_PinLs_BlockstoreError(t *testing.T) {
	mockedBS := &mockP2PBlockstore{
		allKeysFn: func(ctx context.Context) (<-chan cid.Cid, error) {
			return nil, errors.New("blockstore all keys error")
		},
	}

	mockedBitSwap := &mockP2PBitSwap{}
	mockedHost := &mockP2PHost{idVal: peer.ID("testpeer")}

	c := &P2PClient{
		host:  mockedHost,
		bs:    mockedBitSwap,
		store: mockedBS,
	}

	_, err := c.PinLs()
	assert.Error(t, err, "PinLs should return error when blockstore fails")
}

func TestP2PClient_PinRm_BlockstoreError(t *testing.T) {
	mockedBS := &mockP2PBlockstore{
		deleteFn: func(ctx context.Context, c cid.Cid) error {
			return errors.New("blockstore delete error")
		},
	}

	mockedBitSwap := &mockP2PBitSwap{}
	mockedHost := &mockP2PHost{idVal: peer.ID("testpeer")}

	c := &P2PClient{
		host:  mockedHost,
		bs:    mockedBitSwap,
		store: mockedBS,
	}

	err := c.PinRm("QmTestCID")
	assert.Error(t, err, "PinRm should return error when blockstore fails")
}

func TestP2PClient_Close_ClosesBlockstore(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	bs, err := NewBadgerBlockstore(ctx, tmpDir)
	require.NoError(t, err)

	mockedHost := &mockP2PHost{idVal: peer.ID("testpeer")}
	mockedBitSwap := &mockP2PBitSwap{}

	c := &P2PClient{
		host:  mockedHost,
		bs:    mockedBitSwap,
		store: bs,
	}

	err = c.Close()
	assert.NoError(t, err, "Close should succeed")
}
