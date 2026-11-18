// Gas City hooks for Pi Coding Agent — rex pack override.
//
// gc installs its managed copy of this file from the core pack
// (internal/bootstrap/packs/core/overlay/per-provider/pi/.pi/extensions/gc-hooks.js).
// This rex-pack overlay supersedes that copy when the provider stages each
// pack's per-provider/pi subtree into the work dir after pre_start (see
// internal/provider/overlay.go). gc's own staging can skip the rex copy: its
// managed-file preserve predicate keeps an existing current managed hook, so
// the controller's last-writer-wins pass is not sufficient on that path. The
// provider also stages this per-provider subtree itself because gc's managed
// preserve predicate keeps an existing current v9 hook on the reconcile path.
//
// The override exists only for the claim guard in before_agent_start: gc's
// stalled-claim backstop re-queues the fallback pool-claim nudge ("Run gc hook
// --claim --drain-ack --json now; ...") for a pool slot whose routed trigger
// bead still reads open, and the managed hook drained and injected it
// unconditionally. A seat that already holds a claim then received that text
// mid-task, ran `gc hook --claim --drain-ack` again, handed its claim back and
// abandoned the task (GCREX-33; my-city events 2026-11-18 06:47-06:53,
// mc-wisp-gwble). The guard below leaves the nudge queue undrained while the
// session still holds a live claim, so nothing is injected mid-claim and
// nothing is acked or lost; once the stamped bead closes, is released, or is
// owned by another session the claim is no longer held and the next turn
// drains and delivers what was queued.
//
// Keep the version marker well above the bundled managed version (gc 1.5.1
// ships v9; gc HEAD ships v10). gc's preserve predicate
// (internal/hooks/preserve.go) reads the marker of the file already on the
// destination, not the desired overlay's, and preserves that file whenever its
// marker is not older than the managed version. Once this override is staged
// its marker is what keeps gc from replacing it. The predicate otherwise keeps
// an existing current managed hook, so the provider stages this per-provider
// subtree itself and the controller never writes this override. The provider's
// writer does not compare versions; if a future managed hook exceeds 100,
// re-bump this marker.
//
// Pi 0.70+ extension API uses a factory function and pi.on(...)
// subscriptions. Keep this file as .js for existing Gas City provider args
// and auto-discovery paths.
//
// Events:
//   session_start    → gc prime --hook (load context side effects)
//   session_compact  → gc prime --hook + gc handoff --auto "context cycle"
//   before_agent_start → gc hook --inject + queued nudges + unread mail

const { execFileSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");

const GC_PI_HOOK_VERSION = 100;
// Managed sessions supply the authoritative binary and PATH. Do not prepend
// ambient host locations here: gc's descendants must inherit the same runtime.
const GC_BIN = process.env.GC_BIN || "gc";
let mirrorTempCounter = 0;
// `gc prime --hook` writes the session's startup context to stdout. pi has no
// session_start hook surface that can inject into the model's context, so the
// SessionStart output is parked here and drained into the system prompt by the
// next before_agent_start. Dropping it would consume auto-handoff mail (gc
// archives it once the write succeeds) without ever delivering it.
let pendingPrimeContext = "";

function run(args, cwd, extraEnv = {}) {
  try {
    return execFileSync(GC_BIN, args, {
      cwd: cwd || process.cwd(),
      encoding: "utf-8",
      timeout: 30000,
      stdio: ["ignore", "pipe", "inherit"],
      env: {
        ...process.env,
        ...extraEnv,
      },
    }).trim();
  } catch (err) {
    logRunFailure(args, cwd, err);
    return "";
  }
}

function logRunFailure(args, cwd, err) {
  try {
    const detail =
      (err && (err.code || err.signal || err.message)) || "unknown error";
    console.error(
      "gc-hooks run:",
      `gc ${args.join(" ")}`,
      "cwd",
      cwd || process.cwd(),
      "failed:",
      detail,
    );
  } catch {
    // Keep Pi hooks non-fatal even if stderr is unavailable.
  }
}

function safeSessionID(sessionID) {
  // Keep this filename contract in sync with safePiSessionID in
  // internal/sessionlog/pi_reader.go.
  return String(sessionID || "").replace(/[^A-Za-z0-9_.-]/g, "_");
}

function mirrorTempPath(dst) {
  mirrorTempCounter += 1;
  return `${dst}.tmp.${process.pid}.${Date.now()}.${mirrorTempCounter}`;
}

function sessionManagerHeader(manager, cwd) {
  try {
    const header = manager.getHeader && manager.getHeader();
    if (header && typeof header === "object") {
      return { ...header, cwd: header.cwd || cwd };
    }
  } catch {
    // Continue to the fallback header below.
  }
  return {
    type: "session",
    version: 3,
    id: manager.getSessionId && manager.getSessionId(),
    timestamp: new Date().toISOString(),
    cwd,
  };
}

function providerSessionEnv(ctx) {
  const sessionID = ctx?.sessionManager?.getSessionId?.() || "";
  const env = { GC_PROVIDER_SESSION_ID_REQUIRED: "pi" };
  if (!sessionID) {
    return env;
  }
  env.GC_PROVIDER_SESSION_ID = String(sessionID);
  return env;
}

// gc identifies a managed hook invocation by GC_MANAGED_SESSION_HOOK and
// routes on GC_HOOK_EVENT_NAME, exactly as the claude settings.json hook does.
// Without both, `gc prime --hook` cannot tell that gc already delivered the
// startup prompt inline and re-emits the entire prompt on top of it.
function hookEnv(ctx, eventName) {
  return {
    ...providerSessionEnv(ctx),
    GC_MANAGED_SESSION_HOOK: "1",
    GC_HOOK_EVENT_NAME: eventName,
  };
}

// currentClaimID returns the work bead this session most recently claimed
// through `gc hook --claim`, or "" when it holds no claim. gc's contract is
// that the session claim is read only through `gc hook current --id-only`;
// $GC_BEAD_ID exists only in the controller's dispatch condition environment,
// never in a session shell. If it ever reached a session it would name a
// foreign or stale bead, so the guard consults gc unconditionally. run() turns
// the exit-1 "no current claim" answer into "".
function currentClaimID(ctx) {
  return run(["hook", "current", "--id-only"], ctx.cwd).trim();
}

// claimIsLive reports whether the stamped work bead `claim` is still held by
// this session. gc stamps current_claim_bead_id on the session bead and does
// not clear it when the claimed bead closes (gc 1.5.1
// internal/session/store.go CurrentClaimBeadID:356-362), so a non-empty stamp
// alone is not liveness. gc's own reconciler re-reads the work bead and treats
// a closed, unassigned, or foreign-owned claim as not held (gc 1.5.1
// cmd/gc/session_ownership.go claimedWorkStillHeldBy:123-133). This mirrors
// that predicate with `gc bd list --id <claim> --all --json`; an empty or
// unreadable answer is treated as not held so a finished claim stops deferring
// the backstop nudge instead of suppressing it until the session ends.
function claimIsLive(ctx, claim) {
  if (!claim) {
    return false;
  }
  const sessionID = String(process.env.GC_SESSION_ID || "").trim();
  const out = run(["bd", "list", "--id", claim, "--all", "--json"], ctx.cwd);
  if (!out) {
    return false;
  }
  let records;
  try {
    records = JSON.parse(out);
  } catch {
    return true; // Garbled read: keep the claim rather than clobber a seat.
  }
  if (!Array.isArray(records) || records.length === 0) {
    return false;
  }
  const work = records[0] || {};
  if (work.status === "closed") {
    return false;
  }
  if (work.status === "open" && !String(work.assignee || "").trim()) {
    return false;
  }
  const owner = String((work.metadata && work.metadata["gc.session_id"]) || "").trim();
  if (owner && owner !== sessionID) {
    return false;
  }
  return true;
}

function mirrorTranscript(ctx) {
  const exportDir = process.env.GC_PI_TRANSCRIPT_DIR || "";
  const manager = ctx && ctx.sessionManager;
  if (!exportDir || !manager) {
    return;
  }
  let tmp = "";
  try {
    const cwd = (manager.getCwd && manager.getCwd()) || ctx.cwd || process.cwd();
    const sessionID = safeSessionID(manager.getSessionId && manager.getSessionId());
    if (!sessionID) {
      return;
    }
    fs.mkdirSync(exportDir, { recursive: true });
    const dst = path.join(exportDir, `${sessionID}.jsonl`);
    tmp = mirrorTempPath(dst);
    const sessionFile = manager.getSessionFile && manager.getSessionFile();
    if (sessionFile && fs.existsSync(sessionFile)) {
      fs.copyFileSync(sessionFile, tmp);
      fs.renameSync(tmp, dst);
      return;
    }
    const header = sessionManagerHeader(manager, cwd);
    const entries = manager.getEntries ? manager.getEntries() : [];
    const lines = [header, ...entries].map((entry) => JSON.stringify(entry));
    fs.writeFileSync(tmp, `${lines.join("\n")}\n`, "utf8");
    fs.renameSync(tmp, dst);
  } catch (err) {
    if (tmp) {
      try {
        fs.rmSync(tmp, { force: true });
      } catch {
        // Keep the original mirror error as the useful diagnostic.
      }
    }
    try {
      console.error(
        "gc-hooks mirrorTranscript:",
        err && err.message ? err.message : String(err),
      );
    } catch {
      // Keep Pi hooks non-fatal even if stderr is unavailable.
    }
  }
}

function appendSystemPrompt(systemPrompt, additions) {
  const extras = additions.filter(Boolean);
  if (extras.length === 0) {
    return systemPrompt;
  }
  return [systemPrompt, ...extras].filter(Boolean).join("\n\n");
}

function gascityPiExtension(pi) {
  pi.on("session_start", (_event, ctx) => {
    pendingPrimeContext = run(["prime", "--hook"], ctx.cwd, hookEnv(ctx, "SessionStart"));
    mirrorTranscript(ctx);
  });

  pi.on("session_compact", (_event, ctx) => {
    run(["prime", "--hook"], ctx.cwd, hookEnv(ctx, "PreCompact"));
    run(["handoff", "--auto", "context cycle"], ctx.cwd);
    mirrorTranscript(ctx);
  });

  pi.on("before_agent_start", (event, ctx) => {
    const prime = pendingPrimeContext;
    pendingPrimeContext = "";
    const work = run(["hook", "--inject"], ctx.cwd);
    // Defer the queued nudges while this session still holds a live claim.
    // Draining here would inject gc's fallback pool-claim nudge mid-task;
    // running it again with --drain-ack hands the claim back and clobbers the
    // work in progress. Leaving the queue undrained defers every nudge, so
    // nothing is acked and the next turn after the claim stops being held
    // delivers what is queued.
    const claim = currentClaimID(ctx);
    const nudges = claimIsLive(ctx, claim) ? "" : run(["nudge", "drain", "--inject"], ctx.cwd);
    const mail = run(["mail", "check", "--inject"], ctx.cwd);
    const systemPrompt = appendSystemPrompt(event.systemPrompt, [prime, work, nudges, mail]);
    if (systemPrompt !== event.systemPrompt) {
      return { systemPrompt };
    }
  });

  pi.on("message_end", (_event, ctx) => {
    mirrorTranscript(ctx);
  });

  pi.on("agent_end", (_event, ctx) => {
    mirrorTranscript(ctx);
  });

  pi.on("session_shutdown", (_event, ctx) => {
    mirrorTranscript(ctx);
  });
}

// Exposing it keeps the declaration referenced and documents the override's
// version. The provider's writer does not compare it, and gc's preserve
// predicate decides from the existing destination file, not this marker.
gascityPiExtension.gcPiHookVersion = GC_PI_HOOK_VERSION;
module.exports = gascityPiExtension;
