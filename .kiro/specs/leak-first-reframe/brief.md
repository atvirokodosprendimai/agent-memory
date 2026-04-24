# Brief: leak-first-reframe

## Problem

The v1 design (`SPEC.md`, `README.md`) treats agent-memory as a private-by-default, E2E-encrypted store gated by a long-lived shared secret (`AGENT_MEMORY_SECRET` → HKDF → encryption / index / signing keys). That frame is wrong for the actual workload.

The actual workload is:

- **Decisions**: "We picked Stripe over LemonSqueezy because…"
- **Architecture records (ADRs)**: context / decision / consequences, Nygard-style
- **Pointers**: "The rate-limiter lives at `wgmesh/internal/ratelimit/token_bucket.go:47`, referenced by ADR-0012"

None of these are secrets. All of them are things we want future agents, future humans, future auditors to read. Trying to keep them private is both unnecessary and expensive (key distribution, rotation, loss risk).

Operationally we must also assume **every byte written will eventually leak**: keys get committed, CI logs get indexed, laptops get stolen, drand rounds roll over, court orders arrive. Confidentiality as a design goal is a lie. Designing as if it holds produces bad writer hygiene and false comfort.

## Current State

- `SPEC.md` §4 defines HKDF-SHA256 from a user secret into three keys; §2 centers the value prop on "E2E encrypted"; §10 threat model lists `IPFS node sees plaintext` as the top threat with encryption as mitigation.
- `internal/config/config.go` stores `SaltHex` + loads a secret from env to re-derive keys. `GetKeys` returns a `Keys{EncryptionKey, IndexKey, SigningKey}`.
- `internal/crypto/` implements AES-256-GCM seal/open and HMAC-SHA256. Every `store.Write` encrypts content. Every `store.Read` decrypts.
- `cmd/agent-memory rotate` exists explicitly to rotate the long-lived secret and re-encrypt all entries — a feature that only makes sense if confidentiality is the goal.
- `Entry.Type` enum has 6 loose values; `trace`, `observation`, `note`-ish types dominate usage and absorb content that was never meant to be durable.

## Desired Outcome

A store whose design contract is:

1. **Public-by-default.** An entry, once written, is meant to be readable forever by anyone who finds its CID. Writers write accordingly.
2. **Signed, not hidden.** Authenticity and provenance (who said what, when) survive every leak and every republication. Signatures are mandatory; confidentiality is opt-in.
3. **Encryption = time-shift, not secrecy.** Encrypted entries have an explicit unseal condition (timelock round, event-triggered key release). When the condition fires, the entry is public forever. There is no "encrypted forever" path.
4. **Writer hygiene is a hard contract.** The CLI refuses to write anything that looks like a secret, credential, or PII. Overriding requires an explicit acknowledgement that the value will leak.
5. **Schema narrows to intent.** First-class types: `decision`, `adr`, `pointer`. Everything else is `note` (explicitly best-effort).

## Approach

### Entry classes at write time

| Class | Visibility | Mechanism |
|---|---|---|
| `public` | immediate, permanent | plaintext IPLD on IPFS |
| `embargo:timelock` | public after round/time T | tlock (drand-based timelock encryption) |
| `embargo:event` | public when holder publishes key | ephemeral X25519 keypair, private half released as a later entry |
| `never` | rejected at write path | writer-hygiene linter blocks |

### Crypto surface

- **Mandatory**: Ed25519 signature over `(content || embargo_metadata || timestamp || author_pubkey)`.
- **Optional**: tlock or symmetric seal for embargoed entries only.
- **Removed**: long-lived shared secret, HKDF three-key derivation, `AGENT_MEMORY_SECRET` as an access gate, `rotate` command.
- **Identity**: per-agent Ed25519 keypair stored at `~/.config/agent-memory/identity.json`. Public key doubles as author identifier in every entry and in the index.

### Writer-hygiene linter

Pre-`Write` scan against a built-in ruleset:

- High-entropy token patterns (`AKIA…`, `sk-…`, `ghp_…`, `xoxb-…`)
- `BEGIN … PRIVATE KEY` blocks
- `Authorization: Bearer …`, `password=`, `token=` forms
- Email + credit-card shaped strings

Hit → hard refuse with exit code. `--acknowledge-leak` flag allows override and writes a second `note`-type entry recording the acknowledgement.

### Schema

```json
{
  "id": "sha256-of-signed-payload",
  "type": "decision|adr|pointer|note",
  "author_pubkey": "ed25519:…",
  "signature": "…",
  "timestamp": "2026-04-24T12:00:00Z",
  "tags": ["billing", "vendor"],
  "embargo": {
    "class": "public|timelock|event",
    "unsealed_at": "2026-06-01T00:00:00Z",
    "releaser_pubkey": "ed25519:…",
    "beacon_chain": "…",
    "beacon_round": 4523512
  },
  "content": {
    "...": "type-specific payload"
  }
}
```

Type-specific `content`:

- `decision`: `{ question, chosen, alternatives_considered[], rationale, links[] }`
- `adr`: `{ number, title, status, context, decision, consequences, supersedes?, superseded_by? }`
- `pointer`: `{ target, kind: "url"|"cid"|"repo_path"|"adr_ref", why }`
- `note`: `{ body }` — freeform, de-emphasized

### Index

- Public (plaintext IPLD).
- Carries `UnsealedAt` per entry; readers can filter `--only-unsealed` / `--include-sealed`.
- Signature required on the index root too; each publish is author-attributable.

## Scope

**In**:
- New `internal/identity/` package: Ed25519 keypair, load/save, sign/verify helpers.
- New `internal/hygiene/` package: secret-pattern linter + rule bank.
- New `internal/embargo/` package: tlock wrapper (drand integration) + event-embargo key mint/release.
- Schema change in `internal/store/`: new `Entry` + `IndexEntry` shapes with `Embargo`, `AuthorPubkey`, `Signature`. Plaintext index path.
- CLI changes:
  - Add: `identity`, `release <cid>`, `verify <cid>`, `lint <stdin>`.
  - Remove: `rotate`, `--secret` long-lived flag, `AGENT_MEMORY_SECRET` gate.
  - Modify: `init` produces identity + config pointing at a drand chain; `write` takes `--embargo` and runs hygiene linter; `read`/`list` expose `unsealed_at` and signature status.
- `SPEC.md` v2 replacing v1, and a migration note.
- ADR entries in the new store that record this reframe itself — dogfood on day one.

**Out**:
- Backwards compatibility with v1 encrypted entries (clean break — see "Open Questions" on migration).
- Access control / revocation beyond what signatures + tombstones naturally give.
- Key recovery for lost identity keys (user responsibility; losing it loses write authority, not read access).
- Any attempt to prevent leak of plaintext entries (explicitly out of threat model).

## Boundary Candidates

1. **Identity** (`internal/identity/`): keypair on disk, sign/verify, author-pubkey as stable ID.
2. **Hygiene linter** (`internal/hygiene/`): rule bank, scanner, hard-refuse API.
3. **Embargo** (`internal/embargo/`): drand tlock client, event-embargo mint/release.
4. **Store schema v2** (`internal/store/`): entry/index shapes, plaintext index, signature verification in `Read`.
5. **CLI surface** (`cmd/agent-memory/`): new / removed / modified subcommands.

## Out of Boundary

- `internal/p2p/` and `internal/ipfs/` transport layer — still valid, still behind `StorageClient`.
- CRDT `Index.Merge` logic — still correct, operates on plaintext now (simpler).
- Tamarin proofs under `security/` — untouched; still cover the P2P transport.
- `shared-memory-skill` — touched only where schema/flags change.

## Upstream / Downstream

- **Upstream**: `github.com/drand/tlock` (timelock encryption), `crypto/ed25519` (stdlib), `filippo.io/age` (considered for event-embargo symmetric seal).
- **Downstream**: every existing agent/integration that writes via `AGENT_MEMORY_SECRET` — forced migration.

## Existing Spec Touchpoints

- **Supersedes**: core premise of `SPEC.md` v1 (private-by-default, shared-secret gated).
- **Unaffected**: `p2p-transport`, `crdt-index` (both operate below this layer).
- **Affects**: `shared-memory-skill`, `helia-backend` — session lifecycle no longer keyed by secret; keyed by identity instead.

## Constraints

- Drand chain hash + bootstrap URLs must be explicit in config; no silent defaults.
- Identity keypair generation must happen exactly once per machine, via `init`. No auto-generation on first write.
- Linter rule bank is version-controlled in-repo and updatable without code change (YAML or JSON).
- `--acknowledge-leak` must not be the path of least resistance — it should be painful enough that writers classify correctly first.
- Mandatory signing means loss of the identity key loses the ability to *write* but does not lose anything already written. Read remains open.

## Rejected Alternatives

1. **Always-encrypted + better key management** — does not change the fact that leak is inevitable; just adds complexity while preserving a false premise.
2. **Per-reader encryption (age recipients)** — scales badly, requires maintaining recipient lists, still fights inevitability.
3. **Shamir secret sharing for N-of-M unseal** — possible mechanism for `embargo:event` but adds operational burden; defer to a later spec if demand appears.
4. **Keeping v1 schema + adding a `sensitive` flag** — hides the reframe under a boolean; writers will default-flag everything and we're back to v1.

## Open Questions

1. **Migration path for existing v1 entries** — clean break (tombstone all, do not port) or one-time `import --from-v1` that verifies the old secret and re-emits as plaintext signed entries?
2. **Drand chain selection** — League of Entropy mainnet (30s round) or quicknet (3s round)? Document the trade-off but pick a default.
3. **Pointer immutability** — `pointer.target=repo_path` points at a mutable file. Snapshot the referenced content (hash + excerpt) at write time, or accept rot?
4. **Hygiene linter false positives** — what's the escape hatch for legitimate high-entropy content (CIDs, hashes, public keys)? Whitelist by prefix, or mandatory `--acknowledge-leak` + justification?
5. **Identity rotation** — if an identity key is suspected compromised, new identity is trivial, but old entries remain signed by the old key. Publish a `revocation` entry signed by old key declaring it retired? Treat as out of scope for v2?
