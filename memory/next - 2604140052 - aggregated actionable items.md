---
tldr: Aggregated actionable items for agent-memory as of 2026-04-14
---

# Next: agent-memory — 2026-04-14

## From Spec Implementation Plan

### Phase 0 (done)
- [x] README, SPEC, INTEGRATION docs
- [x] Go module, cmd/ structure
- [x] `init` — key derivation, config, IPFS test

### Phase 1 (code-complete, tests partial)
- [x] `write` — encrypt + pin
- [x] `read` — fetch + decrypt
- [x] `list` — index query
- [x] Index management (encrypted on IPFS)
- [x] Store + filter unit tests (ab5765d)
- [ ] cmd/agent-memory tests (0% coverage)
- [ ] `gc` command (spec §6.1)
- [ ] `export` command (spec §6.1)
- [ ] `import` command (spec §6.1)
- [ ] `--raw` flag on read

### Phase 2 (not started)
- [ ] collect-memory.sh integration
- [ ] Observation loop writes assessments
- [ ] Goose post-session hook

### Phase 3 (not started)
- [ ] Capture billing v1/v2/v3 context as first real payload
- [ ] Verify observation loop reads billing context

## Open Questions (spec §11)
- [ ] Remote pinning service choice
- [ ] Multi-agent key strategy
- [ ] Index sync / merge conflict strategy
- [ ] Content size limits
- [ ] Full-text search over encrypted data

## Prioritised

1 - Test coverage gaps
  1.1 - cmd/agent-memory tests (0% coverage, CLI wrappers)
2 - Missing Phase 1 commands
  2.1 - gc command — unpin entries older than --max-age, rebuild index
  2.2 - export command — decrypt + write JSONL
  2.3 - import command — read JSONL + encrypt + pin
  2.4 - --raw flag on read (print full JSON)
3 - Phase 2: Integration
  3.1 - Patch collect-memory.sh to read from agent-memory
  3.2 - Add agent-memory write step to observation-loop.yml
  3.3 - Goose post-session hook
4 - Phase 3: First real payload
  4.1 - Write billing v1/v2/v3 context entries
  4.2 - End-to-end verify: observation loop picks up billing context
5 - Open questions
  5.1 - Decide remote pinning service
  5.2 - Decide multi-agent key strategy
  5.3 - Design index merge conflict resolution
