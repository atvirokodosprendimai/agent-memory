# Technology Stack

## Architecture

Go CLI with layered internal packages. The application is structured as:

```
cmd/agent-memory/     → CLI entry point, command dispatch, skill integration
internal/
  config/             → Config file, HKDF key derivation
  crypto/             → AES-256-GCM encryption, HMAC-SHA256
  ipfs/               → Kubo HTTP RPC client
  store/              → Encrypted entry storage, indexing, CRDT-style merge
```

Command handlers depend on `internal/config` and `internal/store`; store depends on `internal/ipfs` and `internal/crypto`.

## Core Technologies

- **Language**: Go 1.25.6
- **Cryptography**: `golang.org/x/crypto` (HKDF-SHA256, AES-256-GCM, HMAC-SHA256)
- **IPFS**: Kubo daemon HTTP RPC API (`http://localhost:5001`)
- **Standard library**: `net/http`, `encoding/json`, `flag`, `os`, `os/signal`, `crypto/rand`

## Key Libraries

Only one external dependency: `golang.org/x/crypto`. All other functionality uses the Go standard library.

## Development Standards

### Type Safety
Go's static typing. No `any` assertions without type checks. Interface definitions used for store/IPFS/crypto to enable test doubles.

### Code Quality
- `go vet ./...` — static analysis
- `go build ./...` — compilation check
- `go test ./...` — unit tests alongside package files (`*_test.go`)

### Testing
- Table-driven tests following Go convention
- Store factory pattern (`storeFactory` variable) enables dependency injection for tests
- No external IPFS required for unit tests — IPFS client methods mocked via interface

## Development Environment

### Required Tools
- Go 1.25+
- IPFS daemon (Kubo) running locally (`ipfs daemon`)
- Optional: `agent-memory` binary built from source

### Common Commands
```bash
go build ./...          # Build
go test ./...           # Test
go vet ./...            # Lint
go run ./cmd/agent-memory -- init --secret "mysecret"  # Init
agent-memory write --type decision --content "..."      # Write entry
agent-memory read --type decision                        # Read entries
```

## Key Technical Decisions

### Why IPFS for storage?
Content-addressed, decentralized, pinned permanence. No database to manage, no server to run. Any IPFS node (local Kubo, remote pinning service, browser-based Helia) serves the same data.

### Why AES-256-GCM + HKDF?
Standard, battle-tested crypto. HKDF-SHA256 derives three keys (encryption, index, signing) from a single shared secret. Same secret + same config salt = same keys = shared memory across agents.

### Why CRDT-style index merge?
Multiple agents can write concurrently. The encrypted index is a map of entry type → sorted list of entries, merged by timestamp. No locking, no coordination needed.

### Why shared-memory-skill as an OpenCode skill?
Enables in-process tool calling for OpenCode agents. Universal CLI tool (`agent-memory skill tool`) ensures any other framework can participate with just shell access.
