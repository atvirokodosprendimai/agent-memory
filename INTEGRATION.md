# Agent Memory Integration Guide

## How to connect all GitHub repos and actions to agent-memory.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     IPFS (content-addressed)                │
│                                                             │
│   ┌──────────┐  ┌──────────┐  ┌──────────┐                │
│   │ encrypted│  │ encrypted│  │ encrypted│  ...            │
│   │ entry #1 │  │ index    │  │ entry #N │                │
│   └──────────┘  └──────────┘  └──────────┘                │
│         ▲             ▲                                     │
└─────────┼─────────────┼─────────────────────────────────────┘
          │             │
          │     ┌───────┴───────┐
          │     │ config.json   │  ← tracks index CID
          │     │ ~/.config/    │
          │     └───────────────┘
          │
    ┌─────┴──────────────────────────────────────────────────┐
    │              agent-memory CLI                           │
    │  write | read | list | init | pins                     │
    └──┬──────────┬──────────┬──────────┬────────────────────┘
       │          │          │          │
       ▼          ▼          ▼          ▼
┌──────────┐ ┌────────┐ ┌────────┐ ┌──────────────────┐
│ observ.  │ │ health │ │ custom │ │ any repo's       │
│ loop     │ │ check  │ │ action │ │ collect-memory.sh│
│ (writes) │ │ (writes│ │ (r/w)  │ │ (reads → /tmp/)  │
└──────────┘ └────────┘ └────────┘ └──────────────────┘
```

## Integration Points

### 1. Observation Loop → agent-memory (WRITER)

The observation loop already writes episodic memory to `memory/episodic/`.
We add a **parallel write** to agent-memory so every assessment is also
stored encrypted on IPFS.

**In `.github/workflows/observation-loop.yml`**, add after the "Write episodic memory" step:

```yaml
      - name: Write to agent-memory
        env:
          AGENT_MEMORY_SECRET: ${{ secrets.AGENT_MEMORY_SECRET }}
        run: |
          # Install agent-memory (download release binary)
          curl -sL https://github.com/atvirokodosprendimai/agent-memory/releases/latest/download/agent-memory-linux-amd64 \
            -o /usr/local/bin/agent-memory && chmod +x /usr/local/bin/agent-memory

          # Initialize if not already done (idempotent)
          agent-memory init 2>/dev/null || true

          # Extract assessment data
          stage=$(jq -r '.stage_name // "Unknown"' /tmp/assessment.json)
          narrative=$(jq -r '.assessment // "No assessment."' /tmp/assessment.json)
          blockers=$(jq -r '(.blockers // []) | join("; ")' /tmp/assessment.json)
          actions=$(jq -r '(.top_actions // []) | map(.action) | join("; ")' /tmp/assessment.json)

          # Write encrypted memory entry
          agent-memory write \
            --type observation \
            --source observation-loop \
            --tag "assessment,${stage,,},run-$(cat /tmp/run-count.txt)" \
            --content "Stage: ${stage}. ${narrative} Blockers: ${blockers}. Actions: ${actions}."
```

**GitHub secret needed:** `AGENT_MEMORY_SECRET` (the passphrase that derives encryption keys)

### 2. collect-memory.sh → agent-memory (READER)

The observation loop feeds agent context via `collect-memory.sh`. We extend
it to also pull from agent-memory.

**In `company/scripts/collect-memory.sh`**, add after the episodic section:

```bash
# ── Agent Memory layer ──────────────────────────────────────
# Pull recent entries from the encrypted IPFS-backed memory store.
# Falls back silently if agent-memory is not installed or not configured.
if command -v agent-memory &>/dev/null && [ -n "${AGENT_MEMORY_SECRET:-}" ]; then
  agent_output=$(agent-memory list --limit 3 --since "$(date -u -d '3 days ago' '+%Y-%m-%d')" 2>/dev/null || true)
  if [ -n "$agent_output" ]; then
    output="${output}

---

# Agent Memory (encrypted/IPFS, last 3 days)
${agent_output}"
  fi
fi
```

This is **additive** — the existing semantic + episodic layers still work,
and agent-memory provides a third layer that persists across repo boundaries.

### 3. Any Repo → agent-memory (READER)

For any repo that wants agent memory context (wgmesh, m4, chimney, etc.):

```bash
# In any CI workflow or local script:
export AGENT_MEMORY_SECRET="${{ secrets.AGENT_MEMORY_SECRET }}"
agent-memory read --tag billing --limit 5
```

Or in a Goose session / Claude Code:
```bash
agent-memory read --tag billing --since 2026-03-01
```

### 4. Custom GitHub Actions → agent-memory (WRITER)

Any workflow can write to the shared memory. Examples:

**Pipeline health check writes failures:**
```yaml
      - name: Record failure to agent-memory
        if: failure()
        env:
          AGENT_MEMORY_SECRET: ${{ secrets.AGENT_MEMORY_SECRET }}
        run: |
          agent-memory write \
            --type blocker \
            --source pipeline-health \
            --tag "pipeline,health-check,failure" \
            --content "Pipeline health check failed in ${{ github.workflow }}. See run ${{ github.run_url }}"
```

**Billing webhook handler writes events:**
```yaml
      - name: Record billing event
        if: github.event.action == 'payment_received'
        env:
          AGENT_MEMORY_SECRET: ${{ secrets.AGENT_MEMORY_SECRET }}
        run: |
          agent-memory write \
            --type observation \
            --source billing-webhook \
            --tag "billing,payment,revenue" \
            --content "Payment received: ${{ github.event.client_payload.amount }} ${{ github.event.client_payload.currency }}"
```

**Heartbeat writes merge events:**
```yaml
      - name: Record merge to agent-memory
        env:
          AGENT_MEMORY_SECRET: ${{ secrets.AGENT_MEMORY_SECRET }}
        run: |
          agent-memory write \
            --type learning \
            --source heartbeat-merge \
            --tag "merge,${{ github.event.pull_request.base.ref }}" \
            --content "Merged PR #${{ github.event.pull_request.number }}: ${{ github.event.pull_request.title }}"
```

### 5. Human / Agent → agent-memory (MANUAL)

Capturing what happened in a session — the thing that's been missing:

```bash
# After a billing integration session:
agent-memory write \
  --type decision \
  --source human \
  --tag "billing,stripe,decision" \
  --content "Decided to use Stripe Checkout (not LemonSqueezy). Reason: better API for B2B invoicing. Previous attempt used LemonSqueezy but their webhook was unreliable for subscription lifecycle events."

# Capturing a failed attempt:
agent-memory write \
  --type trace \
  --source goose \
  --tag "billing,attempt,failure" \
  --content "Attempted Stripe integration via webhook. Failed because: 1) No test mode API key configured, 2) GitHub Actions can't receive Stripe webhooks directly — needs a relay. Next step: set up Stripe CLI for local testing first."
```

---

## Secret Distribution

The `AGENT_MEMORY_SECRET` is the master key. It needs to be:

1. **Set as GitHub secret** in every repo that reads/writes:
   ```bash
   # One-time setup per repo:
   gh secret set AGENT_MEMORY_SECRET --repo atvirokodosprendimai/ai-pipeline-template
   gh secret set AGENT_MEMORY_SECRET --repo atvirokodosprendimai/wgmesh
   gh secret set AGENT_MEMORY_SECRET --repo atvirokodosprendimai/agent-memory
   # ... etc
   ```

2. **Set locally** for CLI use:
   ```bash
   export AGENT_MEMORY_SECRET="your-secret-here"
   # Or add to ~/.config/agent-memory/.env
   ```

3. **Same secret** across all repos — this is what makes it *shared* memory.
   The config (salt, index CID) is per-machine but the secret is universal.

---

## IPFS Daemon in GitHub Actions

The main challenge: IPFS daemon needs to be running for writes.

**Option A: Start ephemeral IPFS node in CI**
```yaml
      - name: Start IPFS daemon
        run: |
          curl -sL https://dist.ipfs.tech/kubo/v0.32.1/kubo_v0.32.1_linux-amd64.tar.gz | tar xz
          export PATH="$PWD/kubo:$PATH"
          ipfs init --profile test
          ipfs daemon &
          sleep 5
          # agent-memory now works (defaults to localhost:5001)
```

**Option B: Use a persistent IPFS pinning service**
- [Pinata](https://pinata.cloud) — free tier, API-based
- [web3.storage](https://web3.storage) — free tier, IPFS + Filecoin
- Add pinning service config to agent-memory later

**Option C: Hybrid — local IPFS + pinning service**
- CI starts ephemeral node, writes, then pins to remote service
- Local reads use local cache
- Best of both worlds

---

## What This Solves

| Before | After |
|--------|-------|
| Billing attempted 3x, zero traces | Every attempt is encrypted + timestamped on IPFS |
| Observation loop only sees GitHub API data | Loop reads human-written decisions from agent-memory |
| Memory is per-repo (ai-pipeline-template only) | Memory is cross-repo, same secret = same memory |
| Session context lost when 128k fills up | Key decisions persist in agent-memory, retrieved on demand |
| No way to capture "why" behind decisions | `--type decision` entries capture reasoning |
| Pipeline health failures invisible to loop | Health check writes failures → loop reads them next run |

---

## Migration Path

1. **Now**: Install `ipfs` locally, run `agent-memory init`, start writing decisions
2. **Next**: Add agent-memory write step to observation-loop.yml
3. **Then**: Extend collect-memory.sh to read from agent-memory
4. **Then**: Distribute AGENT_MEMORY_SECRET to all repos
5. **Later**: Add IPFS ephemeral node to CI workflows
6. **Eventually**: Add pinning service for persistence beyond CI lifetime
