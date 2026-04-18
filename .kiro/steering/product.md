# Product Overview

agent-memory provides E2E encrypted, content-addressed, decentralized memory persistence for AI agents. Any agent (Goose, Copilot, Claude Code, custom) can write encrypted knowledge that any other agent with the correct key can read. IPFS serves as the storage backbone, making the knowledge portable and decentralized.

## Core Capabilities

- **Encrypted write**: Agent writes a memory entry (type, content, tags, source) → AES-256-GCM encryption → IPFS pin → CID returned. Only keyholders decrypt.
- **Filtered read**: Query by entry type, tags, source, or timestamp. Entries sorted by timestamp descending.
- **Shared secret sessions**: Multiple agents share a secret → same HKDF-SHA256 key derivation → same derived keys → shared memory access.
- **Universal CLI tool interface**: Any LLM framework can call memory tools via shell (`agent-memory skill tool <tool> --secret <secret>`), no framework lock-in.
- **IPFS-backed durability**: Content-addressed storage means entries are immutable and independently addressable. Index tracks latest CIDs.

## Target Use Cases

- **Multi-agent coordination**: Agents share decisions, learnings, and context via a common encrypted store.
- **Session continuity**: Knowledge generated in one session persists into the next without SaaS dependency.
- **Agent-agnostic memory**: Any agent or script that knows the shared secret can participate in the same memory space.
- **Decentralized knowledge**: Memory survives agent restarts, machine changes, or SaaS outages — it's on IPFS.

## Value Proposition

- **Zero infrastructure**: Runs against a local IPFS node. No servers, no databases, no SaaS accounts.
- **Agent-agnostic**: Shell-accessible CLI means it works with any agent framework.
- **E2E encrypted**: IPFS sees only ciphertext. Key material never leaves the agent.
- **Portable**: CID is universal — works with Kubo, Helia, or pinning.services.
- **Write-heavy, read-light**: Optimized for agents writing after sessions and reading at session start.
