# Project Structure

## Organization Philosophy

**Layered internal packages**: `cmd/` depends on `internal/`, `internal/` packages depend on no other internal packages (only standard library + `golang.org/x/crypto`). Each `internal/` package is a single concern.

## Directory Patterns

### CLI entry point
**Location**: `cmd/agent-memory/`  
**Purpose**: Main binary. `main.go` handles signal propagation, `commands.go` dispatches subcommands (`init`, `write`, `read`, `list`, `pins`, `gc`, `skill`).  
**Example**: `main.go` uses a `switch` on `os.Args[1]` to dispatch commands. Handlers live in `commands.go`.

### Skill integration
**Location**: `cmd/agent-memory/skills/shared_memory/`  
**Purpose**: OpenCode skill implementation. `session.go` manages `SkillState` (sessions map, InitSession, CloseSession), `tools.go` provides the four tool handlers (`HandleWrite`, `HandleRead`, `HandleList`, `HandleSession`).  
**Pattern**: Skill state is session-scoped (map of secret → store). Each tool call is stateless — secret passed per-call.

### Config package
**Location**: `internal/config/`  
**Purpose**: Loads `~/.config/agent-memory/config.json`, derives keys via HKDF-SHA256. Single `Config` struct with `GetKeys()` method. No global state.  
**Pattern**: `config.Load()` reads the config file; `GetKeys()` derives encryption, index, and signing keys.

### Crypto package
**Location**: `internal/crypto/`  
**Purpose**: Thin wrapper around `crypto/aes` and `crypto/cipher` for AES-256-GCM. Also provides HMAC-SHA256. Stateless functions.  
**Pattern**: Functions like `Seal`, `Open`, `Hash` — no stateful struct.

### IPFS client
**Location**: `internal/ipfs/`  
**Purpose**: HTTP RPC client for Kubo. `Client` struct wraps `*http.Client`. All methods return typed errors.  
**Pattern**: `NewClient(addr string) *Client`, `Ping`, `Add`, `Cat`, `Pin`, `Unpin`, `Pins`.

### Store package
**Location**: `internal/store/`  
**Purpose**: High-level encrypted storage. `Store` struct holds IPFS client, crypto keys, and encrypted index. `Write` encrypts and pins entries. `Read` retrieves and decrypts.  
**Pattern**: `New(cfg, secret) (*Store, error)`, `Write`, `Read`, `GC`, `Close`. `Filter` struct for query parameters.

## Naming Conventions

- **Files**: Lowercase, underscore-separated (`store.go`, `store_test.go`, `commands_test.go`)
- **Packages**: Lowercase, single word or underscore (`config`, `crypto`, `ipfs`, `store`)
- **Types**: PascalCase (`Store`, `Entry`, `Filter`, `EntryType`)
- **Functions**: PascalCase, verb-first (`Write`, `Read`, `GC`, `Close`, `Seal`, `Open`)
- **Constants**: PascalCase or camelCase for simple cases (`TypeDecision`, `maxStdinSize`)
- **Interfaces**: PascalCase, often `-er` suffix or descriptive (`StoreInterface`, `IPFSClient`)

## Import Organization

Go import paths are the module-absolute paths:
```go
import (
    "fmt"                              // stdlib
    "github.com/atvirokodosprendimai/agent-memory/internal/config"
    "github.com/atvirokodosprendimai/agent-memory/internal/store"
)
```

No path aliases needed. Standard library imports grouped separately from internal packages.

## Code Organization Principles

- **Package boundary**: Each internal package is a single logical domain (config, crypto, ipfs, store)
- **Interface for dependencies**: `StoreInterface` in `commands.go` lets tests inject a fake store
- **Error handling**: All errors wrapped with `fmt.Errorf("context: %w", err)` — no bare sentinel errors
- **No global state**: Config loaded per-command, store created per-session, no `init()` functions
- **Command dispatch**: `commands.go` uses a `switch os.Args[1]` pattern in `main()`; each case calls a `run*` function
- **Flag parsing**: Standard `flag` package; subcommand args parsed manually (not via `flag.FlagSet`) to handle positional args before flags
