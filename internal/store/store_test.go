package store

import (
	"testing"

	"github.com/atvirokodosprendimai/agent-memory/internal/config"
	"github.com/atvirokodosprendimai/agent-memory/internal/ipfs"
)

// --- Pure function unit tests (no IPFS needed) ---

func TestNormalizeTags(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "lowercases and sorts",
			in:   []string{"Stripe", "billing", "API"},
			want: []string{"api", "billing", "stripe"},
		},
		{
			name: "deduplicates",
			in:   []string{"billing", "Billing", "BILLING"},
			want: []string{"billing"},
		},
		{
			name: "trims whitespace",
			in:   []string{" billing ", " stripe"},
			want: []string{"billing", "stripe"},
		},
		{
			name: "filters empty strings",
			in:   []string{"", "billing", "", "stripe", ""},
			want: []string{"billing", "stripe"},
		},
		{
			name: "nil input returns empty slice",
			in:   nil,
			want: []string{},
		},
		{
			name: "single tag",
			in:   []string{"billing"},
			want: []string{"billing"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeTags(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("normalizeTags(%v) = %v (len %d), want %v (len %d)", tt.in, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("normalizeTags(%v)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPreview(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string unchanged",
			input:  "hello",
			maxLen: 120,
			want:   "hello",
		},
		{
			name:   "exact length unchanged",
			input:  "12345",
			maxLen: 5,
			want:   "12345",
		},
		{
			name:   "long string truncated with ellipsis",
			input:  "this is a very long string that exceeds the limit",
			maxLen: 20,
			want:   "this is a very lo...",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 120,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := preview(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("preview(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestComputeIDDeterministic(t *testing.T) {
	cfg, err := config.Create("test-secret-compute-id", "http://localhost:5001")
	if err != nil {
		t.Fatalf("config.Create: %v", err)
	}
	keys, err := cfg.GetKeys("test-secret-compute-id")
	if err != nil {
		t.Fatalf("GetKeys: %v", err)
	}

	s := &Store{keys: keys}

	entry := &Entry{
		Type:      TypeDecision,
		Tags:      []string{"billing", "stripe"},
		Content:   "Use Stripe for billing",
		Timestamp: "2026-04-12T11:05:00Z",
	}

	id1 := s.computeID(entry)
	id2 := s.computeID(entry)

	if id1 != id2 {
		t.Errorf("computeID not deterministic: %q != %q", id1, id2)
	}
}

func TestComputeIDDifferentContent(t *testing.T) {
	cfg, err := config.Create("test-secret-diff", "http://localhost:5001")
	if err != nil {
		t.Fatalf("config.Create: %v", err)
	}
	keys, err := cfg.GetKeys("test-secret-diff")
	if err != nil {
		t.Fatalf("GetKeys: %v", err)
	}

	s := &Store{keys: keys}

	entry1 := &Entry{Type: TypeDecision, Tags: []string{"billing"}, Content: "Use Stripe", Timestamp: "2026-04-12T11:05:00Z"}
	entry2 := &Entry{Type: TypeDecision, Tags: []string{"billing"}, Content: "Use Paddle", Timestamp: "2026-04-12T11:05:00Z"}

	id1 := s.computeID(entry1)
	id2 := s.computeID(entry2)

	if id1 == id2 {
		t.Error("different content produced same ID")
	}
}

func TestComputeIDDifferentKeys(t *testing.T) {
	cfg1, _ := config.Create("secret-one", "http://localhost:5001")
	cfg2, _ := config.Create("secret-two", "http://localhost:5001")
	keys1, _ := cfg1.GetKeys("secret-one")
	keys2, _ := cfg2.GetKeys("secret-two")

	s1 := &Store{keys: keys1}
	s2 := &Store{keys: keys2}

	entry := &Entry{Type: TypeDecision, Tags: []string{"billing"}, Content: "Same content", Timestamp: "2026-04-12T11:05:00Z"}

	id1 := s1.computeID(entry)
	id2 := s2.computeID(entry)

	if id1 == id2 {
		t.Error("different keys produced same ID (extremely unlikely)")
	}
}

// --- Integration tests (require IPFS daemon) ---

const testSecret = "integration-test-secret-16chars"

func newTestStore(t *testing.T) *Store {
	t.Helper()

	// Check IPFS daemon is running
	client := ipfs.NewClient("http://localhost:5001")
	if err := client.Ping(); err != nil {
		client.Close()
		t.Skipf("IPFS daemon not running: %v", err)
	}
	client.Close()

	cfg, err := config.Create(testSecret, "http://localhost:5001")
	if err != nil {
		t.Fatalf("config.Create: %v", err)
	}

	// Save config to temp dir so saveIndex works
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("cfg.Save: %v", err)
	}

	s, err := New(cfg, testSecret)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}

	t.Cleanup(func() { s.Close() })
	return s
}

func TestWriteAndReadRoundTrip(t *testing.T) {
	s := newTestStore(t)

	entry, err := s.Write(TypeDecision, "human", []string{"billing", "Stripe"}, "Use Stripe for billing v3", nil)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if entry.ID == "" {
		t.Fatal("Write returned entry with empty ID")
	}
	if entry.Type != TypeDecision {
		t.Errorf("Type = %q, want %q", entry.Type, TypeDecision)
	}
	if entry.Source != "human" {
		t.Errorf("Source = %q, want %q", entry.Source, "human")
	}
	if entry.Content != "Use Stripe for billing v3" {
		t.Errorf("Content = %q, want original", entry.Content)
	}
	// Tags should be normalized (lowercased, sorted)
	if len(entry.Tags) != 2 || entry.Tags[0] != "billing" || entry.Tags[1] != "stripe" {
		t.Errorf("Tags = %v, want [billing stripe]", entry.Tags)
	}

	// Read it back
	results, err := s.Read(Filter{Type: TypeDecision, Limit: 10})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Read returned no results")
	}

	found := false
	for _, r := range results {
		if r.ID == entry.ID {
			found = true
			if r.Content != entry.Content {
				t.Errorf("Read content = %q, want %q", r.Content, entry.Content)
			}
			break
		}
	}
	if !found {
		t.Errorf("written entry %s not found in Read results", entry.ID[:16])
	}
}

func TestListReturnsMetadata(t *testing.T) {
	s := newTestStore(t)

	_, err := s.Write(TypeLearning, "goose", []string{"test-list"}, "List test content", nil)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	results, err := s.List(Filter{Tags: []string{"test-list"}, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("List returned no results")
	}

	ie := results[0]
	if ie.Type != "learning" {
		t.Errorf("Type = %q, want learning", ie.Type)
	}
	if ie.Source != "goose" {
		t.Errorf("Source = %q, want goose", ie.Source)
	}
	if ie.CID == "" {
		t.Error("CID is empty")
	}
	if ie.ContentPreview == "" {
		t.Error("ContentPreview is empty")
	}
}

func TestWriteWithMetadata(t *testing.T) {
	s := newTestStore(t)

	meta := map[string]any{
		"session_id": "test-session-123",
		"repos":      []string{"wgmesh"},
	}

	entry, err := s.Write(TypeTrace, "claude-code", []string{"test-meta"}, "Trace with metadata", meta)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if entry.Metadata == nil {
		t.Fatal("Metadata is nil")
	}
	if entry.Metadata["session_id"] != "test-session-123" {
		t.Errorf("Metadata[session_id] = %v, want test-session-123", entry.Metadata["session_id"])
	}

	// Read back and verify metadata survives encrypt/decrypt round-trip
	results, err := s.Read(Filter{Tags: []string{"test-meta"}, Limit: 1})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Read returned no results")
	}
	if results[0].Metadata["session_id"] != "test-session-123" {
		t.Errorf("Read metadata[session_id] = %v, want test-session-123", results[0].Metadata["session_id"])
	}
}

func TestWriteDeduplicate(t *testing.T) {
	s := newTestStore(t)

	entry1, err := s.Write(TypeDecision, "human", []string{"dedup-test"}, "Same content same time", nil)
	if err != nil {
		t.Fatalf("Write 1: %v", err)
	}

	// Writing the exact same entry should produce the same ID (deterministic).
	// The timestamp will differ since time.Now() is called, so this actually
	// produces a different ID. This test verifies that two sequential writes
	// with different timestamps get different IDs.
	entry2, err := s.Write(TypeDecision, "human", []string{"dedup-test"}, "Same content same time", nil)
	if err != nil {
		t.Fatalf("Write 2: %v", err)
	}

	// Timestamps differ → IDs differ
	if entry1.ID == entry2.ID {
		t.Error("two writes with different timestamps should produce different IDs")
	}
}

func TestReadWithNoEntries(t *testing.T) {
	s := newTestStore(t)

	// Filter for a tag that doesn't exist
	results, err := s.Read(Filter{Tags: []string{"nonexistent-tag-xyz"}, Limit: 10})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestReadRespectsLimit(t *testing.T) {
	s := newTestStore(t)

	// Write 3 entries
	for i := 0; i < 3; i++ {
		_, err := s.Write(TypeObservation, "test", []string{"limit-test"}, "Entry for limit test", nil)
		if err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	results, err := s.Read(Filter{Tags: []string{"limit-test"}, Limit: 2})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if len(results) > 2 {
		t.Errorf("Read returned %d entries, limit was 2", len(results))
	}
}

func TestNewStoreRejectsEmptySecret(t *testing.T) {
	cfg, err := config.Create("valid-secret-here", "http://localhost:5001")
	if err != nil {
		t.Fatalf("config.Create: %v", err)
	}

	_, err = New(cfg, "")
	if err == nil {
		t.Error("New with empty secret should return error")
	}
}
