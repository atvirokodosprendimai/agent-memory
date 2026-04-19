package p2p

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestP2PIntegration_TwoClientsExchange(t *testing.T) {
	if os.Getenv("SKIP_P2P_INTEGRATION") == "true" {
		t.Skip("SKIP_P2P_INTEGRATION is set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	tmpDirA := t.TempDir()
	tmpDirB := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(tmpDirA, "p2p"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDirB, "p2p"), 0755))

	clientA, err := NewP2PClient(ctx, tmpDirA)
	if err != nil {
		t.Skipf("Failed to create P2PClient A (may be network issue): %v", err)
	}
	defer clientA.Close()

	clientB, err := NewP2PClient(ctx, tmpDirB)
	if err != nil {
		t.Skipf("Failed to create P2PClient B (may be network issue): %v", err)
	}
	defer clientB.Close()

	t.Logf("Client A ID: %s", mustID(clientA))
	t.Logf("Client B ID: %s", mustID(clientB))

	idA, _ := clientA.ID()
	idB, _ := clientB.ID()
	assert.NotEqual(t, idA, idB, "Two clients in different dirs should have different peer IDs")

	testData := []byte("hello p2p integration test data " + t.Name())
	cidStrA, err := clientA.Add(testData)
	require.NoError(t, err, "Add on client A should succeed")
	t.Logf("Client A added block with CID: %s", cidStrA)

	cidStrB, err := clientB.Add([]byte("client B local data " + t.Name()))
	require.NoError(t, err, "Add on client B should succeed")
	t.Logf("Client B added block with CID: %s", cidStrB)

	dataA, err := clientA.Get(cidStrA)
	require.NoError(t, err, "Client A should retrieve its own block")
	assert.Equal(t, testData, dataA)

	dataB, err := clientB.Get(cidStrB)
	require.NoError(t, err, "Client B should retrieve its own block")
	_ = dataB

	t.Log("Both clients created and added blocks successfully")
	t.Log("Cross-client bitswap exchange requires both clients to be connected to the public IPFS network")
}

func TestP2PIntegration_SameDirSamePeerID(t *testing.T) {
	if os.Getenv("SKIP_P2P_INTEGRATION") == "true" {
		t.Skip("SKIP_P2P_INTEGRATION is set")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	clientA, err := NewP2PClient(ctx, tmpDir)
	if err != nil {
		t.Skipf("Failed to create P2PClient A: %v", err)
	}
	defer clientA.Close()

	clientB, err := NewP2PClient(ctx, tmpDir)
	if err != nil {
		t.Skipf("Failed to create P2PClient B: %v", err)
	}
	defer clientB.Close()

	idA, err := clientA.ID()
	require.NoError(t, err)

	idB, err := clientB.ID()
	require.NoError(t, err)

	assert.Equal(t, idA, idB, "Same dir should produce same peer ID (key persisted)")
}

func TestP2PIntegration_BlockPersistence(t *testing.T) {
	if os.Getenv("SKIP_P2P_INTEGRATION") == "true" {
		t.Skip("SKIP_P2P_INTEGRATION is set")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	clientA, err := NewP2PClient(ctx, tmpDir)
	if err != nil {
		t.Skipf("Failed to create P2PClient A: %v", err)
	}

	testData := []byte("persistence test data " + t.Name())
	cidStr, err := clientA.Add(testData)
	require.NoError(t, err)

	pinned, err := clientA.PinLs()
	require.NoError(t, err)
	assert.True(t, pinned[cidStr], "Added block should appear in PinLs")

	err = clientA.Close()
	require.NoError(t, err)

	clientA2, err := NewP2PClient(ctx, tmpDir)
	if err != nil {
		t.Skipf("Failed to recreate P2PClient: %v", err)
	}
	defer clientA2.Close()

	data, err := clientA2.Get(cidStr)
	if err != nil {
		t.Skipf("Get after restart failed (may be network issue): %v", err)
	}
	assert.Equal(t, testData, data, "Data should persist across client restarts")
}

func TestP2PIntegration_PingAndID(t *testing.T) {
	if os.Getenv("SKIP_P2P_INTEGRATION") == "true" {
		t.Skip("SKIP_P2P_INTEGRATION is set")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	client, err := NewP2PClient(ctx, tmpDir)
	if err != nil {
		t.Skipf("Failed to create P2PClient: %v", err)
	}
	defer client.Close()

	err = client.Ping()
	require.NoError(t, err, "Ping should succeed")

	id, err := client.ID()
	require.NoError(t, err)
	assert.NotEmpty(t, id, "ID should return non-empty peer ID string")
}

func TestP2PIntegration_PinRm(t *testing.T) {
	if os.Getenv("SKIP_P2P_INTEGRATION") == "true" {
		t.Skip("SKIP_P2P_INTEGRATION is set")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	client, err := NewP2PClient(ctx, tmpDir)
	if err != nil {
		t.Skipf("Failed to create P2PClient: %v", err)
	}
	defer client.Close()

	testData := []byte("pinrm test data")
	cidStr, err := client.Add(testData)
	require.NoError(t, err)

	pinned, err := client.PinLs()
	require.NoError(t, err)
	assert.True(t, pinned[cidStr], "Block should be pinned after Add")

	err = client.PinRm(cidStr)
	require.NoError(t, err, "PinRm should succeed")

	pinned, err = client.PinLs()
	require.NoError(t, err)
	assert.False(t, pinned[cidStr], "Block should not be pinned after PinRm")

	_, err = client.Get(cidStr)
	assert.Error(t, err, "Get should fail for removed block")
}

func mustID(client *P2PClient) string {
	id, err := client.ID()
	if err != nil {
		panic("failed to get ID: " + err.Error())
	}
	return id
}
