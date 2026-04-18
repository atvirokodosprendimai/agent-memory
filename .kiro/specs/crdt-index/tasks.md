# Tasks: crdt-index

## Task Map

| # | Boundary | Description | Parallel? | Depends |
|---|----------|-------------|-----------|---------|
| 1 | `internal/store/store.go` | Add `Removed` field to `IndexEntry` struct | P | — |
| 2 | `internal/store/store.go` | Change `Index.Entries` from `[]IndexEntry` to `map[string]IndexEntry` | P | — |
| 3 | `internal/store/store.go` | Implement `Merge(other Index) Index` with LWW per entry ID | P | — |
| 4 | `internal/store/store.go` | Update `saveIndex` for concurrent-write detection + merge | P | 3 |
| 5 | `internal/store/store.go` | Update `addToIndex` for map-based insertion | P | 2 |
| 6 | `internal/store/store.go` | Update `loadIndex` for backward compat — detect old slice format, migrate to map | P | 2 |
| 7 | `internal/store/store.go` | Update `List` and `Read` to skip `removed: true` entries | P | 2 |
| 8 | `internal/store/store.go` | Update `GC` to use tombstones (`removed: true`) instead of rebuilding slice | P | 2 |
| 9 | `internal/store/filter.go` | Update `filter.go` `Match()` to iterate over map instead of slice | P | 2 |
| 10 | `internal/store/store_test.go` | Add unit tests for `Merge` — concurrent writes, LWW, tombstone wins over add | P | 3 |
| 11 | `internal/store/store_test.go` | Add backward compat migration test — old slice format → new map format | P | 6 |
| 12 | `internal/store/store_test.go` | Run full test suite — all tests pass | P | 5, 7, 8, 9, 10, 11 |

## Details

### Task 1 — Add `Removed` field to `IndexEntry` struct
_Boundary: `internal/store/store.go`_

Add `Removed bool `json:"removed"`` to the `IndexEntry` struct. Default is `false` (zero value). No other changes in this task.

### Task 2 — Change `Index.Entries` from `[]IndexEntry` to `map[string]IndexEntry`
_Boundary: `internal/store/store.go`_

Change `Entries []IndexEntry` to `Entries map[string]IndexEntry` in the `Index` struct. Initialize with `make(map[string]IndexEntry)` wherever an empty index is created. All existing slice-based references become map references (iteration, access).

### Task 3 — Implement `Merge(other Index) Index` with LWW per entry ID
_Boundary: `internal/store/store.go`_

Implement `func (idx Index) Merge(other Index) Index`. See design.md Section 2.3 for algorithm. Key rules:
- Union of all entry IDs from both maps.
- LWW tiebreak: newer `Timestamp` wins.
- Secondary tiebreak on `Source` for deterministic results when timestamps equal.
- Tombstone wins: `Removed: true` on either side overwrites `Removed: false`.
- Result `Updated` is the newer of the two `Updated` timestamps.
- CRDT properties: commutative, associative, idempotent.

### Task 4 — Update `saveIndex` for concurrent-write detection + merge
_Boundary: `internal/store/store.go`_

See design.md Section 2.2. Track the CID used to load the in-memory index (the `loadedCID`). On `saveIndex`, compare the current `s.cfg.IndexCID` against the `loadedCID`. If they differ, a concurrent write occurred:
1. Load the remote index via `loadIndex()`.
2. Merge local into remote via `idx.Merge(remote)`.
3. Save the merged result.

If no concurrent modification, save as-is. The `loadedCID` is set by `loadIndex` and compared at save time.

### Task 5 — Update `addToIndex` for map-based insertion
_Boundary: `internal/store/store.go`_

Replace slice append with map upsert: `idx.Entries[entry.ID] = IndexEntry{...}`. No duplicate scan needed. No other logic changes.

### Task 6 — Update `loadIndex` for backward compat — detect old slice format, migrate to map
_Boundary: `internal/store/store.go`_

See design.md Section 4.1. Probe raw JSON bytes to detect if `"entries"` is a JSON array (`[`) or object (`{`). If array: deserialize into helper struct with slice, migrate each entry into `map[string]IndexEntry` keyed by `entry.ID`. If object: normal unmarshal. Return normalized map struct. The first subsequent `saveIndex` persists the new format transparently.

### Task 7 — Update `List` and `Read` to skip `removed: true` entries
_Boundary: `internal/store/store.go`_

In `List`: iterate `range idx.Entries`, skip `entry.Removed == true` before applying filter. In `Read`: same — skip removed entries. No changes to filter signatures.

### Task 8 — Update `GC` to use tombstones (`removed: true`) instead of rebuilding slice
_Boundary: `internal/store/store.go`_

Replace slice filtering with tombstone setting: for each expired entry, set `entry.Removed = true` in the map. Do not delete from map. Unpin CIDs as before. Call `saveIndex(idx)` to persist tombstones. Return count of entries tombstoned.

### Task 9 — Update `filter.go` `Match()` to iterate over map instead of slice
_Boundary: `internal/store/filter.go`_

`Match()` receives a single `*IndexEntry` — no structural change to `Match()` itself. The caller (`List`/`Read` in store.go) iterates over the map and passes each entry to `Match`. This task only affects the iteration site in `List`/`Read`, not `filter.go` itself, so this task collapses into Task 7. No changes to `filter.go` needed beyond confirming it already handles `*IndexEntry` correctly.

### Task 10 — Add unit tests for `Merge`
_Boundary: `internal/store/store_test.go`_

Add the following tests:
- `TestMerge_Commutative`: `a.Merge(b)` equals `b.Merge(a)`.
- `TestMerge_Associative`: `(a.Merge(b)).Merge(c)` equals `a.Merge((b.Merge(c)))`.
- `TestMerge_Idempotent`: `a.Merge(a)` equals `a`.
- `TestMerge_LWWWins`: Same ID with different timestamps — newer timestamp wins.
- `TestMerge_TombstoneWins`: `Removed: true` on one side wins over `Removed: false` on the other.
- `TestMerge_ConcurrentWrites`: Two agents write different entries simultaneously; both entries present after merge.

### Task 11 — Add backward compat migration test
_Boundary: `internal/store/store_test.go`_

Add `TestBackwardCompat_MigrationOnWrite`: serialize an old-format `Index` with `Entries []IndexEntry`, load it, verify entries are accessible in map form, call `saveIndex`, serialize the result, verify `"entries"` is now a JSON object (`{`) not array (`[`).

### Task 12 — Run full test suite
_Boundary: `internal/store/store_test.go`_

Run `go test ./internal/store/... -v`. All existing tests pass. New CRDT tests pass. Backward compat test passes.
