#!/usr/bin/env python3
"""Show the workstreams a Gas City rig is actively working on.

A *workstream* is the top-level "origin" bead that a unit of work was launched
for (an issue-build, a CodeRabbit merge, a hand-filed bug, ...). Beads created
by a formula carry their ancestry in metadata (`gc.root_bead_id`,
`gc.graphv2_root_key`, `gc.parent_convoy_id`) and in convoy `tracks` edges;
this script walks those links to roll every active bead up under its origin and
prints one bordered box per workstream (--layout tree indents instead).

By default only workstreams the rig is *actually working on* are shown: a
stream is in flight when one of its beads is `in_progress`, or is `open` with a
`started_at` (a session has claimed it - shown as the 'claimed' icon). The
open backlog is summarised in one line; use --scope to widen it.

Data comes from one `bd export` call, so the whole graph is resolved locally.

Each bead shows its duration (started, or created, to closed or now) and each
workstream shows its span (earliest member start to now, or the last close).

Each workstream also carries a progress bar over its whole lifetime: the bar is
split by status (closed = full block, in progress = dark shade, blocked =
medium, open = light), so the filled fraction is the share of the stream's
beads that are done.

Run it as a Gas City pack command:

    gc rex dashboard [--rig DIR] [--scope working|attention|all]
                     [--all] [--layout box|tree] [--columns N]
                     [--watch] [--interval SECONDS] [--json]

Without --rig it shows the rig under the current directory: it asks
`gc rig list --json` for the city's rigs and picks the one whose path contains
cwd, nearest first. Run it from a rig checkout, a rig subdirectory, or the
city root, and it lands on the rig you are standing in.

The same file runs standalone for development:

    python3 pack/commands/dashboard/dashboard.py -C ~/my-city/rigs/rex-gascity
    python3 pack/commands/dashboard/dashboard.py --scope all --layout tree
    python3 pack/commands/dashboard/dashboard.py --json | jq .
"""

from __future__ import annotations

import argparse
import collections
import json
import math
import os
import select
import shutil
import signal
import subprocess
import sys
import time
import unicodedata
from datetime import datetime, timezone

try:
    import termios as _termios
    import tty as _tty
except ImportError:  # pragma: no cover - non-POSIX terminals
    _termios = None
    _tty = None

ICON = {
    "open": "\u25cb",        # ○
    "in_progress": "\u25d0",  # ◐
    "blocked": "\u25cf",      # ●
    "deferred": "\u2744",    # ❄
    "closed": "\u2713",       # ✓
    "pinned": "\U0001f4cc",  # 📌
    "hooked": "\u25c7",       # ◇
    "claimed": "\u25d4",      # ◔ open but claimed by a session (started_at)
}
BAR_CHAR = {
    "closed": "\u2588",       # full block
    "in_progress": "\u2593",  # dark shade
    "claimed": "\u2593",
    "blocked": "\u2592",      # medium shade
    "open": "\u2591",         # light shade
    "deferred": "\u2591",
    "pinned": "\u2591",
    "hooked": "\u2591",
}
STYLE = {
    "open": "\033[2;37m",
    "in_progress": "\033[1;36m",
    "blocked": "\033[1;31m",
    "deferred": "\033[34m",
    "closed": "\033[32m",
    "claimed": "\033[36m",
}
RESET = "\033[0m"
BOLD = "\033[1m"
DIM = "\033[2m"
# Kept out of f-string replacement fields: Python 3.12+ allows a backslash
# there, older versions do not.
DOT = "\u00b7"
GEAR = "\u2699"


class Paint:
    def __init__(self, enabled: bool) -> None:
        self.enabled = enabled

    def __call__(self, text: str, style: str) -> str:
        if not self.enabled:
            return text
        return f"{style}{text}{RESET}"


# --- bead graph -------------------------------------------------------------


class Model:
    def __init__(self, recs: dict, active_statuses: tuple[str, ...]) -> None:
        self.recs = recs
        self.active_statuses = active_statuses
        self._parent_cache: dict[str, str | None] = {}
        self._track_cache: dict[str, list[str]] = {}

    def md(self, rec: dict, key: str):
        return (rec.get("metadata") or {}).get(key)

    def tracks_targets(self, cid: str) -> list[str]:
        if cid in self._track_cache:
            return self._track_cache[cid]
        rec = self.recs.get(cid)
        out = []
        if rec:
            out = [
                d["depends_on_id"]
                for d in (rec.get("dependencies") or [])
                if d.get("type") == "tracks"
            ]
        self._track_cache[cid] = out
        return out

    def parent_needs(self, bid: str) -> list[str]:
        """Convoy targets whose parent this bead's rule needs resolved first."""
        rec = self.recs.get(bid)
        if not rec or rec.get("issue_type") != "convoy":
            return []
        targets = [t for t in self.tracks_targets(bid) if t != bid]
        return targets if len(targets) > 1 else []

    def parent_rule(self, bid: str) -> str | None:
        """The parent rule, reading already-resolved targets from the cache."""
        rec = self.recs.get(bid)
        if not rec:
            return None
        if rec.get("issue_type") == "convoy":
            targets = [t for t in self.tracks_targets(bid) if t != bid]
            if len(targets) == 1:
                return targets[0]
            if targets:
                parents = [self._parent_cache.get(t) for t in targets]
                if all(x == parents[0] for x in parents):
                    return parents[0]
            return None
        root = self.md(rec, "gc.root_bead_id")
        if root and root != bid:
            return root
        key = self.md(rec, "gc.graphv2_root_key")
        p: str | None = None
        if key:
            parts = key.split(":")
            convoy = parts[1] if len(parts) > 1 else None
            targets = (
                [t for t in self.tracks_targets(convoy) if t != bid]
                if convoy
                else []
            )
            if len(targets) == 1:
                p = targets[0]
        if p is None:
            convoy = self.md(rec, "gc.parent_convoy_id") or self.md(
                rec, "gc.drain_parent_convoy_id"
            )
            if convoy:
                targets = [t for t in self.tracks_targets(convoy) if t != bid]
                if len(targets) == 1:
                    p = targets[0]
        return p

    def parent(self, bid: str) -> str | None:
        """Nearest container of a bead, or None if it is itself an origin.

        Resolved with an explicit stack: a convoy's parent rule needs the
        parents of its targets, and a deep convoy chain must not overflow
        Python's recursion limit. A bead still on the stack (a cycle) reads
        as None, which preserves the old cycle guard.
        """
        if bid in self._parent_cache:
            return self._parent_cache[bid]
        stack = [bid]
        visiting = {bid}
        while stack:
            cur = stack[-1]
            # Push one need at a time. Pushing a sibling set at once makes
            # later siblings look like a cycle to each other, so their parent
            # rule would read targets that are not resolved yet.
            nxt = next(
                (
                    t
                    for t in self.parent_needs(cur)
                    if t not in self._parent_cache and t not in visiting
                ),
                None,
            )
            if nxt is not None:
                stack.append(nxt)
                visiting.add(nxt)
                continue
            self._parent_cache[cur] = self.parent_rule(cur)
            stack.pop()
            visiting.discard(cur)
        return self._parent_cache[bid]

    def origin(self, bid: str) -> str:
        seen: set[str] = set()
        while bid and bid not in seen:
            seen.add(bid)
            p = self.parent(bid)
            if not p:
                return bid
            bid = p
        return bid

    def chain(self, bid: str) -> list[str]:
        out: list[str] = []
        seen: set[str] = set()
        while bid and bid not in seen:
            seen.add(bid)
            out.append(bid)
            bid = self.parent(bid)
        return out

    def active(self) -> list[dict]:
        return [r for r in self.recs.values() if r.get("status") in self.active_statuses]

    def work_items(self) -> set[str]:
        out: set[str] = set()
        for rec in self.recs.values():
            if rec.get("issue_type") == "convoy":
                out.update(self.tracks_targets(rec["id"]))
        return out

    def groups(self) -> dict[str, list[dict]]:
        groups: dict[str, list[dict]] = collections.defaultdict(list)
        for rec in self.active():
            groups[self.origin(rec["id"])].append(rec)
        return groups

    def all_groups(self) -> dict[str, list[dict]]:
        """Every bead ever, rolled up under its origin (for progress counts)."""
        groups: dict[str, list[dict]] = collections.defaultdict(list)
        for rec in self.recs.values():
            groups[self.origin(rec["id"])].append(rec)
        return groups

    def nearest(self, bid: str, candidates: set[str]) -> str | None:
        for anc in self.chain(bid)[1:]:
            if anc in candidates:
                return anc
        return None


# --- rendering --------------------------------------------------------------


def truncate(text: str, width: int) -> str:
    text = " ".join(text.split())
    if width <= 0 or len(text) <= width:
        return text
    if width <= 1:
        return "\u2026"
    return text[: width - 1] + "\u2026"


def dwidth(text: str) -> int:
    """Display columns of a plain string (wide/emoji runes count as 2)."""
    n = 0
    for ch in text:
        if unicodedata.combining(ch):
            continue
        n += 2 if unicodedata.east_asian_width(ch) in ("W", "F") else 1
    return n


Segment = "tuple[str, str | None]"


def wrap(text: str, width: int) -> list[str]:
    """Word-wrap text to a display width (hard-splits over-long tokens)."""
    text = " ".join(text.split())
    if width <= 0:
        return [text]
    lines: list[str] = []
    cur = ""
    curw = 0
    for word in text.split():
        ww = dwidth(word)
        while ww > width:
            if cur:
                lines.append(cur)
                cur, curw = "", 0
            piece = ""
            pw = 0
            for ch in word:
                cw = 2 if unicodedata.east_asian_width(ch) in ("W", "F") else 1
                if pw + cw > width:
                    break
                piece += ch
                pw += cw
            lines.append(piece)
            word = word[len(piece) :]
            ww = dwidth(word)
        if curw == 0:
            cur, curw = word, ww
        elif curw + 1 + ww <= width:
            cur += " " + word
            curw += 1 + ww
        else:
            lines.append(cur)
            cur, curw = word, ww
    if cur:
        lines.append(cur)
    return lines or [""]


def wrap_segs(prefix: list[Segment], text: str, width: int) -> list[list[Segment]]:
    """Wrap text under a styled prefix, hanging-indenting continuations."""
    pw = sum(dwidth(t) for t, _ in prefix)
    avail = max(width - pw, 8)
    out: list[list[Segment]] = []
    for i, w in enumerate(wrap(text, avail)):
        pre = prefix if i == 0 else [(" " * pw, None)]
        out.append(list(pre) + [(w, None)])
    return out or [list(prefix)]


def parse_ts(iso: str):
    try:
        return datetime.fromisoformat(iso.replace("Z", "+00:00"))
    except (ValueError, AttributeError):
        return None


def rel_time(iso: str) -> str:
    dt = parse_ts(iso)
    if dt is None:
        return "?"
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    secs = (datetime.now(timezone.utc) - dt).total_seconds()
    if secs < 60:
        return "just now"
    if secs < 3600:
        return f"{int(secs // 60)}m ago"
    if secs < 86400:
        return f"{int(secs // 3600)}h ago"
    return f"{int(secs // 86400)}d ago"


def _utc(dt):
    if dt is None:
        return None
    return dt if dt.tzinfo else dt.replace(tzinfo=timezone.utc)


def start_ts(rec: dict):
    return parse_ts(rec.get("started_at") or rec.get("created_at") or "")


def end_ts(rec: dict):
    return parse_ts(rec.get("closed_at") or "")


def humanize_duration(secs: float) -> str:
    secs = max(int(secs), 0)
    days, rem = divmod(secs, 86400)
    hours, rem = divmod(rem, 3600)
    mins = rem // 60
    if days:
        return f"{days}d {hours}h"
    if hours:
        return f"{hours}h {mins}m"
    if mins:
        return f"{mins}m"
    return "<1m"


def bead_duration(rec: dict) -> str:
    """Elapsed time: started (or created) to closed (or now)."""
    s = _utc(start_ts(rec))
    if s is None:
        return ""
    e = _utc(end_ts(rec)) or datetime.now(timezone.utc)
    return humanize_duration((e - s).total_seconds())


def workstream_span(members: list[dict]) -> str:
    """Wall-clock span: earliest member start to now (or the last close)."""
    starts = [s for s in (_utc(start_ts(m)) for m in members) if s]
    if not starts:
        return ""
    start = min(starts)
    if any(m.get("status") != "closed" for m in members):
        end = datetime.now(timezone.utc)
    else:
        ends = [e for e in (_utc(end_ts(m)) for m in members) if e]
        end = max(ends) if ends else datetime.now(timezone.utc)
    return humanize_duration((end - start).total_seconds())


def effective_status(rec: dict) -> str:
    """A bead that is open but claimed (started_at) reads as 'claimed'."""
    st = rec.get("status", "open")
    if st == "open" and rec.get("started_at"):
        return "claimed"
    return st


def icon_seg(rec: dict) -> Segment:
    st = effective_status(rec)
    return (ICON.get(st, "?"), STYLE.get(st, ""))


def icon(rec: dict, paint: Paint) -> str:
    return paint(*icon_seg(rec))


class Renderer:
    def __init__(
        self,
        model: Model,
        paint: Paint,
        width: int,
        scope: str,
        layout: str = "box",
        columns: int = 0,
        max_rows: int = 12,
    ) -> None:
        self.model = model
        self.paint = paint
        self.width = max(width, 60)
        self.scope = scope
        self.layout = layout
        self.columns = columns
        self.max_rows = max_rows
        self.lines: list[str] = []

    def bead(self, rec: dict, title_w: int) -> str:
        p = self.paint
        dur = bead_duration(rec)
        tag = p(f" {dur}", DIM) if dur else ""
        tw = max(title_w - (len(dur) + 1 if dur else 0), 8)
        return (
            f"{icon(rec, p)} {p(rec['id'], BOLD)}{tag}  "
            f"{truncate(rec.get('title', ''), tw)}"
        )

    def in_scope(self, members: list[dict]) -> bool:
        statuses = {effective_status(m) for m in members}
        if self.scope == "working":
            return bool(statuses & {"in_progress", "claimed"})
        if self.scope == "attention":
            return bool(statuses & {"in_progress", "claimed", "blocked", "hooked"})
        return True

    def render(self) -> str:
        model, p = self.model, self.paint
        groups = model.groups()

        def stream_key(item):
            _origin, members = item
            st = collections.Counter(effective_status(m) for m in members)
            active = st.get("in_progress", 0) + st.get("claimed", 0)
            rank = 0 if active else (1 if st.get("blocked") else 2)
            return (rank, -len(members), _origin)

        ordered = sorted(groups.items(), key=stream_key)
        shown = [x for x in ordered if self.in_scope(x[1])]
        hidden = [x for x in ordered if not self.in_scope(x[1])]
        beads = [r for _o, m in shown for r in m]
        counts = collections.Counter(effective_status(r) for r in beads)

        stats = [
            (s, ICON.get(s, "?"), counts.get(s, 0))
            for s in ("in_progress", "claimed", "blocked", "open")
        ]
        plain_bar = "   ".join(f"{ic} {n} {s.replace('_', ' ')}" for s, ic, n in stats)
        bar = "   ".join(
            f"{p(ic, STYLE.get(s, ''))} {n} {s.replace('_', ' ')}"
            for s, ic, n in stats
        )
        noun = "workstream" if len(shown) == 1 else "workstreams"
        head = (
            f"{datetime.now().strftime('%Y-%m-%d %H:%M')}   "
            f"{len(shown)} {noun}   {len(beads)} beads"
        )
        # The bar is informative, not essential: drop it rather than
        # overflow a narrow terminal with a header that cannot wrap.
        if dwidth(head) + 3 + dwidth(plain_bar) <= self.width:
            head = f"{head}   {bar}"
        self.lines.append("")

        if not shown:
            if hidden:
                self.lines.append(
                    p(
                        f"nothing in progress \u00b7 {len(hidden)} idle workstream(s) "
                        f"(use --scope all)",
                        DIM,
                    )
                )
            else:
                self.lines.append(p("no active workstreams", DIM))
            return "\n".join(self.lines)

        work_items = model.work_items()
        allg = model.all_groups()
        if self.layout == "box":
            infos = [
                self.classify(
                    model.recs.get(oid, {"id": oid, "title": oid}),
                    members,
                    work_items,
                    allg.get(oid, members),
                )
                for oid, members in shown
            ]
            self.emit_boxes(infos)
        else:
            for origin_id, members in shown:
                origin = model.recs.get(origin_id, {"id": origin_id, "title": origin_id})
                self.stream(origin, members, work_items, allg.get(origin_id, members))

        if hidden:
            hb = sum(len(m) for _o, m in hidden)
            self.lines.append(
                p(
                    f"{len(hidden)} more workstream(s), {hb} beads \u00b7 --scope all",
                    DIM,
                )
            )
        return "\n".join(self.lines)

    def classify(
        self,
        origin: dict,
        members: list[dict],
        work_items: set[str],
        all_members: list[dict] | None = None,
    ) -> dict:
        """Resolve one workstream into the pieces both layouts draw."""
        model = self.model
        ids = {m["id"] for m in members}
        counts = collections.Counter(effective_status(m) for m in members)
        pool = members if all_members is None else all_members
        totals = collections.Counter(effective_status(m) for m in pool)

        top = [
            r
            for r in members
            if model.md(r, "gc.kind") == "workflow" and model.parent(r["id"]) == origin["id"]
        ]
        top_ids = {r["id"] for r in top}

        wi = [
            model.recs[i]
            for i in (work_items & ids)
            if i not in top_ids and i != origin["id"]
        ]
        wi_ids = {r["id"] for r in wi}

        focus = [
            m
            for m in members
            if effective_status(m) in ("in_progress", "claimed", "blocked")
            and m["id"] not in top_ids
            and m["id"] not in wi_ids
            and m["id"] != origin["id"]
        ]

        # Bucket focus leaves under their nearest work item.
        leaves_by_wi: dict[str | None, list[dict]] = collections.defaultdict(list)
        for leaf in focus:
            leaves_by_wi[model.nearest(leaf["id"], wi_ids)].append(leaf)

        # Work items with active work first, then by priority.
        wi.sort(
            key=lambda r: (
                0 if leaves_by_wi.get(r["id"]) else 1,
                r.get("priority", 9),
                r["id"],
            )
        )

        shown = set(top_ids) | {origin["id"]} | wi_ids | {m["id"] for m in focus}
        stamps = [m["updated_at"] for m in members if m.get("updated_at")]
        return {
            "origin": origin,
            "members": members,
            "counts": counts,
            "top": top,
            "top_ids": top_ids,
            "wi": wi,
            "focus": focus,
            "leaves_by_wi": leaves_by_wi,
            "rest": len(ids - shown),
            "compact": not top and not wi and not focus,
            "activity": rel_time(max(stamps)) if stamps else "?",
            "duration": workstream_span(pool),
            "totals": totals,
            "total": sum(totals.values()),
        }

    def stream(
        self,
        origin: dict,
        members: list[dict],
        work_items: set[str],
        all_members: list[dict] | None = None,
    ) -> None:
        model, p = self.model, self.paint
        info = self.classify(origin, members, work_items, all_members)
        st = info["counts"]
        top = info["top"]
        top_ids = info["top_ids"]
        wi = info["wi"]
        leaves_by_wi = info["leaves_by_wi"]

        # Nothing but the origin (and structural convoys): one compact line.
        if info["compact"]:
            detail = f" ({len(members)} active)" if len(members) > 1 else ""
            tag = origin.get("issue_type", "?")
            if info.get("duration"):
                tag += f" \u00b7 {info['duration']}"
            if info["total"]:
                tag += f" \u00b7 {self.progress_str(info)}"
            self.lines.append(
                f"{icon(origin, p)} {p(origin['id'], BOLD)} {p(DOT, DIM)} "
                f"{truncate(origin.get('title', ''), self.width - 32)}{detail} "
                f"{p('[' + tag + ']', DIM)}"
            )
            return

        self.lines.append(self.bead(origin, self.width - 6))
        summary = ", ".join(
            f"{st[s]} {s.replace('_', ' ')}"
            for s in ("in_progress", "claimed", "blocked", "open")
            if st.get(s)
        )
        span = f" \u00b7 span {info['duration']}" if info.get("duration") else ""
        self.lines.append(
            p(
                f"   {origin.get('issue_type', '?')} \u00b7 {len(members)} active ({summary}){span}",
                DIM,
            )
        )
        if info["total"]:
            self.lines.append(
                "   " + self.seg(self.progress_segs(info, self.width - 4))
            )

        loose = leaves_by_wi.get(None, [])
        if top:
            for ti, root in enumerate(top):
                rchildren = [w for w in wi if model.nearest(w["id"], top_ids) == root["id"]]
                rfocus = [
                    leaf
                    for leaf in loose
                    if model.nearest(leaf["id"], top_ids) == root["id"]
                ]
                formula = model.md(root, "gc.formula_name") or "workflow"
                self.lines.append(
                    f"   {p(GEAR, DIM)} {icon(root, p)} {p(root['id'], BOLD)}  "
                    f"{p(formula, DIM)}"
                )
                entries: list[tuple[dict, list[dict]]] = [
                    (w, leaves_by_wi.get(w["id"], [])) for w in rchildren
                ]
                entries += [(leaf, []) for leaf in rfocus]
                for ei, (rec, children) in enumerate(entries):
                    last = ei == len(entries) - 1
                    conn = "\u2514\u2500" if last else "\u251c\u2500"
                    self.lines.append(
                        f"      {conn} {self.bead(rec, self.width - 34)}"
                    )
                    for ci, child in enumerate(children):
                        clast = ci == len(children) - 1
                        cconn = "\u2514\u2500" if clast else "\u251c\u2500"
                        cont = "   " if last else "\u2502  "
                        self.lines.append(
                            f"      {cont}{cconn} {self.bead(child, self.width - 40)}"
                        )
        # Work items and focus beads with no workflow ancestor stay visible at
        # the origin: their nearest workflow root is None. Without a workflow
        # root, every work item and loose focus bead belongs here.
        if top:
            entries = [
                (w, leaves_by_wi.get(w["id"], []))
                for w in wi
                if model.nearest(w["id"], top_ids) is None
            ]
            entries += [
                (leaf, [])
                for leaf in loose
                if model.nearest(leaf["id"], top_ids) is None
            ]
        else:
            entries = [(w, leaves_by_wi.get(w["id"], [])) for w in wi]
            entries += [(leaf, []) for leaf in loose]
        for ei, (rec, children) in enumerate(entries):
            last = ei == len(entries) - 1
            conn = "\u2514\u2500" if last else "\u251c\u2500"
            self.lines.append(f"   {conn} {self.bead(rec, self.width - 30)}")
            for ci, child in enumerate(children):
                cconn = "\u2514\u2500" if ci == len(children) - 1 else "\u251c\u2500"
                cont = "   " if last else "\u2502  "
                self.lines.append(
                    f"   {cont}{cconn} {self.bead(child, self.width - 36)}"
                )

        rest = info["rest"]
        if rest:
            indent = "      " if top else "   "
            self.lines.append(
                p(f"{indent}\u2514\u2500 \u2026 {rest} more open (steps, convoys)", DIM)
            )
        self.lines.append("")

    def seg(self, segs: list[Segment]) -> str:
        p = self.paint
        return "".join(p(t, st) if st else t for t, st in segs)

    def progress_str(self, info: dict) -> str:
        total = info["total"]
        done = info["totals"].get("closed", 0)
        pct = int(round(100 * done / total)) if total else 0
        return f"{pct}% \u00b7 {done}/{total}"

    def progress_segs(self, info: dict, width: int) -> list[Segment]:
        """A proportional bar of closed/in_progress/blocked/open over all beads."""
        totals = info["totals"]
        total = info["total"]
        done = totals.get("closed", 0)
        pct = int(round(100 * done / total)) if total else 0
        label = f" {pct}% \u00b7 {done}/{total}"
        barw = max(width - dwidth(label), 4)
        order = (
            "closed",
            "in_progress",
            "blocked",
            "claimed",
            "open",
            "hooked",
            "deferred",
            "pinned",
        )
        entries = [(s, totals.get(s, 0)) for s in order if totals.get(s, 0)]
        cells: list[int] = []
        frac: list[float] = []
        for _s, c in entries:
            exact = c * barw / total if total else 0.0
            cells.append(int(exact))
            frac.append(exact - int(exact))
        leftover = barw - sum(cells)
        for i in sorted(range(len(cells)), key=lambda i: frac[i], reverse=True)[
            :leftover
        ]:
            cells[i] += 1
        segs: list[Segment] = []
        for (s, _c), n in zip(entries, cells):
            if n:
                segs.append((BAR_CHAR.get(s, "\u2591") * n, STYLE.get(s, DIM)))
        used = sum(cells)
        if used < barw:
            segs.append(("\u2591" * (barw - used), DIM))
        segs.append((label, DIM))
        return segs

    def emit_boxes(self, infos: list[dict]) -> None:
        min_cell = 28  # box_cell draws at least 24 inner columns plus borders
        cols = self.columns or (2 if self.width >= 118 else 1)
        gap = 2
        # Cap the column count to the boxes that fit: boxes do not shrink
        # below min_cell, so a wider request would overflow the row.
        fits = max(1, (self.width + gap) // (min_cell + gap))
        cols = max(1, min(cols, fits, len(infos)))
        cell_w = max((self.width - gap * (cols - 1)) // cols, min_cell)
        cells = [self.box_cell(info, cell_w) for info in infos]
        # One uniform cell size: pad every box above its bottom border.
        height = max(len(c) for c in cells)
        inner = cell_w - 4
        blank = self.paint("\u2502 ", DIM) + " " * inner + self.paint(" \u2502", DIM)
        for c in cells:
            pad = height - len(c)
            if pad:
                c[-1:-1] = [blank] * pad
        for ri in range(0, len(cells), cols):
            if ri:
                self.lines.append("")
            row = cells[ri : ri + cols]
            for i in range(height):
                self.lines.append((" " * gap).join(c[i] for c in row))

    def box_cell(self, info: dict, width: int) -> list[str]:
        p = self.paint
        origin = info["origin"]
        inner = max(width - 4, 24)
        hid = truncate(origin["id"], inner - 2)
        ic = icon_seg(origin)[0]
        hw = dwidth(ic) + 1 + dwidth(hid)
        dashes = max(inner - 1 - hw, 1)
        lines = [
            p("\u256d\u2500 ", DIM)
            + icon(origin, p)
            + " "
            + p(hid, BOLD)
            + " "
            + p("\u2500" * dashes + "\u256e", DIM)
        ]

        def emit(segs: list[Segment]) -> None:
            body = "".join(p(t, st) if st else t for t, st in segs)
            vis = sum(dwidth(t) for t, _ in segs)
            lines.append(
                p("\u2502 ", DIM)
                + body
                + " " * max(inner - vis, 0)
                + p(" \u2502", DIM)
            )

        def paragraph(text: str, style: str | None = None) -> None:
            for w in wrap(text, inner):
                emit([(w, style)])

        paragraph(origin.get("title", ""), BOLD)
        counts = info["counts"]
        span = f" \u00b7 span {info['duration']}" if info.get("duration") else ""
        paragraph(
            f"{origin.get('issue_type', '?')} \u00b7 {info['total']} beads"
            f"{span} \u00b7 last active {info['activity']}",
            DIM,
        )
        if info["total"]:
            emit(self.progress_segs(info, inner))
        summary = ", ".join(
            f"{counts[s]} {s.replace('_', ' ')}"
            for s in ("in_progress", "claimed", "blocked", "open")
            if counts.get(s)
        )
        if summary:
            paragraph(f"active: {summary}", DIM)
        emit([])

        body = self.box_body(info, inner)
        if len(body) > self.max_rows:
            kept = self.max_rows - 1
            dropped = len(body) - kept
            body = body[:kept] + wrap_segs([], f"\u2026 {dropped} more lines", inner)
        for segs in body:
            emit(segs)

        lines.append(p("\u2570" + "\u2500" * (inner + 2) + "\u256f", DIM))
        return lines

    def box_body(self, info: dict, inner: int) -> list[list[Segment]]:
        model = self.model
        out: list[list[Segment]] = []

        def add(prefix: list[Segment], text: str, style: str | None = None) -> None:
            for segs in wrap_segs(prefix, text, inner):
                if style is not None:
                    segs = segs[:-1] + [(segs[-1][0], style)]
                out.append(segs)

        def bead_prefix(rec: dict, indent: int) -> list[Segment]:
            dur = bead_duration(rec)
            return [
                ("  " * indent, None),
                icon_seg(rec),
                (" ", None),
                (rec["id"], BOLD),
                (f"  {dur}", DIM) if dur else ("", None),
                ("  ", None),
            ]

        top = info["top"]
        top_ids = info["top_ids"]
        wi = info["wi"]
        leaves = info["leaves_by_wi"]
        loose = leaves.get(None, [])
        if top:
            for root in top:
                formula = model.md(root, "gc.formula_name") or "workflow"
                add(
                    [
                        ("\u2699 ", DIM),
                        icon_seg(root),
                        (" ", None),
                        (root["id"], BOLD),
                        ("  ", None),
                    ],
                    formula,
                    DIM,
                )
                children = [
                    w
                    for w in wi
                    if model.nearest(w["id"], top_ids) == root["id"]
                ]
                for w in children:
                    add(bead_prefix(w, 0), w.get("title", ""))
                    for leaf in leaves.get(w["id"], []):
                        add(bead_prefix(leaf, 1), leaf.get("title", ""))
                for leaf in loose:
                    if model.nearest(leaf["id"], top_ids) == root["id"]:
                        add(bead_prefix(leaf, 1), leaf.get("title", ""))
            for w in wi:
                if model.nearest(w["id"], top_ids) is not None:
                    continue
                add(bead_prefix(w, 0), w.get("title", ""))
                for leaf in leaves.get(w["id"], []):
                    add(bead_prefix(leaf, 1), leaf.get("title", ""))
            for leaf in loose:
                if model.nearest(leaf["id"], top_ids) is None:
                    add(bead_prefix(leaf, 0), leaf.get("title", ""))
        else:
            for w in wi:
                add(bead_prefix(w, 0), w.get("title", ""))
                for leaf in leaves.get(w["id"], []):
                    add(bead_prefix(leaf, 1), leaf.get("title", ""))
            for leaf in loose:
                add(bead_prefix(leaf, 0), leaf.get("title", ""))

        if info["rest"]:
            add([], f"\u2026 {info['rest']} more open", DIM)
        return out


# --- data loading -----------------------------------------------------------


def list_rigs(gc: str) -> list[dict]:
    """Rigs the city knows about, via `gc rig list --json` (empty on failure)."""
    try:
        proc = subprocess.run(
            [gc, "rig", "list", "--json"],
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            text=True,
        )
    except (FileNotFoundError, OSError):
        return []
    if proc.returncode != 0:
        return []
    try:
        data = json.loads(proc.stdout)
    except json.JSONDecodeError:
        return []
    if isinstance(data, dict):
        data = data.get("rigs", [])
    return [r for r in data if isinstance(r, dict) and r.get("path")]


def find_rig(cwd: str, gc: str = "gc") -> str | None:
    """The rig whose directory contains `cwd` (longest match), or None.

    `gc rig list` runs in this process's working directory, so it resolves the
    city the operator is standing in. No rig argument means "the rig under the
    current directory".
    """
    cwd = os.path.realpath(cwd)
    best: str | None = None
    for rig in list_rigs(gc):
        path = os.path.realpath(rig["path"])
        if cwd == path or cwd.startswith(path + os.sep):
            if best is None or len(path) > len(best):
                best = path
    return best


def run_bd(bd: str, bd_dir: str | None, args: list[str]) -> str:
    cmd = [bd]
    if bd_dir:
        cmd += ["-C", bd_dir]
    cmd += args
    try:
        proc = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    except FileNotFoundError:
        sys.exit(f"dashboard: {bd!r} not found on PATH")
    if proc.returncode != 0:
        sys.stderr.write(proc.stderr)
        sys.exit(f"dashboard: {' '.join(cmd)} failed ({proc.returncode})")
    return proc.stdout


def load_records(bd: str, bd_dir: str | None) -> dict:
    raw = run_bd(bd, bd_dir, ["export"])
    recs: dict = {}
    for line in raw.splitlines():
        line = line.strip()
        if not line:
            continue
        rec = json.loads(line)
        if rec.get("_type") == "issue":
            recs[rec["id"]] = rec
    return recs


def rig_label(bd: str, bd_dir: str | None) -> str:
    if bd_dir:
        return os.path.basename(os.path.abspath(bd_dir)) or bd_dir
    try:
        out = run_bd(bd, None, ["context"])
    except SystemExit:
        return os.path.basename(os.getcwd())
    root = database = ""
    for line in out.splitlines():
        line = line.strip()
        if line.startswith("repo root:"):
            root = line.split(":", 1)[1].strip()
        elif line.startswith("database:"):
            database = line.split(":", 1)[1].strip()
    name = os.path.basename(root) if root else os.path.basename(os.getcwd())
    return f"{name} ({database})" if database else name


def active_statuses_for(args, respect_scope: bool = True) -> tuple[str, ...]:
    if args.all:
        args.scope = "all"
        return (
            "open",
            "in_progress",
            "blocked",
            "deferred",
            "closed",
            "pinned",
            "hooked",
        )
    chosen = args.status or "open,in_progress,blocked"
    statuses = tuple(s.strip() for s in chosen.split(",") if s.strip())
    if (
        respect_scope
        and args.scope == "attention"
        and args.status is None
        and "hooked" not in statuses
    ):
        # --scope attention promises hooked work; the default set omits it.
        statuses += ("hooked",)
    return statuses


def detect_color(args) -> bool:
    return (
        not args.no_color
        and sys.stdout.isatty()
        and os.environ.get("NO_COLOR") is None
        and os.environ.get("TERM") != "dumb"
    )


def render_frame(args, paint: Paint, label: str) -> str:
    model = Model(load_records(args.bd, args.rig), active_statuses_for(args))
    width = args.width or shutil.get_terminal_size((100, 24)).columns
    body = Renderer(
        model, paint, width, args.scope, args.layout, args.columns, args.max_rows
    ).render()
    return f"{paint(label, BOLD)}\n{body}"


def wait_for_key(seconds: float, fd: int | None, resized: dict) -> str:
    """Wait up to `seconds`. Return 'quit', 'redraw', or 'timeout'."""
    deadline = time.monotonic() + seconds
    while True:
        if resized["flag"]:
            resized["flag"] = False
            return "redraw"
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            return "timeout"
        if fd is None:
            time.sleep(min(remaining, 0.2))
            continue
        ready, _, _ = select.select([fd], [], [], min(remaining, 0.2))
        if not ready:
            continue
        key = os.read(fd, 1).decode("utf-8", "ignore").lower()
        if key == "q":
            return "quit"
        if key == "r":
            return "redraw"


def watch(args, paint: Paint, label: str) -> int:
    """Re-render on a timer; q quits, r refreshes, SIGWINCH redraws."""
    out = sys.stdout
    on_tty = out.isatty()
    if not on_tty and not args.frames:
        # No screen to clear and no frame cap: emit one snapshot and stop.
        print(render_frame(args, paint, label))
        return 0

    stdin = sys.stdin
    key_mode = on_tty and stdin.isatty() and _termios is not None
    fd = stdin.fileno() if key_mode else None
    old_attrs = None
    old_winch = None
    screen = False
    shown = 0
    resized = {"flag": False}

    def on_winch(_signum, _frame):
        resized["flag"] = True

    # The try starts before any setup: a Ctrl-C between two setup steps, or
    # inside the first flush, must still restore what was already acquired.
    try:
        if on_tty and hasattr(signal, "SIGWINCH"):
            try:
                old_winch = signal.signal(signal.SIGWINCH, on_winch)
            except ValueError:
                old_winch = None
        if key_mode:
            old_attrs = _termios.tcgetattr(fd)
            _tty.setcbreak(fd)  # keep ISIG, so Ctrl-C still works
        if on_tty:
            screen = True
            out.write("\x1b[?1049h\x1b[?25l")  # alternate screen, hide cursor
            out.flush()
        while True:
            frame = render_frame(args, paint, label)
            if on_tty:
                out.write("\x1b[H\x1b[2J")  # home + clear
            out.write(frame + "\n")
            out.flush()
            shown += 1
            if args.frames and shown >= args.frames:
                break
            if wait_for_key(args.interval, fd, resized) == "quit":
                break
    except KeyboardInterrupt:
        pass
    finally:
        if key_mode and old_attrs is not None:
            _termios.tcsetattr(fd, _termios.TCSADRAIN, old_attrs)
        if screen:
            out.write("\x1b[?25h\x1b[?1049l")  # show cursor, leave alt screen
            out.flush()
        if old_winch is not None:
            signal.signal(signal.SIGWINCH, old_winch)
    return 0


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(
        description="Visualize the workstreams a Gas City rig is working on.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    ap.add_argument(
        "-C",
        "--rig",
        metavar="DIR",
        help=(
            "rig directory (passed to bd -C); default: the rig that contains "
            "the current directory, from `gc rig list`"
        ),
    )
    ap.add_argument("--bd", default="bd", help="bd binary (default: bd)")
    ap.add_argument(
        "--status",
        default=None,
        help=(
            "statuses treated as active (default: open,in_progress,blocked; "
            "--scope attention also adds hooked)"
        ),
    )
    ap.add_argument(
        "--scope",
        choices=("working", "attention", "all"),
        default="working",
        help=(
            "which workstreams the terminal view shows: working = has an "
            "in_progress bead (default); attention = working + blocked/hooked "
            "(hooked is added unless --status is given); all = every stream "
            "with active beads. --json ignores --scope."
        ),
    )
    ap.add_argument(
        "--all",
        action="store_true",
        help="show everything, including closed beads (implies --scope all)",
    )
    ap.add_argument(
        "--layout",
        choices=("box", "tree"),
        default="box",
        help="box = bordered lanes per workstream (default); tree = indented tree",
    )
    ap.add_argument(
        "--columns",
        type=int,
        default=0,
        help="box columns (default: auto, 2 when wide enough)",
    )
    ap.add_argument(
        "--max-rows",
        type=int,
        default=12,
        help="max body rows per box before folding (default: 12)",
    )
    ap.add_argument(
        "-w",
        "--watch",
        action="store_true",
        help="re-render on an interval; q quits, r refreshes (Ctrl-C also quits)",
    )
    ap.add_argument(
        "--interval",
        type=float,
        default=60,
        help="seconds between refreshes in --watch (default: 60)",
    )
    ap.add_argument(
        "--frames",
        type=int,
        default=0,
        help="with --watch, render N frames then exit (default: until Ctrl-C)",
    )
    ap.add_argument(
        "--json",
        action="store_true",
        help=(
            "emit the unfiltered origin → bead ids model as JSON "
            "(--scope affects the terminal view only)"
        ),
    )
    ap.add_argument("--no-color", action="store_true", help="disable ANSI colors")
    ap.add_argument("--width", type=int, default=0, help="output width (default: terminal)")
    args = ap.parse_args(argv)
    if not math.isfinite(args.interval) or args.interval <= 0:
        ap.error("--interval must be a positive number of seconds")

    if args.rig is None:
        args.rig = find_rig(os.getcwd(), os.environ.get("GC_BIN") or "gc")

    paint = Paint(detect_color(args))

    if args.json:
        # --scope only shapes the terminal view, so the JSON model keeps the
        # same membership for every scope.
        model = Model(
            load_records(args.bd, args.rig),
            active_statuses_for(args, respect_scope=False),
        )
        payload = {
            origin: sorted(m["id"] for m in members)
            for origin, members in model.groups().items()
        }
        json.dump(payload, sys.stdout, indent=2, sort_keys=True)
        sys.stdout.write("\n")
        return 0

    label = rig_label(args.bd, args.rig)
    if args.watch:
        return watch(args, paint, label)
    print(render_frame(args, paint, label))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
