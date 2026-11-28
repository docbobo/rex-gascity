# 0020. Keep `fs.host` undeclared

- Status: Accepted
- Date: 2026-11-26

## Context

Parity row 90 (the parity document's lines `:90` and `:313`, retired by
[ADR-0011](0011-verify-the-provider-against-gc-and-conformance-not-herdr-core-parity.md))
read: we declare `"locality":"local"` but not the `fs.host` capability token.
The live statement of the same fact is `docs/rex-provider.md:200-220` (the
handshake, `locality`, and `fs.host` under "Capabilities not declared") and
`docs/REX-CONTRACT.md:344-365` (the non-baseline-token rule). The RPP doc says
what the token buys. A handshake declaring `fs.host` **with** `locality: local`
promises that its boxes run on gc's host, as gc's user, with gc's filesystem
view (`/Volumes/Sources/gascity/docs/reference/exec-session-provider.md:729-736`).
gc then does the session's host work itself, around `start`
(`exec-session-provider.md:738-771`):

1. It stages the work dir (`pack_overlay_dirs`, `overlay_dir`, `copy_files`)
   and runs `pre_start` on the host.
2. It sends a start config without those fields, and without `session_setup`
   and `session_setup_script`.
3. It runs `session_setup` and `session_setup_script` on the host afterwards.
4. It reads the agent's transcripts for turn tracking.
5. On a v2-ready script, it reaps a session's leftover processes from the host
   process table.
6. It admits skills and MCP for the runtime.

The token is read through one predicate, `ProtocolInfo.SharesHostFS()` =
`DeclaresLocal() && Has("fs.host")`
(`/Volumes/Sources/gascity/internal/runtime/protocol.go:236-242`, pinned by
`protocol_test.go:16-33`). Every consumer routes through it
(`internal/runtime/exec/hoststage.go:53`, `:64`
and `cmd/gc/runtime_registry.go:163-168`). The pack's GASCITY_SRC checkout is
the edge build, `890a58a3e`. The parity doc's pin is `v1.5.1`, which parses six
tokens and ignores the rest (`parity doc:312`).

In a local deployment Rex boxes run on gc's host, as gc's user. The spool is
local, and the server is a unix socket under the controller's `Application
Support` (`cmd/gc-runtime-rex/main.go:41-45`, `docs/rex-provider.md:207-215`).
So `locality: local` is true there. It is false for a provider pointed at a
remote server through a tunnel (`docs/rex-provider.md:215`), and the handshake
declares `local` unconditionally today (`cmd/gc-runtime-rex/main.go:60`). This
ADR therefore scopes its decision to a local deployment. Making the field
follow the deployment target needs a detection mechanism this ADR does not
choose. The question here is only whether we take on the `fs.host` promise.

Today the provider reads a start config that carries `overlay_dir`,
`pre_start`, `session_setup` and `session_setup_script`, and runs them itself
(the parity document is retired by ADR-0011; the live anchors are
`docs/rex-provider.md:660-664` and `docs/REX-CONTRACT.md:874-876`, which record
the provider staging `copy_files`/`overlay_dir`, running `pre_start`, and
running `session_setup`/`session_setup_script` itself). That is the "older gc"
path. A script does exactly what its start config carries
(`exec-session-provider.md:778-783`).

## Decision

We keep `fs.host` undeclared. For a local deployment `locality` stays
`"local"`, and the handshake declares the seven tokens it declares today
(`cmd/gc-runtime-rex/main.go:50-59`).

The Rex model makes every behaviour the token would add ours to re-prove. Each
one also copies a promise we already own:

- **Host staging and `pre_start`.** gc would stage into `work_dir` on the host
  before `start`. This provider keeps that copy only because gc sends it. We
  would then keep the provider path for an older gc and the gc path for a newer
  one. A cached handshake decides the split, and it stays stale until gc
  restarts (`exec-session-provider.md:783-789`).
- **Transcripts.** gc would read agent transcripts from paths on the host
  (`exec-session-provider.md:791`). The provider feeds idleness from the pane
  stream instead (`report-activity` plus the `idle-boundary`-gated `idle` op;
  `docs/rex-provider.md:217-226`, `docs/REX-CONTRACT.md:344-365`). The retired
  parity rows are not citable.
- **Orphan reaping.** gc would scan its process table for processes that carry
  the session's `GC_SESSION_ID` and `GC_CITY_PATH`
  (`exec-session-provider.md:801-810`). Our reaper closes Rex panes
  (`internal/provider/reap.go`). A second reaper on the same identity becomes a
  second owner of one promise.
- **Skill and MCP admission.** These run off `SharesHostFilesystem`, and
  `Locality` alone does not set it (`exec-session-provider.md:791-799` and
  `cmd/gc/skill_integration.go:89`). This is the one real gap. The gc change
  that admits `herdr` by name closes it, and the token does not. Rex uses the
  `herdr` provider family.

`REX-CONTRACT.md:339-365` already carries the rule. Declare a non-baseline
token only in the change that implements the behaviour it gates. The same text
says "`fs.host` is not a declaration. Agents simply run on gc's host".

## Consequences

- A newer gc sends the full start config, as `v1.5.1` does. The provider keeps
  one staging path and one `session_setup` path.
- Skill and MCP materialization for a Rex agent stays on the legacy
  `legacyStage1RuntimeNames` path. gc admits `herdr` by name there
  (`cmd/gc/skill_integration.go:64-68`), until gc admits the runtime family.
  If gc drops that list, this decision re-opens for the admission rows only.
- gc's orphan reaper does not run for Rex sessions. `internal/provider/reap.go`
  stays the only reaper.
- The retired parity rows 90 and deviation 2 recorded `matches` on `locality`
  (retired by ADR-0011). The live capability set stays "7 tokens, 3 outside
  ours" (`cmd/gc-runtime-rex/main.go:60`, `docs/rex-provider.md:200-220`).
- Whoever declares `fs.host` later must delete the provider's staging and
  `session_setup`, and keep a path for an older gc. That is a restart-visible
  split, not a flag.

## Alternatives rejected

- **Declare `fs.host` and drop the provider's staging and `pre_start`.** gc
  would be the only stager. Every gc before the token is consumed then breaks.
  A `v1.5.1` pin also ignores the token and sends the full config. A cached
  handshake decides the divergence, so a script that drops `fs.host` still gets
  a host-staged config until gc restarts (`exec-session-provider.md:783-789`).
- **Declare `fs.host` and keep running staging ourselves.** The start config is
  the only signal, and under `fs.host` gc leaves those fields out. Running
  nothing is what herdr-core's "older gc" path does. Running twice is worse. The
  token promises that gc does the work, so handing it back is not an option.
- **Declare `fs.host` for the skill and MCP gates alone.** Those gates are the
  only gap. The token also drags staging, transcripts, reaping and a second
  `session_setup` path with it. One gap is not worth five behaviours. Admit the
  family by name instead (`skill_integration.go:64-68`).
- **Drop `locality` too, since we refuse the token.** `locality` is a statement
  of deployment fact with its own consumer. `sessionRuntimeIsRemote` uses it for
  the storage-split check (`cmd/gc/storage_verb_exclusion.go:314`). The fact is
  true for a local deployment. Only the `fs.host` narrowing is untaken.

## Sources and status

Every gc-source claim below was re-checked for this run. Repo claims name the
file read; a claim backed by a test names the test that proves it. No gc source
tree is present in the run environment, so the gc readings stay `unverified`
and name the missing source.

| Claim | Source read | v1.5.1 vs edge | Status |
| --- | --- | --- | --- |
| Handshake declares seven tokens, `locality: local`, no `fs.host` | `cmd/gc-runtime-rex/main.go:60`; `cmd/gc-runtime-rex/dispatch_test.go:84-114` | provider repo | verified by `TestProtocolHandshake` |
| `ProtocolInfo.SharesHostFS()` is `DeclaresLocal() && Has("fs.host")` | none — gc tree absent | both | unverified (missing gc checkout) |
| Staging/`pre_start` consumers (`internal/runtime/exec/hoststage.go`, `cmd/gc/runtime_registry.go`) | none — gc tree absent | both | unverified (missing gc checkout) |
| `v1.5.1` parses six tokens and has no `fs.host` consumer | none — pin tree absent | pin | unverified (missing `v1.5.1` tree) |
| Skill/MCP admission runs off `SharesHostFilesystem`, and `legacyStage1RuntimeNames` names `herdr` | none — gc tree absent | both | unverified (missing gc checkout) |
| Non-baseline tokens are declared only with the behaviour they gate; `fs.host` is not a declaration | `docs/REX-CONTRACT.md:344-365` | repo | verified (read in this repo) |
| Provider feeds idleness from the pane stream | `docs/rex-provider.md:217-226` | repo | verified (read in this repo) |

The earlier handshake check read `hs["fs.host"]`, a top-level key the token
never is; it could not fail. `TestProtocolHandshake` now checks the
`capabilities` array (the ADR-0007 shape).

When CI or a reviewer has a gc checkout, re-read each `unverified` row at both
`v1.5.1` and the edge build and replace the label. Until then this ADR does not
claim those readings. The `/Volumes/Sources/gascity` path cited above is the
authoring machine's checkout and is absent in the run environment.
