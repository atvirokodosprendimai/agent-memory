// Package store manages encrypted memory entries on IPFS.
//
// It provides a high-level API for writing, reading, and querying
// memory entries. Each entry is encrypted with AES-256-GCM and pinned
// to IPFS. An encrypted index tracks all entries for efficient querying.
package store

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/atvirokodosprendimai/agent-memory/internal/config"
	"github.com/atvirokodosprendimai/agent-memory/internal/crypto"
	"github.com/atvirokodosprendimai/agent-memory/internal/ipfs"
)

// EntryType defines the kind of memory entry.
type EntryType string

const (
	TypeDecision    EntryType = "decision"
	TypeLearning    EntryType = "learning"
	TypeTrace       EntryType = "trace"
	TypeObservation EntryType = "observation"
	TypeBlocker     EntryType = "blocker"
	TypeContext     EntryType = "context"
)

// Entry is a single memory record.
type Entry struct {
	ID        string         `json:"id"`
	Type      EntryType      `json:"type"`
	Source    string         `json:"source"`
	Timestamp string         `json:"timestamp"`
	Tags      []string       `json:"tags"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Version   int            `json:"version"`
}

// IndexEntry is a lightweight entry in the encrypted index.
type IndexEntry struct {
	ID             string   `json:"id"`
	CID            string   `json:"cid"`
	Type           string   `json:"type"`
	Tags           []string `json:"tags"`
	Timestamp      string   `json:"timestamp"`
	Source         string   `json:"source"`
	ContentPreview string   `json:"content_preview"`
	Removed        bool     `json:"removed"`
}

// Index is the top-level index structure.
type Index struct {
	Version int                   `json:"version"`
	Updated string                `json:"updated"`
	Entries map[string]IndexEntry `json:"entries"`
}

// Merge combines two indexes using LWW per entry ID and tombstone-wins semantics.
// It is commutative, associative, and idempotent — satisfying CRDT requirements.
func (idx Index) Merge(other Index) Index {
	result := make(map[string]IndexEntry)

	// Copy idx entries into result
	for id, entry := range idx.Entries {
		result[id] = entry
	}

	// Merge other entries with LWW tiebreak and tombstone-wins
	for id, otherEntry := range other.Entries {
		if existing, ok := result[id]; !ok {
			// No existing entry — take otherEntry directly
			result[id] = otherEntry
		} else {
			// Determine the winner using LWW + tombstone-wins
			var winner IndexEntry

			if existing.Removed && !otherEntry.Removed {
				// Tombstone wins: existing is removed, other is not
				winner = existing
			} else if !existing.Removed && otherEntry.Removed {
				// Tombstone wins: other is removed, existing is not
				winner = otherEntry
			} else {
				// Neither or both are tombstones — use LWW
				if otherEntry.Timestamp > existing.Timestamp {
					winner = otherEntry
				} else if otherEntry.Timestamp == existing.Timestamp && otherEntry.Source > existing.Source {
					// Secondary tiebreak: deterministic order by Source string
					winner = otherEntry
				} else {
					winner = existing
				}
			}

			result[id] = winner
		}
	}

	// Result Updated is the newer of the two Updated timestamps
	resultUpdated := idx.Updated
	if other.Updated > resultUpdated {
		resultUpdated = other.Updated
	}

	return Index{
		Version: idx.Version,
		Updated: resultUpdated,
		Entries: result,
	}
}

// Store manages encrypted memory entries on IPFS.
type Store struct {
	cfg       *config.Config
	keys      *config.Keys
	ipfs      *ipfs.Client
	loadedCID string // CID used when the in-memory index was loaded; compared against cfg.IndexCID to detect concurrent writes
}

// New creates a new Store from config and secret.
func New(cfg *config.Config, secret string) (*Store, error) {
	keys, err := cfg.GetKeys(secret)
	if err != nil {
		return nil, fmt.Errorf("deriving keys: %w", err)
	}
	client := ipfs.NewClient(cfg.IPFSAddr)
	return &Store{cfg: cfg, keys: keys, ipfs: client}, nil
}

// Close releases resources.
func (s *Store) Close() error {
	return s.ipfs.Close()
}

// Write creates an encrypted memory entry and pins it to IPFS.
func (s *Store) Write(entryType EntryType, source string, tags []string, content string, metadata map[string]any) (*Entry, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	// Normalize tags
	normalizedTags := normalizeTags(tags)

	entry := &Entry{
		Type:      entryType,
		Source:    source,
		Timestamp: now,
		Tags:      normalizedTags,
		Content:   content,
		Metadata:  metadata,
		Version:   1,
	}

	// Compute deterministic ID
	entry.ID = s.computeID(entry)

	// Serialize and encrypt
	plainJSON, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("marshaling entry: %w", err)
	}

	ciphertext, err := crypto.Seal(s.keys.EncryptionKey, plainJSON)
	if err != nil {
		return nil, fmt.Errorf("encrypting entry: %w", err)
	}

	// Pin to IPFS
	cid, err := s.ipfs.Add(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("pinning to IPFS: %w", err)
	}

	// Update index
	if err := s.addToIndex(entry, cid); err != nil {
		// Entry is pinned but index failed — not fatal, but warn
		fmt.Printf("Warning: entry pinned (CID: %s) but index update failed: %v\n", cid, err)
	}

	return entry, nil
}

// Read retrieves and decrypts memory entries matching the given filters.
func (s *Store) Read(filter Filter) ([]*Entry, error) {
	idx, err := s.loadIndex()
	if err != nil {
		return nil, fmt.Errorf("loading index: %w", err)
	}

	var results []*Entry
	for _, ie := range idx.Entries {
		if !filter.Match(&ie) {
			continue
		}
		entry, err := s.getEntry(ie.CID)
		if err != nil {
			fmt.Printf("Warning: failed to decrypt entry %s: %v\n", ie.ID, err)
			continue
		}
		results = append(results, entry)
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp > results[j].Timestamp
	})

	return results, nil
}

// List returns index entries matching the filter (no decryption needed).
func (s *Store) List(filter Filter) ([]IndexEntry, error) {
	idx, err := s.loadIndex()
	if err != nil {
		return nil, fmt.Errorf("loading index: %w", err)
	}

	var results []IndexEntry
	for _, ie := range idx.Entries {
		if filter.Match(&ie) {
			results = append(results, ie)
		}
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}

	// Sort by timestamp descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp > results[j].Timestamp
	})

	return results, nil
}

// Pins returns all CIDs pinned by this store.
func (s *Store) Pins() (map[string]bool, error) {
	return s.ipfs.PinLs()
}

// GC removes entries older than maxAge from the index and unpins them.
// Returns the number of entries removed.
func (s *Store) GC(maxAge time.Duration) (int, error) {
	idx, err := s.loadIndex()
	if err != nil {
		return 0, fmt.Errorf("loading index: %w", err)
	}

	cutoff := time.Now().UTC().Add(-maxAge)
	var removedCIDs []string
	removed := 0
	var keysToDelete []string

	for _, ie := range idx.Entries {
		ts, err := time.Parse(time.RFC3339, ie.Timestamp)
		if err != nil || ts.Before(cutoff) {
			if ie.CID != "" {
				removedCIDs = append(removedCIDs, ie.CID)
			}
			keysToDelete = append(keysToDelete, ie.ID)
			removed++
		}
	}

	if removed == 0 {
		return 0, nil
	}

	for _, key := range keysToDelete {
		delete(idx.Entries, key)
	}
	idx.Updated = time.Now().UTC().Format(time.RFC3339)

	if err := s.saveIndex(idx); err != nil {
		return removed, fmt.Errorf("saving index after GC: %w", err)
	}

	var unpinErrs []error
	for _, cid := range removedCIDs {
		if pinErr := s.ipfs.PinRm(cid); pinErr != nil {
			unpinErrs = append(unpinErrs, pinErr)
			fmt.Printf("Warning: failed to unpin %s: %v\n", cid, pinErr)
		}
	}

	return removed, nil
}

// Export returns all decrypted entries matching the filter.
func (s *Store) Export(filter Filter) ([]*Entry, error) {
	return s.Read(Filter{
		Type:   filter.Type,
		Tags:   filter.Tags,
		Source: filter.Source,
		Since:  filter.Since,
		Limit:  0, // no limit for export
	})
}

// Import encrypts and pins entries, adding them to the index.
// It returns the number of entries imported.
func (s *Store) Import(entries []*Entry) (int, error) {
	imported := 0
	for _, entry := range entries {
		// Re-encrypt and pin
		plainJSON, err := json.Marshal(entry)
		if err != nil {
			return imported, fmt.Errorf("marshaling entry: %w", err)
		}

		ciphertext, err := crypto.Seal(s.keys.EncryptionKey, plainJSON)
		if err != nil {
			return imported, fmt.Errorf("encrypting entry: %w", err)
		}

		cid, err := s.ipfs.Add(ciphertext)
		if err != nil {
			return imported, fmt.Errorf("pinning to IPFS: %w", err)
		}

		if err := s.addToIndex(entry, cid); err != nil {
			fmt.Printf("Warning: entry pinned (CID: %s) but index update failed: %v\n", cid, err)
		}
		imported++
	}
	return imported, nil
}

// getEntry fetches and decrypts a single entry by CID.
func (s *Store) getEntry(cid string) (*Entry, error) {
	ciphertext, err := s.ipfs.Get(cid)
	if err != nil {
		return nil, fmt.Errorf("fetching from IPFS: %w", err)
	}

	plaintext, err := crypto.Open(s.keys.EncryptionKey, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypting entry: %w", err)
	}

	var entry Entry
	if err := json.Unmarshal(plaintext, &entry); err != nil {
		return nil, fmt.Errorf("parsing entry: %w", err)
	}

	return &entry, nil
}

// addToIndex adds an entry to the encrypted index and updates the config.
func (s *Store) addToIndex(entry *Entry, cid string) error {
	idx, err := s.loadIndex()
	if err != nil {
		// Start with empty index if none exists
		idx = &Index{Version: 1, Entries: make(map[string]IndexEntry)}
	}

	// Upsert: map assignment handles both insert and update — no duplicate scan needed
	idx.Entries[entry.ID] = IndexEntry{
		ID:             entry.ID,
		CID:            cid,
		Type:           string(entry.Type),
		Tags:           entry.Tags,
		Timestamp:      entry.Timestamp,
		Source:         entry.Source,
		ContentPreview: preview(entry.Content, 120),
		Removed:        false,
	}
	idx.Updated = time.Now().UTC().Format(time.RFC3339)

	return s.saveIndex(idx)
}

// loadIndex loads and decrypts the index from IPFS.
func (s *Store) loadIndex() (*Index, error) {
	if s.cfg.IndexCID == "" {
		s.loadedCID = ""
		return &Index{Version: 1, Entries: make(map[string]IndexEntry)}, nil
	}

	ciphertext, err := s.ipfs.Get(s.cfg.IndexCID)
	if err != nil {
		return nil, fmt.Errorf("fetching index: %w", err)
	}

	plaintext, err := crypto.Open(s.keys.IndexKey, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypting index: %w", err)
	}

	var idx Index
	if err := json.Unmarshal(plaintext, &idx); err != nil {
		return nil, fmt.Errorf("parsing index: %w", err)
	}

	s.loadedCID = s.cfg.IndexCID
	return &idx, nil
}

// saveIndex encrypts and pins the index, then updates the config.
// It detects concurrent writes by comparing the CID used to load the
// in-memory index (loadedCID) against the current cfg.IndexCID. If they
// differ, the remote index is merged into the local one before saving.
func (s *Store) saveIndex(idx *Index) error {
	merged := *idx

	// Concurrent-write detection: if loadedCID differs from cfg.IndexCID,
	// another agent wrote to the index since we loaded it.
	if s.cfg.IndexCID != "" && s.loadedCID != s.cfg.IndexCID {
		remote, err := s.loadIndex()
		if err == nil {
			// Merge local into remote: OR-Set union with LWW per key
			merged = idx.Merge(*remote)
		}
		// If err != nil, proceed with local index — remote couldn't be loaded
	}

	idxJSON, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshaling index: %w", err)
	}

	ciphertext, err := crypto.Seal(s.keys.IndexKey, idxJSON)
	if err != nil {
		return fmt.Errorf("encrypting index: %w", err)
	}

	cid, err := s.ipfs.Add(ciphertext)
	if err != nil {
		return fmt.Errorf("pinning index: %w", err)
	}

	s.cfg.IndexCID = cid
	s.loadedCID = cid // reset loadedCID to new CID after successful save
	configPath := configDir() + "/config.json"
	return s.cfg.Save(configPath)
}

func (s *Store) computeID(entry *Entry) string {
	mac := hmac.New(sha256.New, s.keys.SigningKey[:])
	mac.Write([]byte(string(entry.Type)))
	mac.Write([]byte(strings.Join(entry.Tags, ",")))
	mac.Write([]byte(entry.Content))
	mac.Write([]byte(entry.Timestamp))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(strings.ToLower(t))
		if t != "" && !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}
	sort.Strings(result)
	return result
}

func preview(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func configDir() string {
	return config.Dir()
}
