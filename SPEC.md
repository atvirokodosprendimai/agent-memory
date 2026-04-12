# agent-memory — Technical Specification

> Version: 0.1.0-draft | Date: 2026-04-12

## 1. Problem

Agents (AI coding assistants, observation loops, CI/CD pipelines) generate institutional knowledge during sessions. When the session ends, that knowledge is lost. The next session — or the next agent — starts from zero.

**Concrete example**: Billing integration for wgmesh has been attempted ~3 times across multiple sessions. Zero traces of what was tried, what failed, or what was decided exist in any memory system. Each attempt starts from scratch.

**Current state**:
- `m4` (personal vault): 523 traces, 485 essences — rich but only human-curated
- `ai-pipeline-template/memory/`: pipeline brain — only captures observation loop output
- `collect-memory.sh`: surfaces existing memory to LLM — can't create new memory
- **No agent can write institutional memory that persists across sessions**

## 2. Solution

A Go CLI (`agent-memory`) that provides encrypted, content-addressed memory persistence on IPFS.

### Data flow

```
┌─────────────┐     ┌──────────────┐     ┌──────────┐     ┌────────────┐
│ Agent        │────▶│ agent-memory  │────▶│ Encrypt   │────▶│ IPFS Pin   │
│ (goose, etc) │     │ CLI / API     │     │ AES-GCM   │     │ Kubo/Helia │
└─────────────┘     └──────────────┘     └──────────┘     └────────────┘
                                                │                │
                                           agent key          CID
                                                │                │
┌─────────────┐     ┌──────────────┐     ┌──────▼────────┐     │
│ Agent        │◀────│ agent-memory  │◀────│ Decrypt       │◀────┘
│ (next run)   │     │ read / list   │     │ AES-GCM       │
└─────────────┘     └──────────────┘     └───────────────┘
```

### Key insight: reuse wgmesh crypto patterns

The wgmesh codebase already has production E2E encryption:
- `pkg/crypto/derive.go`: HKDF key derivation from shared secret
- `pkg/crypto/envelope.go`: AES-256-GCM seal/open with nonces

agent-memory uses the same pattern. Agent secret → HKDF → encryption key. Same team, same trust model, same cryptographic rigor.

## 3. Data model

### 3.1 Memory entry

```json
{
  "id": "sha256-of-decrypted-content",
  "type": "decision | learning | trace | observation | blocker | context",
  "source": "goose | copilot | claude-code | observation-loop | human",
  "timestamp": "2026-04-12T11:05:00Z",
  "tags": ["billing", "vendor-evaluation", "stripe"],
  "content": "Evaluated LemonSqueezy vs Stripe for billing v3. LemonSqueezy webhooks dropped 3/10 test events. Stripe's test mode is reliable. Decision: use Stripe Checkout for initial integration, defer Stripe Billing portal to post-first-customer.",
  "metadata": {
    "session_id": "optional-session-ref",
    "repos": ["wgmesh", "ai-pipeline-template"],
    "funnel_stage": 1,
    "related_cids": ["Qm..."]
  },
  "version": 1
}
```

### 3.2 Encrypted blob (what goes on IPFS)

```
[12 bytes nonce][N bytes AES-256-GCM ciphertext + 16 byte tag]
```

Raw binary. No JSON wrapper. The nonce is prepended for self-contained decryption.

### 3.3 Index (also encrypted, also on IPFS)

```json
{
  "version": 1,
  "updated": "2026-04-12T11:05:00Z",
  "entries": [
    {
      "id": "sha256-...",
      "cid": "Qm...",
      "type": "decision",
      "tags": ["billing", "vendor-evaluation"],
      "timestamp": "2026-04-12T11:05:00Z",
      "source": "human",
      "content_preview": "first 120 chars of decrypted content..."
    }
  ]
}
```

The index CID is stored locally in `~/.config/agent-memory/index.json` (unencrypted — it only contains CIDs and metadata previews, not content). The actual encrypted index blob lives on IPFS.

### 3.4 Entry types

| Type | Purpose | Example |
|------|---------|---------|
| `decision` | A choice made and why | "Using Stripe over LemonSqueezy for v3" |
| `learning` | Something discovered | "LemonSqueezy webhooks drop events under load" |
| `trace` | Raw session context | "Billing session #3: explored Paddle, LemonSqueezy, Stripe" |
| `observation` | State noticed | "No billing code exists anywhere in any repo" |
| `blocker` | Something preventing progress | "No payment provider selected yet" |
| `context` | Background for future sessions | "Billing v1 was LemonSqueezy abandoned after..." |

## 4. Cryptography

### 4.1 Key derivation

```
agent_secret (user-provided, >= 16 chars)
    │
    ▼ HKDF-SHA256 (salt: random 32 bytes, stored in config)
    │
    ├──▶ encryption_key (32 bytes) — AES-256-GCM key for entries
    ├──▶ index_key (32 bytes) — AES-256-GCM key for index
    └──▶ signing_key (32 bytes) — HMAC-SHA256 for entry ID computation
```

Domain separation via HKDF info strings:
- `agent-memory-encryption-v1`
- `agent-memory-index-v1`
- `agent-memory-signing-v1`

### 4.2 Encryption

- AES-256-GCM (same as wgmesh `SealEnvelope`/`OpenEnvelope`)
- 12-byte random nonce per entry (prepended to ciphertext)
- 16-byte authentication tag (appended by GCM)
- No associated data (entries are independent)

### 4.3 Entry ID

```
HMAC-SHA256(signing_key, type + tags.join(",") + content + timestamp)
```

Deterministic — same content at same time = same ID. Enables dedup.

## 5. IPFS integration

### 5.1 Requirements

- Local Kubo node (go-ipfs) running at `localhost:5001` (RPC API)
- Fallback: HTTP gateway for read-only (e.g., `ipfs.io`)
- Future: Helia (JS/browser), pinning services (web3.storage, nft.storage)

### 5.2 Pinning strategy

- Every encrypted entry is pinned (recursive)
- Index blob is pinned (recursive)
- Local pin ensures persistence even without remote pinning
- Optional: configure remote pinning service for redundancy

### 5.3 Garbage collection

- `agent-memory gc` removes pins for entries older than `--max-age`
- Index is rebuilt after GC
- Default: no GC (keep everything — storage is cheap, per the m4 trace)

## 6. CLI specification

### 6.1 Commands

```
agent-memory init [--secret SECRET] [--ipfs-addr ADDR]
    Creates ~/.config/agent-memory/config.json
    Derives keys from secret (or prompts)
    Tests IPFS connection
    Creates empty encrypted index

agent-memory write --type TYPE --tag TAGS [--source SOURCE] --content CONTENT
    Creates entry, encrypts, pins to IPFS
    Updates index
    Prints CID

agent-memory read [--tag TAG] [--type TYPE] [--since DATE] [--limit N] [--raw]
    Decrypts and prints matching entries
    --raw: print full JSON including metadata
    Default: pretty-print content + tags + timestamp

agent-memory list [--tag TAG] [--type TYPE] [--since DATE]
    Prints entry metadata from index (no decryption needed)
    Faster than read — only hits local index file

agent-memory pins
    Lists all CIDs pinned by agent-memory

agent-memory gc [--max-age DURATION]
    Unpins entries older than duration, rebuilds index

agent-memory export [--tag TAG] [--type TYPE] --output FILE
    Decrypts and exports matching entries as JSONL

agent-memory import --input FILE [--source SOURCE]
    Imports entries from JSONL, encrypts, pins
```

### 6.2 Configuration

`~/.config/agent-memory/config.json`:

```json
{
  "version": 1,
  "ipfs_addr": "http://localhost:5001",
  "salt_hex": "random 32 bytes hex-encoded",
  "index_cid": "Qm... (CID of current encrypted index)",
  "created": "2026-04-12T11:05:00Z"
}
```

The secret is NOT stored. It's provided via:
1. `--secret` flag
2. `AGENT_MEMORY_SECRET` env var
3. Prompt (interactive mode)

## 7. Integration with existing systems

### 7.1 Observation loop (ai-pipeline-template)

The observation loop's `collect-memory.sh` adds a new source:

```bash
# In collect-memory.sh, add:
if command -v agent-memory &>/dev/null; then
  echo "## Agent Memory" >> "$OUTPUT"
  agent-memory read --limit 5 --since "$(date -d '7 days ago' '+%Y-%m-%d')" >> "$OUTPUT" 2>/dev/null || true
fi
```

### 7.2 Goose sessions

A goose session can write memory at session end:

```bash
# Post-session hook or manual:
agent-memory write --type trace --tag "$TOPIC" --source goose \
  --content "$(cat /tmp/session_summary.txt)"
```

### 7.3 CI/CD (GitHub Actions)

```yaml
# In observation-loop.yml, after assessment:
- name: Write agent memory
  env:
    AGENT_MEMORY_SECRET: ${{ secrets.AGENT_MEMORY_SECRET }}
  run: |
    agent-memory write --type observation --tag "loop-assessment" \
      --source observation-loop --content "$(jq -r '.assessment' /tmp/assessment.json)"
```

## 8. Implementation plan

### Phase 0: Scaffold (today)
- [x] README.md, SPEC.md
- [ ] Go module init, cmd/ structure
- [ ] `agent-memory init` — key derivation, config, IPFS test

### Phase 1: Core (this week)
- [ ] `agent-memory write` — encrypt + pin
- [ ] `agent-memory read` — fetch + decrypt
- [ ] `agent-memory list` — index query
- [ ] Index management (encrypted index on IPFS)

### Phase 2: Integration (next week)
- [ ] `collect-memory.sh` integration
- [ ] Observation loop writes assessments as memories
- [ ] Goose post-session hook

### Phase 3: First real payload
- [ ] Capture billing v1/v2 context from human memory (before it's gone)
- [ ] Write billing vendor evaluation as first real entries
- [ ] Verify observation loop picks up billing context in next run

## 9. Why IPFS (not S3, not SQLite, not a database)

1. **Zero infrastructure** — local Kubo node, no server to maintain, no S3 bucket to configure
2. **Content-addressed** — immutable entries, dedup by CID, no overwrite accidents
3. **Portable** — CID works everywhere. Move the index between machines, pin to remote services, share (encrypted) with collaborators
4. **Decentralized** — aligns with wgmesh philosophy. No single point of failure.
5. **Egress economics** — IPFS pins locally, replicates to peers. No egress costs for reads.
6. **Encryption at rest** — IPFS stores ciphertext. No trust in storage provider needed.

## 10. Threat model

| Threat | Mitigation |
|--------|------------|
| IPFS node sees plaintext | E2E encryption — node only sees ciphertext |
| Key compromise | Secret is never stored. Derive from user-provided secret. |
| Index leak (local file) | Index contains only CIDs and 120-char previews, not full content |
| CID discoverability | CIDs are not guessable. Attacker must know the CID to fetch. |
| Replay / tampering | AES-GCM provides authentication. Tampered ciphertext fails decryption. |
| Loss of IPFS node | Pin to remote service for redundancy. CIDs survive node loss if replicated. |

## 11. Open questions

1. **Remote pinning** — which service? web3.storage? nft.storage? Self-hosted?
2. **Multi-agent keys** — should different agents have different keys? Or one key per project?
3. **Index synchronization** — if two agents write simultaneously, index conflicts. Merge strategy?
4. **Content size limits** — max entry size? Streaming for large entries?
5. **Search** — tag-based filtering is basic. Full-text search over encrypted data is hard. Acceptable for v1?
