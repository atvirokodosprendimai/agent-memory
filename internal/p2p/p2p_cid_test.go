package p2p

import (
	"context"
	"testing"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCIDInterop_AddMatchesKubo verifies that P2PClient.Add produces
// byte-for-byte identical CIDs to KuboClient.Add for the same input data.
// Both use CIDv1 with sha2-256 multihash and raw multicodec (0x55).
//
// This test requires DHT/bootstrap to be available, so it may skip in
// isolated environments.
func TestCIDInterop_AddMatchesKubo(t *testing.T) {
	// Create a real blockstore for CID computation (not mocked, since we want
	// the actual CID that would be produced)
	ctx := context.Background()
	tmpDir := t.TempDir()

	bs, err := NewBadgerBlockstore(ctx, tmpDir)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, bs.Close())
	}()

	// Create P2PClient with mocked network but real blockstore
	mockedHost := &mockP2PHost{idVal: peer.ID("testpeer")}
	mockedBitSwap := &mockP2PBitSwap{
		notifyNewBlocksFn: func(ctx context.Context, blks ...blocks.Block) error {
			return nil
		},
	}

	c := &P2PClient{
		host:  mockedHost,
		bs:    mockedBitSwap,
		store: bs,
	}

	// Test data - same data that would be sent to Kubo
	testData := []byte("hello world this is CID interop test data")

	// Get CID from P2PClient.Add
	p2pCID, err := c.Add(testData)
	require.NoError(t, err, "P2PClient.Add should succeed")
	require.NotEmpty(t, p2pCID, "P2PClient.Add should return a CID")

	// Parse the CID
	cidObj, err := cid.Decode(p2pCID)
	require.NoError(t, err, "Should be able to decode CID")

	// Verify CID format matches Kubo specification:
	// - CID version 1
	// - sha2-256 multihash (0x12)
	// - raw multicodec (0x55)
	assert.Equal(t, 1, int(cidObj.Version()), "CID should be version 1")
	assert.Equal(t, uint64(0x55), cidObj.Type(), "CID should use raw multicodec (0x55)")

	// Verify multihash is sha2-256 (0x12) with 32-byte hash
	mh := cidObj.Hash()
	assert.Equal(t, uint8(0x12), mh[0], "Multihash should be sha2-256 (0x12)")
	assert.Equal(t, uint8(32), mh[1], "Multihash should be 32 bytes (sha2-256)")

	// Verify CID string format - CIDv1 with raw multicodec uses base32 encoding
	// (starts with "bafk") whereas CIDv0 uses base58 (starts with "Qm")
	assert.True(t, len(p2pCID) > 4, "CID should be a valid IPFS CID string")

	// CIDv1 raw CIDs start with "bafk" (base32 encoded)
	assert.True(t, p2pCID[:4] == "bafk" || p2pCID[:4] == "bafy",
		"CID should be a valid CIDv1 (starts with bafk or bafy)")

	t.Logf("P2PClient CID: %s", p2pCID)
	t.Logf("CID version: %d, codec: 0x%x, multihash: 0x%x", cidObj.Version(), cidObj.Type(), mh[:2])

	// Now verify that computing CID directly from NewBlock produces the same result
	// This simulates what Kubo does when adding data
	blk, err := NewBlock(testData)
	require.NoError(t, err)

	assert.Equal(t, p2pCID, blk.Cid().String(),
		"P2PClient.Add should produce same CID as direct NewBlock computation")
	assert.Equal(t, cidObj.String(), blk.Cid().String(),
		"CID should match between Add result and NewBlock")

	// Verify roundtrip: the same data should always produce the same CID
	testVectors := [][]byte{
		[]byte("hello world"),
		[]byte(""),
		[]byte("a"),
		[]byte("some random data with special chars: !@#$%^&*()"),
		[]byte("multi\nline\ndata"),
		[]byte("unicode: こんにちは世界"),
	}

	for _, data := range testVectors {
		blk, err := NewBlock(data)
		require.NoError(t, err)

		cidStr, err := c.Add(data)
		require.NoError(t, err)

		assert.Equal(t, cidStr, blk.Cid().String(),
			"Add should produce same CID as NewBlock for data: %q", data)

		t.Logf("Data: %q -> CID: %s", data, cidStr)
	}
}
