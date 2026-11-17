---
id: GCREX-5
title: 'Phase 4: RPP v0 delivery and configuration'
status: Done
assignee:
  - '@pi'
created_date: '2026-11-17T16:25:00Z'
updated_date: '2026-11-17T20:18:00Z'
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
- [x] #1 gc-runtime-rex answers the protocol handshake with version 0 and only the capabilities it implements, drawn from report-attachment, report-activity, proc.exec, proc.provision, proc.stream and tty.attach (no locality field)
- [x] #2 Every RPP v0 op used by the provider is dispatched and returns protocol-correct exit codes: 1 on failure, 2 only for an unknown op, and 2 treated as a failure from start and provision
- [x] #3 A pack declares [runtimes.rex] and the runtime resolves by name
- [x] #4 gc runtime check rex and gc runtime conformance rex pass
- [x] #5 Docs explain selecting the provider with [session] provider = "rex" or GC_SESSION=rex
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
Phase 4 scope, resolved against the pinned contract (docs/REX-CONTRACT.md) and the gc 1.5.1 source (GC_SRC=/nix/store/k4vki438fzxsrhpyy6icy5xraqcylh89-source). Subagent-orchestrated; parent owns integration, live acceptance and Backlog finalization.

Deliverables
1. internal/provider additions (frozen API): ListRunning(ctx,prefix) ([]string,error) via a BindingLister on MemoryStore/FileStore; ClearScrollback(ctx,name) error; PeekLines(ctx,name,lines) (Peek delegates with lines=0, "last N lines", lines<=0 => all); Config gains ReadyPromptPrefix, and Start records it into Binding.ReadyPrompt (fallback to the GC_READY_PROMPT_PREFIX meta key); CityFromEnv() resolving the dispatcher namespace (GC_REX_CITY, else GC_CITY_PATH basename, else GC_CITY, else "default"); interprocess Start serialization (flock on <stateDir>/lock); fileMetaStore moves to one file per key so concurrent set-meta processes cannot lose keys.
2. cmd/gc-runtime-rex (package main) RPP v0 dispatcher. Entry run(args, stdin, stdout, stderr, deps) int with an injectable rex.Client (tests use internal/rex/faketest). Ops: protocol (handshake {"version":0,"capabilities":["report-attachment","report-activity"]}, no locality), start, stop, is-running, interrupt, process-alive, nudge, set-meta, get-meta, remove-meta, peek <name> <lines>, list-running <prefix>, get-last-activity, is-attached, clear-scrollback, send-keys. exec, provision, attach, watch-startup and any unknown op exit 2. Known-op failure exits 1 (missing session name, bad stdin, Rex error); interrupt/nudge/send-keys/clear-scrollback are best-effort (provider.ErrSessionNotFound -> exit 0); start fails (exit 1) when the session is already running, matching RPP-LIFECYCLE-002. start config JSON {work_dir, command, ready_prompt_prefix}; command is split into argv (shell-quote aware, fallback sh -lc). Dispatcher constructs cli.Client{Bin: GC_REX_BIN}, provider.New(CityFromEnv(), client, provider.WithStateDir(provider.StateDir(city))).
3. Pack + build: a committed pack declaring [runtimes.rex] (command = "gc-runtime-rex", protocol = 0) plus the install path (go install ./cmd/gc-runtime-rex), and a verification city fixture so gc runtime check rex / gc runtime conformance rex resolve the name.
4. Docs: install + selection ([session] provider = "rex" or GC_SESSION=rex), pack declaration, capabilities, and the known v1 limits.
5. Acceptance: go test ./... green; gc runtime check rex and gc runtime conformance rex green against the built executable and a real Rex; the scratch Rex session is cleaned up afterwards.

Known v1 gaps to record (not silently dropped; out of Phase 4 scope, pilot blockers): pre_start/session_setup/session_setup_script/session_live are not run; the start config env map is not applied (Rex launch options have no env — agents inherit the Rex server env); exec/provision/attach are unimplemented (exit 2); per-city isolation needs GC_REX_CITY because the RPP surface carries no city; OSC 7501 is not emitted and nudge busy-gating stays deferred.

Execution: recon (parallel, read-only) -> provider additions -> dispatcher -> pack/docs/fixture -> independent verification (different model family) -> parent live acceptance and finalization.
<!-- SECTION:PLAN:END -->

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

GCREX-4 closure corrections and additions. (Correction) internal/rex/cli NOW sends focus on session.new_window/new_split via rex.LaunchSpec.Focus, default false; login/interactive are still not sent. (New) Binding.ReadyPrompt is persisted but nothing populates it: Start should read the GC_READY_PROMPT_PREFIX metadata (the meta KV stores values byte-for-byte) and record it, or the field should be dropped as unused. (New) rex.Client.Events remains unimplemented and v1 does not consume the event stream by design - do not build dispatcher behavior on events; activity is pull-based through GetLastActivity.

Implemented (subagent-orchestrated, parent-integrated): cmd/gc-runtime-rex RPP v0 dispatcher (run(args,stdin,stdout,stderr,deps), injectable rex.Client); provider additions ListRunning/ClearScrollback/PeekLines/Config.ReadyPromptPrefix/BindingLister/CityFromEnv, a cross-process flock around Start, and a one-file-per-key base64 file meta store; pack/pack.toml [runtimes.rex]; testdata/city fixture; Makefile; docs/rex-provider.md; both live gates green.

Verification evidence: gc runtime check rex -> 14/14 PASS, exit 0; gc runtime conformance rex -> 6 PASS / 2 optional SKIP (exec, provision), exit 0; both printed the 'resolved runtime "rex" from pack "rex"' line. gofmt clean, go build/vet/test clean, go test -race clean. Handshake is exactly {"version":0,"capabilities":["report-attachment","report-activity"]} with no locality. Direct probes: unknown/exec/provision/attach/watch-startup -> 2; known op missing name -> 1; duplicate start -> 1; unreadable meta store get-meta -> 1. Scratch Rex sessions were killed (block-close first) and no pre-existing session touched.

Independent cross-family verification found 8 issues; all fixed and re-verified: concurrent start double-success (new provider.StartNew returns ErrAlreadyRunning inside the state lock; Start stays idempotent), case-insensitive state-dir collision (safeName now encodes uppercase), file meta not byte-faithful (values base64), go test -race failure from an unsynchronized fake (Fake is now mutex-guarded, returns copies), handshake write error ignored (now exits 1), get-meta masking an unreadable store (MetaStore.Get now returns an error), Makefile absolute-BIN_DIR PATH bug, and two docs overstatements (GC_SESSION scope; state-dir uniqueness).

Recorded v1 limits (documented, not silently dropped, pilot blockers): RPP carries no city so GC_REX_CITY is the isolation lever (the controller exports no GC_CITY* under launchd); the start config's env/nudge/pre_start/session_setup/session_setup_script/session_live/copy_files/overlay_dir/pack_overlay_dirs are not applied (gc does not resend the startup nudge, and Rex launch options have no env); list-running is sidecar-derived; exec/provision/attach/watch-startup stay exit 2 and proc.exec/proc.provision are not declared.
<!-- SECTION:NOTES:END -->

## Final Summary

<!-- SECTION:FINAL_SUMMARY:BEGIN -->
Phase 4 (GCREX-5) ships the out-of-tree RPP v0 provider. New: the cmd/gc-runtime-rex dispatcher (one process per op; run() with an injectable rex.Client; exit 0/1/2 with 2 only for unknown or unimplemented ops), the protocol handshake declaring version 0 with only report-attachment and report-activity (no locality, no proc.exec/proc.provision), the pack declaration pack/pack.toml ([runtimes.rex] command=gc-runtime-rex protocol=0), a testdata/city verification fixture, a Makefile (build/install/hooks/test/check/conformance), and docs/rex-provider.md covering [session] provider=rex and GC_SESSION=rex. Provider additions back the wire ops: ListRunning, ClearScrollback, PeekLines, BindingLister, CityFromEnv, Config.ReadyPromptPrefix, a flock that serializes Start across processes, and a one-file-per-key base64 file meta store. Verified: gofmt/build/vet/test and go test -race clean; gc runtime check rex 14/14 PASS and gc runtime conformance rex 6 PASS + 2 optional SKIP, both exit 0 and both resolving the name from the pack. An independent cross-family review found eight defects (concurrent-start double success, case-insensitive state-dir collision, non-byte-faithful metadata, a fake data race, an unchecked handshake write, get-meta masking an unreadable store, a Makefile PATH bug, two docs overstatements); all were fixed with regression tests and re-verified. Risks are recorded as v1 limits: no city is available in an RPP invocation (use GC_REX_CITY), the start config's env/nudge/pre_start/session_setup/copy_files staging is not applied, exec/provision/attach/watch-startup are unimplemented (exit 2), and list-running is sidecar-derived. Those are the Phase 5 pilot blockers.
<!-- SECTION:FINAL_SUMMARY:END -->
