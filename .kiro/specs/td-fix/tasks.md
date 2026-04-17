# Tasks: technical-debt-fix

## Implementation Notes
- All tasks use Go stdlib + golang.org/x/crypto only
- Tests must not require IPFS daemon — use mock interfaces
- All changes must be backward compatible
- No CLI flag or API changes

## Task List

### 1. HTTP Client Timeout
- [x] 1.1 Add 30s timeout to ipfs.Client http.Client in `internal/ipfs/client.go`

### 2. Stdin Size Limit
- [x] 2.1 Add 10MB max content size check in `cmd/agent-memory/commands.go:89-99`

### 3. Source Case Normalization
- [x] 3.1 Normalize Source to lowercase in `store/filter.go:25-27` (match Tags behavior)

### 4. Buffered JSONL Export
- [ ] 4.1 Use buffered writer in `cmd/agent-memory/commands.go:366-373` for JSONL export

### 5. GC Atomicity
- [ ] 5.1 Ensure GC saves index before unpinning in `internal/store/store.go:200-215` (collect errors, fail before changes)

### 6. Signal Handling
- [ ] 6.1 Add signal trap with graceful drain in `cmd/agent-memory/main.go:12-75`

### 7. Command Test Coverage
- [ ] 7.1 Add unit tests for runWrite/runRead/runGC using mock store in `cmd/agent-memory/commands_test.go`

## Dependency Order
1 → 2 → 3 → 4 → 5 → 6 → 7 (no inter-task dependencies, sequential for safety)