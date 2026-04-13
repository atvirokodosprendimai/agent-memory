---
tldr: Phase 1 complete — prioritised next items for agent-memory
---

# Next: agent-memory — 2026-04-14 (Phase 1 complete)

## Session progress
- [x] Phase 0: scaffold (README, SPEC, INTEGRATION, Go module)
- [x] Phase 1: core CLI (init, write, read, list, pins, gc, export, import, --raw)
- [x] Test coverage: 41+ tests across store, filter, cmd packages
- Branch `task/phase1-remaining-commands` ready to merge

## Prioritised

1 - Merge Phase 1 branch
  1.1 - Merge `task/phase1-remaining-commands` into main

2 - Phase 2: Integration (connect to existing infrastructure)
  2.1 - Patch collect-memory.sh to read from agent-memory
  2.2 - Add agent-memory write step to observation-loop.yml
  2.3 - Goose post-session hook

3 - Phase 3: First real payload
  3.1 - Write billing v1/v2/v3 context as first entries
  3.2 - E2E: observation loop picks up billing context

4 - Open questions (spec §11)
  4.1 - Remote pinning service choice
  4.2 - Multi-agent key strategy
  4.3 - Index merge conflict resolution
  4.4 - Content size limits
  4.5 - Full-text search over encrypted data

5 - Quality
  5.1 - Store integration tests (need IPFS daemon to exercise GC/Export/Import)
  5.2 - CI/CD setup (GitHub Actions)
  5.3 - Release binary builds (goreleaser)
