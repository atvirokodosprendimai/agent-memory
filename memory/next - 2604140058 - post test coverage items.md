---
tldr: Actionable items after test coverage push — 2026-04-14
---

# Next: agent-memory — 2026-04-14 (post tests)

## Completed this session
- [x] Store + filter unit tests (ab5765d)
- [x] cmd/agent-memory tests (be99dd3)
- Branch `task/store-and-cmd-tests` ready to merge (3 commits)

## Prioritised

1 - Merge test branch
  1.1 - Merge `task/store-and-cmd-tests` into main
2 - Missing Phase 1 commands (complete the CLI surface)
  2.1 - gc — unpin entries older than --max-age, rebuild index
  2.2 - export — decrypt matching entries to JSONL file
  2.3 - import — read JSONL, encrypt, pin each entry
  2.4 - --raw flag on read (print full JSON with metadata)
3 - Phase 2: Integration (connect to existing infra)
  3.1 - Patch collect-memory.sh to read from agent-memory
  3.2 - Add agent-memory write step to observation-loop.yml
  3.3 - Goose post-session hook
4 - Phase 3: First real payload
  4.1 - Write billing v1/v2/v3 context as first entries
  4.2 - E2E: observation loop picks up billing context
5 - Open questions (spec §11)
  5.1 - Remote pinning service choice
  5.2 - Multi-agent key strategy
  5.3 - Index merge conflict resolution
  5.4 - Content size limits
  5.5 - Full-text search over encrypted data
