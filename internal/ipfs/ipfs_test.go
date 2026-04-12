package ipfs

import (
	"bytes"
	"testing"
)

func newTestClient(t *testing.T) *Client {
	t.Helper()
	c := NewClient("http://localhost:5001")
	if err := c.Ping(); err != nil {
		c.Close()
		t.Skipf("IPFS daemon not running: %v", err)
	}
	return c
}

func TestPing(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	if err := c.Ping(); err != nil {
		t.Fatalf("Ping failed on reachable daemon: %v", err)
	}
}

func TestAddGetRoundTrip(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	original := []byte("hello ipfs integration test 🚀")

	// Add
	cid, err := c.Add(original)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	t.Logf("Added data, CID: %s", cid)

	if cid == "" {
		t.Fatal("Add returned empty CID")
	}

	// Get
	got, err := c.Get(cid)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !bytes.Equal(got, original) {
		t.Fatalf("Get mismatch: got %q, want %q", got, original)
	}
}

func TestPinLs(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	// Add something to guarantee at least one pin exists.
	cid, err := c.Add([]byte("pinls test data"))
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	t.Logf("Added for PinLs test, CID: %s", cid)

	pins, err := c.PinLs()
	if err != nil {
		t.Fatalf("PinLs failed: %v", err)
	}

	if len(pins) == 0 {
		t.Fatal("PinLs returned no pins")
	}

	if !pins[cid] {
		t.Fatalf("PinLs does not contain added CID %s; pins: %v", cid, pins)
	}

	t.Logf("PinLs returned %d pins", len(pins))
}

func TestPinRm(t *testing.T) {
	c := newTestClient(t)
	defer c.Close()

	// Add something to pin.
	cid, err := c.Add([]byte("pinrm test data"))
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	t.Logf("Added for PinRm test, CID: %s", cid)

	// Verify it is pinned.
	pins, err := c.PinLs()
	if err != nil {
		t.Fatalf("PinLs before remove failed: %v", err)
	}
	if !pins[cid] {
		t.Fatalf("CID %s not found in pins before removal", cid)
	}

	// Unpin.
	if err := c.PinRm(cid); err != nil {
		t.Fatalf("PinRm failed: %v", err)
	}

	// Verify it is no longer pinned.
	pins, err = c.PinLs()
	if err != nil {
		t.Fatalf("PinLs after remove failed: %v", err)
	}
	if pins[cid] {
		t.Fatalf("CID %s still present in pins after PinRm", cid)
	}

	t.Logf("PinRm successfully removed CID %s", cid)
}
