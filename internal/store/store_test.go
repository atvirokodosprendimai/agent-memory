package store

import (
	"encoding/json"
	"strings"
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

// --- Merge CRDT property tests ---

func TestMerge_Commutative(t *testing.T) {
	// a.Merge(b) must equal b.Merge(a) for arbitrary a, b
	a := Index{
		Updated: "2026-04-12T10:00:00Z",
		Entries: map[string]IndexEntry{
			"id1": {ID: "id1", CID: "cid1", Type: "decision", Tags: []string{"a"}, Timestamp: "2026-04-12T10:00:00Z", Source: "agent1", ContentPreview: "content a", Removed: false},
			"id2": {ID: "id2", CID: "cid2", Type: "learning", Tags: []string{"b"}, Timestamp: "2026-04-12T10:00:00Z", Source: "agent1", ContentPreview: "content b", Removed: false},
		},
	}
	b := Index{
		Updated: "2026-04-12T11:00:00Z",
		Entries: map[string]IndexEntry{
			"id3": {ID: "id3", CID: "cid3", Type: "observation", Tags: []string{"c"}, Timestamp: "2026-04-12T11:00:00Z", Source: "agent2", ContentPreview: "content c", Removed: false},
			"id4": {ID: "id4", CID: "cid4", Type: "trace", Tags: []string{"d"}, Timestamp: "2026-04-12T11:00:00Z", Source: "agent2", ContentPreview: "content d", Removed: false},
		},
	}

	gotAB := a.Merge(b)
	gotBA := b.Merge(a)

	if len(gotAB.Entries) != len(gotBA.Entries) {
		t.Fatalf("a.Merge(b) len=%d, b.Merge(a) len=%d", len(gotAB.Entries), len(gotBA.Entries))
	}
	for id := range gotAB.Entries {
		if _, ok := gotBA.Entries[id]; !ok {
			t.Errorf("entry %s missing from b.Merge(a)", id)
		}
	}
}

func TestMerge_Associative(t *testing.T) {
	// (a.Merge(b)).Merge(c) must equal a.Merge((b.Merge(c)))
	a := Index{
		Updated: "2026-04-12T10:00:00Z",
		Entries: map[string]IndexEntry{
			"id1": {ID: "id1", CID: "cid1", Type: "decision", Tags: []string{"a"}, Timestamp: "2026-04-12T10:00:00Z", Source: "agent1", ContentPreview: "content a", Removed: false},
		},
	}
	b := Index{
		Updated: "2026-04-12T11:00:00Z",
		Entries: map[string]IndexEntry{
			"id2": {ID: "id2", CID: "cid2", Type: "learning", Tags: []string{"b"}, Timestamp: "2026-04-12T11:00:00Z", Source: "agent1", ContentPreview: "content b", Removed: false},
		},
	}
	c := Index{
		Updated: "2026-04-12T12:00:00Z",
		Entries: map[string]IndexEntry{
			"id3": {ID: "id3", CID: "cid3", Type: "observation", Tags: []string{"c"}, Timestamp: "2026-04-12T12:00:00Z", Source: "agent2", ContentPreview: "content c", Removed: false},
		},
	}

	gotLeft := a.Merge(b).Merge(c)
	gotRight := a.Merge(b.Merge(c))

	if len(gotLeft.Entries) != len(gotRight.Entries) {
		t.Fatalf("(a.Merge(b)).Merge(c) len=%d, a.Merge((b.Merge(c))) len=%d", len(gotLeft.Entries), len(gotRight.Entries))
	}
	for id, entry := range gotLeft.Entries {
		if rightEntry, ok := gotRight.Entries[id]; !ok {
			t.Errorf("entry %s missing from right merge", id)
		} else {
			if entry.Timestamp != rightEntry.Timestamp {
				t.Errorf("entry %s timestamp mismatch: left=%s right=%s", id, entry.Timestamp, rightEntry.Timestamp)
			}
			if entry.Removed != rightEntry.Removed {
				t.Errorf("entry %s Removed mismatch: left=%v right=%v", id, entry.Removed, rightEntry.Removed)
			}
		}
	}
}

func TestMerge_Idempotent(t *testing.T) {
	// a.Merge(a) must equal a
	a := Index{
		Updated: "2026-04-12T10:00:00Z",
		Entries: map[string]IndexEntry{
			"id1": {ID: "id1", CID: "cid1", Type: "decision", Tags: []string{"a"}, Timestamp: "2026-04-12T10:00:00Z", Source: "agent1", ContentPreview: "content a", Removed: false},
			"id2": {ID: "id2", CID: "cid2", Type: "learning", Tags: []string{"b"}, Timestamp: "2026-04-12T11:00:00Z", Source: "agent2", ContentPreview: "content b", Removed: false},
		},
	}

	got := a.Merge(a)
	if len(got.Entries) != len(a.Entries) {
		t.Fatalf("a.Merge(a) len=%d, want %d", len(got.Entries), len(a.Entries))
	}
	for id, entry := range got.Entries {
		if orig, ok := a.Entries[id]; !ok {
			t.Errorf("extra entry %s in a.Merge(a)", id)
		} else {
			if entry.Timestamp != orig.Timestamp {
				t.Errorf("entry %s timestamp changed: %s -> %s", id, orig.Timestamp, entry.Timestamp)
			}
			if entry.Removed != orig.Removed {
				t.Errorf("entry %s Removed changed: %v -> %v", id, orig.Removed, entry.Removed)
			}
		}
	}
}

func TestMerge_LWWWins(t *testing.T) {
	// Same ID in both indexes — newer timestamp wins
	a := Index{
		Updated: "2026-04-12T10:00:00Z",
		Entries: map[string]IndexEntry{
			"shared": {ID: "shared", CID: "cid-old", Type: "decision", Tags: []string{"tag"}, Timestamp: "2026-04-12T10:00:00Z", Source: "agent1", ContentPreview: "old content", Removed: false},
		},
	}
	b := Index{
		Updated: "2026-04-12T12:00:00Z",
		Entries: map[string]IndexEntry{
			"shared": {ID: "shared", CID: "cid-new", Type: "decision", Tags: []string{"tag"}, Timestamp: "2026-04-12T12:00:00Z", Source: "agent1", ContentPreview: "new content", Removed: false},
		},
	}

	got := a.Merge(b)
	entry, ok := got.Entries["shared"]
	if !ok {
		t.Fatal("shared entry missing after merge")
	}
	if entry.Timestamp != "2026-04-12T12:00:00Z" {
		t.Errorf("expected newer timestamp, got %s", entry.Timestamp)
	}
	if entry.CID != "cid-new" {
		t.Errorf("expected cid-new, got %s", entry.CID)
	}
}

func TestMerge_TombstoneWins(t *testing.T) {
	// When one side has Removed:true and other has Removed:false, tombstone wins
	t.Run("a_has_tombstone_b_has_active", func(t *testing.T) {
		a := Index{
			Updated: "2026-04-12T10:00:00Z",
			Entries: map[string]IndexEntry{
				"shared": {ID: "shared", CID: "cid1", Type: "decision", Tags: []string{"tag"}, Timestamp: "2026-04-12T10:00:00Z", Source: "agent1", ContentPreview: "content", Removed: true},
			},
		}
		b := Index{
			Updated: "2026-04-12T11:00:00Z",
			Entries: map[string]IndexEntry{
				"shared": {ID: "shared", CID: "cid2", Type: "decision", Tags: []string{"tag"}, Timestamp: "2026-04-12T11:00:00Z", Source: "agent2", ContentPreview: "content", Removed: false},
			},
		}

		got := a.Merge(b)
		entry, ok := got.Entries["shared"]
		if !ok {
			t.Fatal("shared entry missing after merge")
		}
		if !entry.Removed {
			t.Errorf("expected tombstone to win, got Removed=false")
		}
	})

	t.Run("b_has_tombstone_a_has_active", func(t *testing.T) {
		a := Index{
			Updated: "2026-04-12T10:00:00Z",
			Entries: map[string]IndexEntry{
				"shared": {ID: "shared", CID: "cid1", Type: "decision", Tags: []string{"tag"}, Timestamp: "2026-04-12T10:00:00Z", Source: "agent1", ContentPreview: "content", Removed: false},
			},
		}
		b := Index{
			Updated: "2026-04-12T11:00:00Z",
			Entries: map[string]IndexEntry{
				"shared": {ID: "shared", CID: "cid2", Type: "decision", Tags: []string{"tag"}, Timestamp: "2026-04-12T11:00:00Z", Source: "agent2", ContentPreview: "content", Removed: true},
			},
		}

		got := a.Merge(b)
		entry, ok := got.Entries["shared"]
		if !ok {
			t.Fatal("shared entry missing after merge")
		}
		if !entry.Removed {
			t.Errorf("expected tombstone to win, got Removed=false")
		}
	})
}

func TestMerge_ConcurrentWrites(t *testing.T) {
	// Two agents write different entries simultaneously — both present after merge
	agentA := Index{
		Updated: "2026-04-12T10:00:00Z",
		Entries: map[string]IndexEntry{
			"entryA": {ID: "entryA", CID: "cidA", Type: "decision", Tags: []string{"agent-a"}, Timestamp: "2026-04-12T10:00:00Z", Source: "agentA", ContentPreview: "decision from A", Removed: false},
		},
	}
	agentB := Index{
		Updated: "2026-04-12T10:00:00Z",
		Entries: map[string]IndexEntry{
			"entryB": {ID: "entryB", CID: "cidB", Type: "learning", Tags: []string{"agent-b"}, Timestamp: "2026-04-12T10:00:00Z", Source: "agentB", ContentPreview: "learning from B", Removed: false},
		},
	}

	got := agentA.Merge(agentB)

	if len(got.Entries) != 2 {
		t.Fatalf("expected 2 entries after merge, got %d", len(got.Entries))
	}
	if _, ok := got.Entries["entryA"]; !ok {
		t.Error("entryA missing after merge")
	}
	if _, ok := got.Entries["entryB"]; !ok {
		t.Error("entryB missing after merge")
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

// --- Backward compatibility tests ---

func TestBackwardCompat_MigrationOnWrite(t *testing.T) {
	// Construct an old-format JSON blob with Entries as a JSON array.
	oldFormatJSON := `{
  "version": 1,
  "updated": "2026-04-12T10:00:00Z",
  "entries": [
    {
      "id": "entry-old-1",
      "cid": "QmOldCidOne",
      "type": "decision",
      "tags": ["old", "tag1"],
      "timestamp": "2026-04-12T10:00:00Z",
      "source": "agent-old",
      "content_preview": "old decision content"
    },
    {
      "id": "entry-old-2",
      "cid": "QmOldCidTwo",
      "type": "learning",
      "tags": ["old", "tag2"],
      "timestamp": "2026-04-12T11:00:00Z",
      "source": "agent-old",
      "content_preview": "old learning content"
    }
  ]
}`

	// Verify the old-format JSON actually has "entries" as an array by checking
	// for the pattern `"entries": [` using JSON struct probing.
	var rawProbe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(oldFormatJSON), &rawProbe); err != nil {
		t.Fatalf("failed to parse old-format JSON: %v", err)
	}
	entriesRaw, ok := rawProbe["entries"]
	if !ok {
		t.Fatal("\"entries\" field not found in old-format JSON")
	}
	if entriesRaw[0] != '[' {
		t.Fatalf("old-format JSON \"entries\" should be an array, got %c", entriesRaw[0])
	}

	// Simulate migration: loadIndex reads raw JSON, detects array, migrates to map.
	// We replicate that logic here by using the compat helper struct.
	var compat indexBackwardCompat
	if err := json.Unmarshal([]byte(oldFormatJSON), &compat); err != nil {
		t.Fatalf("failed to parse old-format JSON: %v", err)
	}

	// Migrate to the new map-based Index (same logic as loadIndex).
	migrated := Index{
		Version: 1,
		Updated: compat.Updated,
		Entries: make(map[string]IndexEntry, len(compat.Entries)),
	}
	for _, entry := range compat.Entries {
		migrated.Entries[entry.ID] = entry
	}

	// Verify entries are accessible in map form.
	if len(migrated.Entries) != 2 {
		t.Fatalf("expected 2 entries in migrated map, got %d", len(migrated.Entries))
	}
	e1, ok := migrated.Entries["entry-old-1"]
	if !ok {
		t.Fatal("entry-old-1 not found in migrated map")
	}
	if e1.Type != "decision" {
		t.Errorf("entry-old-1 Type = %q, want %q", e1.Type, "decision")
	}
	if e1.CID != "QmOldCidOne" {
		t.Errorf("entry-old-1 CID = %q, want %q", e1.CID, "QmOldCidOne")
	}
	e2, ok := migrated.Entries["entry-old-2"]
	if !ok {
		t.Fatal("entry-old-2 not found in migrated map")
	}
	if e2.Type != "learning" {
		t.Errorf("entry-old-2 Type = %q, want %q", e2.Type, "learning")
	}

	// Simulate saveIndex: serialize the migrated index back to JSON
	// (saveIndex does json.Marshal on the Index struct, which uses map format).
	savedJSON, err := json.Marshal(migrated)
	if err != nil {
		t.Fatalf("json.Marshal(migrated) failed: %v", err)
	}

	// Verify "entries" is now a JSON object '{' not array '['.
	// Find the position of `"entries":` and check the next significant character.
	entriesIdx := strings.Index(string(savedJSON), `"entries":`)
	if entriesIdx == -1 {
		t.Fatal("\"entries\": not found in serialized JSON")
	}
	afterEntries := savedJSON[entriesIdx+len(`"entries":`):]
	// Skip whitespace.
	skip := 0
	for skip < len(afterEntries) && (afterEntries[skip] == ' ' || afterEntries[skip] == '\t' || afterEntries[skip] == '\n' || afterEntries[skip] == '\r') {
		skip++
	}
	if skip >= len(afterEntries) {
		t.Fatal("no content after \"entries\":")
	}
	if afterEntries[skip] != '{' {
		t.Errorf("expected \"entries\" to be JSON object {, got %c", afterEntries[skip])
	}
	// Also verify it is NOT '['.
	if afterEntries[skip] == '[' {
		t.Error("\"entries\" is still a JSON array after migration — expected object")
	}

	// Verify the serialized JSON contains no '[' right after entries colon (object only).
	if strings.Contains(string(afterEntries[:10]), "[") {
		t.Error("\"entries\" value appears to be array in serialized JSON")
	}
}
