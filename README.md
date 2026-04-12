# agent-memory

> E2E encrypted agent memory on IPFS — portable, decentralized knowledge persistence

Agents (Goose, Copilot, Claude Code, observation loops) generate institutional knowledge — decisions, failures, learnings, context. That knowledge currently dies with the session. `agent-memory` fixes this by providing encrypted, content-addressed, decentralized persistence that any agent can write to and read from.

## Why

- Billing integration has been attempted 3 times. Zero traces of those attempts exist in any memory system.
- Observation loop runs 65 times, sees GitHub state, but cannot see decisions made outside GitHub.
- m4 vault has 523 traces, 485 essences — rich personal memory, but agents can't write to it.
- The pipeline's `collect-memory.sh` can only surface what's already been captured.

**The gap**: work happens in sessions. Sessions end. Knowledge evaporates. Next session starts from zero.

## How it works

```
Agent session → write(memory_entry) → encrypt(agent_key) → pin(IPFS) → CID
                                                                      ↓
Next session → read(CID) → decrypt(agent_key) → memory_entry → agent context
```

1. **Agent writes a memory entry** (JSON: type, content, tags, timestamp, source)
2. **Entry is encrypted** with AES-256-GCM using a key derived from the agent's secret (HKDF, same pattern as wgmesh)
3. **Encrypted blob is pinned to IPFS** via a local Kubo node or Helia (browser/runtime)
4. **CID is the address** — content-addressed, immutable, decentralizable
5. **An index** (also encrypted) maps tags/types → CIDs so agents can query by topic
6. **Any agent with the key** can decrypt and read. Without the key, it's opaque ciphertext on IPFS.

## Design principles

1. **Portable** — CID is universal. Doesn't matter if you're on Kubo, Helia, or pinning.service.io
2. **Zero infrastructure** — runs against a local IPFS node. No servers. No databases. No SaaS.
3. **Agent-agnostic** — Goose, Copilot, Claude Code, bash scripts, any HTTP client
4. **E2E encrypted** — IPFS sees ciphertext. Only keyholders can read. Same HKDF+AES-256-GCM pattern as wgmesh.
5. **Content-addressed** — entries are immutable. New version = new CID. Index tracks the latest.
6. **Write-heavy, read-light** — agents write after sessions, read at session start. Not a real-time database.

## CLI

```bash
# Initialize — creates agent key, connects to IPFS
agent-memory init --secret "my-agent-secret"

# Write a memory entry
agent-memory write --type decision --tag "billing,vendor" \
  --content "Evaluated LemonSqueezy vs Stripe. LemonSqueezy lacks webhook reliability. Going with Stripe for billing v3."

# Query memories
agent-memory read --tag billing --limit 10
agent-memory read --type decision --since 2026-03-01

# List all entries (metadata only, decrypted)
agent-memory list --tag billing

# Pin management
agent-memory pins          # list all pinned CIDs
agent-memory gc            # garbage collect unpinned
```

## Architecture

See [SPEC.md](./SPEC.md) for full technical specification.

## Status

Pre-alpha. Spec in progress.
