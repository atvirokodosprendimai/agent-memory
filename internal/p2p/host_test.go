package p2p

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

// TestHost_NewHost_Success tests that NewHost creates a running host with valid inputs.
func TestHost_NewHost_Success(t *testing.T) {
	ctx := context.Background()
	secret := "test-secret-key-for-hkdf"
	tmpDir := t.TempDir()

	h, err := NewHost(ctx, secret, tmpDir)
	if err != nil {
		t.Fatalf("NewHost failed: %v", err)
	}
	if h == nil {
		t.Fatal("NewHost returned nil host")
	}
	defer h.Close()

	// Verify host is running
	if h.Host() == nil {
		t.Error("Host.Host() returned nil")
	}
	if h.DHT() == nil {
		t.Error("Host.DHT() returned nil")
	}
	if h.PeerID() == peer.ID("") {
		t.Error("Host.PeerID() returned empty peer.ID")
	}
}

// TestHost_SameSecret_SamePeerID tests that the same secret produces the same peer ID.
func TestHost_SameSecret_SamePeerID(t *testing.T) {
	ctx := context.Background()
	secret := "consistent-secret-key"
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	h1, err := NewHost(ctx, secret, tmpDir1)
	if err != nil {
		t.Fatalf("NewHost first instance failed: %v", err)
	}
	defer h1.Close()

	h2, err := NewHost(ctx, secret, tmpDir2)
	if err != nil {
		t.Fatalf("NewHost second instance failed: %v", err)
	}
	defer h2.Close()

	if h1.PeerID() != h2.PeerID() {
		t.Errorf("Same secret produced different peer IDs: %s vs %s", h1.PeerID(), h2.PeerID())
	}
}

// TestHost_DifferentSecret_DifferentPeerID tests that different secrets produce different peer IDs.
func TestHost_DifferentSecret_DifferentPeerID(t *testing.T) {
	ctx := context.Background()
	secret1 := "secret-key-one"
	secret2 := "secret-key-two"
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	h1, err := NewHost(ctx, secret1, tmpDir1)
	if err != nil {
		t.Fatalf("NewHost first instance failed: %v", err)
	}
	defer h1.Close()

	h2, err := NewHost(ctx, secret2, tmpDir2)
	if err != nil {
		t.Fatalf("NewHost second instance failed: %v", err)
	}
	defer h2.Close()

	if h1.PeerID() == h2.PeerID() {
		t.Error("Different secrets produced same peer ID")
	}
}

// TestHost_KeyPersistence tests that the host key is persisted to disk.
func TestHost_KeyPersistence(t *testing.T) {
	ctx := context.Background()
	secret := "persistence-test-secret"
	tmpDir := t.TempDir()

	h1, err := NewHost(ctx, secret, tmpDir)
	if err != nil {
		t.Fatalf("NewHost first instance failed: %v", err)
	}
	peerID1 := h1.PeerID()
	h1.Close()

	// Create new host with same secret - should get same peer ID from persisted key
	h2, err := NewHost(ctx, secret, tmpDir)
	if err != nil {
		t.Fatalf("NewHost second instance failed: %v", err)
	}
	defer h2.Close()

	if h2.PeerID() != peerID1 {
		t.Errorf("Persisted key did not produce same peer ID: got %s, want %s", h2.PeerID(), peerID1)
	}

	// Verify hostkey file exists
	hostKeyPath := filepath.Join(tmpDir, "p2p", "hostkey")
	if _, err := os.Stat(hostKeyPath); os.IsNotExist(err) {
		t.Errorf("Host key file not persisted at %s", hostKeyPath)
	}
}

// TestHost_Close tests that Close shuts down the host cleanly.
func TestHost_Close(t *testing.T) {
	ctx := context.Background()
	secret := "close-test-secret"
	tmpDir := t.TempDir()

	h, err := NewHost(ctx, secret, tmpDir)
	if err != nil {
		t.Fatalf("NewHost failed: %v", err)
	}

	err = h.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	// Second close should return error or be idempotent
	err = h.Close()
	if err != nil {
		t.Logf("Second close returned error (acceptable): %v", err)
	}
}

// TestHost_InvalidSecret tests that empty secret is rejected.
func TestHost_InvalidSecret(t *testing.T) {
	ctx := context.Background()
	secret := ""
	tmpDir := t.TempDir()

	_, err := NewHost(ctx, secret, tmpDir)
	if err == nil {
		t.Error("NewHost should fail with empty secret")
	}
}

// TestHost_EmptyDataDir tests that empty dataDir is rejected.
func TestHost_EmptyDataDir(t *testing.T) {
	ctx := context.Background()
	secret := "test-secret"
	dataDir := ""

	_, err := NewHost(ctx, secret, dataDir)
	if err == nil {
		t.Error("NewHost should fail with empty dataDir")
	}
}
