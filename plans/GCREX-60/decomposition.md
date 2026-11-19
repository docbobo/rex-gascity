---
schema: gc.build.decomposition.v1
workflow: {id: gcrex-140, formula: build-basic}
methodology: {pack: gascity, name: build-basic}
producer: {formula: build-basic, stage: decompose, attempt: 3}
status: approved
trace:
  upstream:
    - path: plans/GCREX-60/requirements.md
      hash: sha256:2b86050135ee63041421b3a6d61a78601d389c1ef3200c6fe7cdc609dadc4130
      ids: [REQ-001, REQ-002, REQ-003, REQ-004]
    - path: plans/GCREX-60/implementation-plan.md
      hash: sha256:325c9d29870f46d9567a8215a9df62cc5d941b741ea847aa635c8451db15a20c
  coverage:
    - {id: REQ-001, status: covered}
    - {id: REQ-002, status: covered}
    - {id: REQ-003, status: covered}
    - {id: REQ-004, status: covered}
---

# Decomposition: GCREX-60 rex-backlog blocked-task loop

## Summary

The fix already exists in `~/my-city` at commit `4c1318c`. Two work items: one verify-only bead covering REQ-001..004, and one optional comment-typo bead. Per plan-review, plan sections 1-3 and 6 share `make test` and stay in one bead.

## Selected Downstream Formulas

- implementation: `implement` (item formula `do-work-item`, target `gc.implementation-worker`)
- review: `review`; fix loop: `fix-loop-base`

## Implementation Convoy

Convoy `gcrex-0sa1` (gcrex-60-implementation) holds `gcrex-hvlf` and `gcrex-72t8`.

## Work Items

### gcrex-hvlf: verify fix against REQ-001..004 (plan §1-3, §6)

- Run `make -C ~/my-city test`: blocked 7, noop 10, ready 10.
- Red/green: ready test against `HEAD~1` gate must FAIL `blocked labeled To Do (todo=1, ready=0) does not fire`; passes on HEAD.
- Write `plans/GCREX-60/implementation-summary.md` with counts.
- Covers REQ-001, REQ-002, REQ-003, REQ-004.

### gcrex-72t8: fix "noon" comment typo (plan §4-5, optional)

- Change `noon` to `no-op` in `~/my-city/orders/rex-backlog.toml` header.
- Leave operator's uncommitted `enabled = false`; stage only the comment hunk.
- Proof: `grep -n noon` empty; diff still shows `enabled = false`.
- No requirement coverage.

## Coverage

| ID | Status |
| --- | --- |
| REQ-001 | covered |
| REQ-002 | covered |
| REQ-003 | covered |
| REQ-004 | covered |
