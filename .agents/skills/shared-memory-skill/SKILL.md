---
name: shared-memory-skill
description: Join a shared encrypted memory space using a shared secret. Exposes memory_write, memory_read, memory_list, and memory_session tools.
metadata:
  secret_arg: shared_secret
  session_required: true
---

# shared-memory-skill

A skill that allows any LLM client to join a shared encrypted memory space by presenting a shared secret. Works via OpenCode framework handlers (for in-process use) or via CLI (for any agent framework via shell commands).

## interpret:
  framework: opencode
  handlers:
    memory_write: shared_memory.HandleWrite
    memory_read: shared_memory.HandleRead
    memory_list: shared_memory.HandleList
    memory_session: shared_memory.HandleSession
  session_initializer: shared_memory.InitSession
  session_closer: shared_memory.CloseSession

## Universal CLI Interface

For any LLM framework that can execute shell commands:

```bash
# Call a tool (secret passed per-call, no session persistence needed)
agent-memory skill tool <tool_name> [json_params] --secret <shared_secret>

# Examples:
agent-memory skill tool memory_session --secret mysecret
agent-memory skill tool memory_write '{"content": "决策：使用 Go", "type": "decision"}' --secret mysecret
agent-memory skill tool memory_read '{"type": "decision", "limit": 5}' --secret mysecret
agent-memory skill tool memory_list '{"tags": ["api"]}' --secret mysecret
```

### Environment Variables
- `AGENT_MEMORY_SECRET` — default secret (use `--secret` to override)
- `AGENT_MEMORY_IPFS_ADDR` — IPFS daemon address (default `http://localhost:5001`)
- `AGENT_MEMORY_SESSION_ID` — session ID for multi-agent coordination (default `default`)

## OpenCode Framework Usage

```
/skill load shared-memory-skill --secret <shared_secret> [--source <source>]
```

- `shared_secret` (required): Passphrase or hex-encoded secret. Same secret + same config salt = same derived keys = shared memory access.
- `source` (optional): Identifier for this agent (e.g., "gpt-4", "claude-3"). Defaults to "opencode-agent".

## Tools

### memory_write

Write a new encrypted memory entry to the shared IPFS-backed store. The entry is encrypted with AES-256-GCM, pinned to IPFS, and added to the encrypted index. All agents using the same shared secret can read and merge this entry.

```json
{
  "name": "memory_write",
  "description": "Write a new encrypted memory entry to the shared IPFS-backed store. The entry is encrypted with AES-256-GCM, pinned to IPFS, and added to the encrypted index. All agents using the same shared secret can read and merge this entry.",
  "parameters": {
    "type": "object",
    "properties": {
      "content": {
        "type": "string",
        "description": "The main content of the memory entry. Free-text, up to tens of kilobytes."
      },
      "type": {
        "type": "string",
        "enum": ["decision", "learning", "trace", "observation", "blocker", "context"],
        "description": "The kind of memory entry. Used for filtering and CRDT merge."
      },
      "tags": {
        "type": "array",
        "items": { "type": "string" },
        "description": "Optional tags for categorizing the entry. Normalized to lowercase, sorted, deduplicated."
      },
      "source": {
        "type": "string",
        "description": "Optional override for the source identifier. Defaults to the session's source."
      }
    },
    "required": ["content", "type"]
  }
}
```

### memory_read

Read and decrypt memory entries matching the given filters. Entries are sorted by timestamp descending (newest first). Entries written by agents with different keys will fail to decrypt and are skipped with a warning.

```json
{
  "name": "memory_read",
  "description": "Read and decrypt memory entries matching the given filters. Entries are sorted by timestamp descending (newest first). Entries written by agents with different keys will fail to decrypt and are skipped with a warning.",
  "parameters": {
    "type": "object",
    "properties": {
      "type": {
        "type": "string",
        "enum": ["decision", "learning", "trace", "observation", "blocker", "context"],
        "description": "Filter by entry type."
      },
      "tags": {
        "type": "array",
        "items": { "type": "string" },
        "description": "Filter by tags (entry must have all listed tags)."
      },
      "source": {
        "type": "string",
        "description": "Filter by source identifier."
      },
      "since": {
        "type": "string",
        "description": "ISO 8601 / RFC 3339 timestamp. Return entries newer than this time."
      },
      "limit": {
        "type": "integer",
        "description": "Maximum number of entries to return. Default 10, max 100.",
        "default": 10
      }
    },
    "required": []
  }
}
```

### memory_list

List entries from the encrypted index without decrypting content. Returns IndexEntry records (id, cid, type, tags, timestamp, source, content_preview). Use this to explore what entries exist before reading full content.

```json
{
  "name": "memory_list",
  "description": "List entries from the encrypted index without decrypting content. Returns IndexEntry records (id, cid, type, tags, timestamp, source, content_preview). Use this to explore what entries exist before reading full content.",
  "parameters": {
    "type": "object",
    "properties": {
      "type": {
        "type": "string",
        "enum": ["decision", "learning", "trace", "observation", "blocker", "context"],
        "description": "Filter by entry type."
      },
      "tags": {
        "type": "array",
        "items": { "type": "string" },
        "description": "Filter by tags (entry must have all listed tags)."
      },
      "source": {
        "type": "string",
        "description": "Filter by source identifier."
      },
      "since": {
        "type": "string",
        "description": "ISO 8601 / RFC 3339 timestamp. Return entries newer than this time."
      },
      "limit": {
        "type": "integer",
        "description": "Maximum number of entries to return. Default 10, max 100.",
        "default": 10
      }
    },
    "required": []
  }
}
```

### memory_session

Return the current session status including whether a session is active, the IPFS address, entry count from the index, and the configured source identifier.

```json
{
  "name": "memory_session",
  "description": "Return the current session status including whether a session is active, the IPFS address, entry count from the index, and the configured source identifier.",
  "parameters": {
    "type": "object",
    "properties": {},
    "required": []
  }
}
```

## Entry Types

- `decision` — Significant choices or conclusions
- `learning` — Insights or knowledge gained
- `trace` — Execution traces or step-by-step records
- `observation` — Noticed facts or events
- `blocker` — Impediments or blockers
- `context` — Contextual background information

## Error Handling

| Condition | Message |
|---|---|
| Missing/empty secret | "A shared secret is required. Provide it at skill-load time." |
| Key derivation failure | "Key derivation failed — verify the shared secret is correct." |
| Config file not found | "agent-memory config not found at ~/.config/agent-memory/config.json" |
| IPFS unreachable | "IPFS daemon unreachable at {addr}. Start the daemon and retry." |
| Entry write failure | "Failed to pin entry: {cause}. Entry may not be persisted." |

**Security**: No error message, log line, or tool result field contains the secret, derived keys, or any cryptographic key material.

## Session Lifecycle

Session initialization (InitSession):
1. Validate secret is non-empty
2. Load config from ~/.config/agent-memory/config.json
3. Derive keys via HKDF-SHA256 (config.GetKeys)
4. Create store (store.New)
5. Verify IPFS connectivity (ipfs.Ping)
6. Store session in skill state

Session close (CloseSession):
1. Call store.Close() (closes IPFS client)
2. Remove session from map
3. Keys eligible for garbage collection
