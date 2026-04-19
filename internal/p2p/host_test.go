package p2p

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestHost_NewHost_Success(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	h, err := NewHost(ctx, tmpDir)
	if err != nil {
		t.Fatalf("NewHost failed: %v", err)
	}
	if h == nil {
		t.Fatal("NewHost returned nil host")
	}
	defer h.Close()

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

func TestHost_DifferentDirs_DifferentPeerID(t *testing.T) {
	ctx := context.Background()
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	h1, err := NewHost(ctx, tmpDir1)
	if err != nil {
		t.Fatalf("NewHost first instance failed: %v", err)
	}
	defer h1.Close()

	h2, err := NewHost(ctx, tmpDir2)
	if err != nil {
		t.Fatalf("NewHost second instance failed: %v", err)
	}
	defer h2.Close()

	if h1.PeerID() == h2.PeerID() {
		t.Errorf("Different dirs should produce different peer IDs: %s", h1.PeerID())
	}
}

func TestHost_KeyPersistence(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	h1, err := NewHost(ctx, tmpDir)
	if err != nil {
		t.Fatalf("NewHost first instance failed: %v", err)
	}
	peerID1 := h1.PeerID()
	h1.Close()

	h2, err := NewHost(ctx, tmpDir)
	if err != nil {
		t.Fatalf("NewHost second instance failed: %v", err)
	}
	defer h2.Close()

	if h2.PeerID() != peerID1 {
		t.Errorf("Persisted key did not produce same peer ID: got %s, want %s", h2.PeerID(), peerID1)
	}

	hostKeyPath := filepath.Join(tmpDir, "p2p", "hostkey")
	if _, err := os.Stat(hostKeyPath); os.IsNotExist(err) {
		t.Errorf("Host key file not persisted at %s", hostKeyPath)
	}
}

func TestHost_Close(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	h, err := NewHost(ctx, tmpDir)
	if err != nil {
		t.Fatalf("NewHost failed: %v", err)
	}

	err = h.Close()
	if err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	err = h.Close()
	if err != nil {
		t.Logf("Second close returned error (acceptable): %v", err)
	}
}

func TestHost_EmptyDataDir(t *testing.T) {
	ctx := context.Background()
	dataDir := ""

	_, err := NewHost(ctx, dataDir)
	if err == nil {
		t.Error("NewHost should fail with empty dataDir")
	}
}
