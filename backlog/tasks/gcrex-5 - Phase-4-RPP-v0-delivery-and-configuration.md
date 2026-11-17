---
id: GCREX-5
title: 'Phase 4: RPP v0 delivery and configuration'
status: To Do
assignee: []
created_date: '2026-11-17T16:25:00Z'
updated_date: '2026-11-17T19:35:00Z'
labels: []
dependencies:
  - GCREX-4
references:
  - docs/plans/native-rex-provider.md
  - docs/REX-CONTRACT.md
ordinal: 5000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
The provider must ship as an out-of-tree executable so Rex support reaches users without a gc release. This phase adds the RPP v0 op dispatcher and protocol handshake, the pack declaration that registers the runtime name rex, and the docs that tell a user how to select it. Gas City validates pack runtimes with gc runtime check and conformance runs.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 gc-runtime-rex answers the protocol handshake with version 0 and only the capabilities it implements, drawn from report-attachment, report-activity, proc.exec, proc.provision, proc.stream and tty.attach (no locality field)
- [ ] #2 Every RPP v0 op used by the provider is dispatched and returns protocol-correct exit codes: 1 on failure, 2 only for an unknown op, and 2 treated as a failure from start and provision
- [ ] #3 A pack declares [runtimes.rex] and the runtime resolves by name
- [ ] #4 gc runtime check rex and gc runtime conformance rex pass
- [ ] #5 Docs explain selecting the provider with [session] provider = "rex" or GC_SESSION=rex
<!-- AC:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Verification notes (from the GCREX-8 plan review). Gaps the verification found between the code that exists today and the plan that describes this phase:

- Wire-op mapping. The plan's §5 table left column holds provider-behavior names (Start, GetLastActivity, CopyTo, RunLive), not wire ops. Before writing the dispatcher, map them onto the pinned operations in docs/REX-CONTRACT.md §9: that table has no CopyTo or RunLive rows and adds protocol, exec, provision and watch-startup.
- Payload formats. Contract §9 is not JSON everywhere: nudges, metadata, captures, booleans, process names, and exec commands/output are plain text. Do not assume JSON on stdin/stdout for every op.
- exec scope. Declare proc.exec only when the exec op's exit status carries the in-box command's exit. The excluded ExecProvider is gc's Go interface, not the wire exec op - say which behavior is intended.
- Capabilities. Advertise only implemented capabilities in the protocol handshake; report-attachment and report-activity become declared here (they back is-attached and get-last-activity).
- Start coordination and launch options. The dispatcher is one process per op (contract §9), so an in-process mutex is not enough. provider.go's Config has only Argv and WorkDir, and internal/rex/cli never sends focus=false or login/interactive; the plan promises all three.
- Unimplemented provider methods the dispatcher will need: IsAttached, Attach, ProcessAlive, ListRunning, GetLastActivity, ClearScrollback, CopyTo, RunLive and the metadata ops. BlockClear exists at the transport layer only. provider.Peek also takes no line count, while the wire peek op passes <lines>.

Deferred from GCREX-4 (do not silently drop): (1) Cross-process Start serialization - two dispatcher processes can both resolve a name as missing and create duplicate panes, or both create the city session; an in-process mutex is not enough. (2) The file stores are best-effort for bindings/activity (Save has no error return and a failed write is recovered by the label scan); only MetaStore.Set/Delete return errors. (3) Metadata is a single JSON map read-modify-written per session, so two processes setting different keys can lose one; use an interprocess lock or one file per key. (4) GCREX-4 implemented IsAttached, ProcessAlive and GetLastActivity; Attach, ListRunning, ClearScrollback, CopyTo, RunLive and the meta ops still need the dispatcher. (5) Start must keep sending focus=false and must pass WithStateDir(StateDir(city)) so bindings survive the one-process-per-op model.
<!-- SECTION:NOTES:END -->
