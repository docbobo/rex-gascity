package provider

import (
	"path"
	"strings"
	"unicode/utf8"
)

// launchIdentity is the naming view of one start: given the projected start
// env, the tab label, pane label, aliases, legacy labels and the claim's
// session id. It is the only code that reads the GC_* naming keys, so a naming
// rule changes here and not at each read site (#136).
type launchIdentity struct {
	name SessionName
	env  map[string]string
	town string
}

// newLaunchIdentity reads the names a start of name carries under env. town is
// the town-level tab label.
func newLaunchIdentity(name SessionName, env map[string]string, town string) launchIdentity {
	return launchIdentity{name: name, env: env, town: town}
}

// TabLabel is the Rex tab label the start opens or joins.
func (i launchIdentity) TabLabel() string { return tabLabel(i.name, i.env, i.town) }

// PaneLabel is the Rex pane label the start carries.
func (i launchIdentity) PaneLabel() string { return paneLabelFor(i.name, i.env) }

// Aliases are the extra gc session names the pane answers to.
func (i launchIdentity) Aliases() []SessionName { return aliasesFor(i.name, i.env) }

// IsTownTab reports whether the start takes the town tab.
func (i launchIdentity) IsTownTab() bool { return isTownStart(i.name, i.env) }

// ClaimSessionID is the gc session bead id the start claims under.
func (i launchIdentity) ClaimSessionID() string { return i.env["GC_SESSION_ID"] }

// LegacyTabLabels lists the tab labels a record or pane from before the
// GCREX-72 rule may carry: the name-derived label first, then the env-derived
// one. Duplicates are dropped.
func (i launchIdentity) LegacyTabLabels() []string {
	tabs := []string{legacyTabLabelFor(i.name, nil)}
	return appendLabel(tabs, legacyTabLabelFor(i.name, i.env))
}

// LegacyPaneLabel is the pane label a pane started before GC_TEMPLATE carries.
func (i launchIdentity) LegacyPaneLabel() string { return legacyPaneLabelFor(i.name, i.env) }

// providerName is the provider whose pack overlay a start stages. It lives here
// with the other start-env reads so callers never index the map themselves.
func providerName(env map[string]string) string { return env["GC_PROVIDER"] }

// isTownStart reports whether a start takes tabLabel's town branch: a
// town-style name and no GC_RIG in the projected start env. A town-style
// name with GC_RIG set (a wisp instance) belongs to its rig tab. Compare the
// classification, never the labels: a pin can equal a rig label, and the tab
// label is clamped while the pin is not (#47).
func isTownStart(name SessionName, env map[string]string) bool {
	return env["GC_RIG"] == "" && name.IsTown()
}

// tabLabel derives the Rex tab label for a started session: "[rig] <GC_RIG>"
// whenever GC_RIG is non-empty, even for a town-style name with no "--" (a wisp
// instance name). Only with no GC_RIG does the name shape decide: the tagged
// town label for a town-level name, otherwise "[rig] " plus the name split.
// Every result is clamped to maxRexLabel.
func tabLabel(name SessionName, env map[string]string, town string) string {
	if isTownStart(name, env) {
		return townTabLabel(town)
	}
	if rig := env["GC_RIG"]; rig != "" {
		return clampRexLabel(taggedLabel(labelRig, rig))
	}
	return clampRexLabel(taggedLabel(labelRig, name.Rig()))
}

// legacyRigTabFor is the tab label the provider derived before the bracketed
// tags: "Rig: <rig>". A window started by an older build still carries it, so
// resolution looks for it as well. Empty for a town-level name with no GC_RIG.
// The old tab label was clamped to maxRexLabel, so the candidate is clamped
// too: for a rig of 124 or more runes the stored tab ends in "…" and an
// unclamped candidate would never match it.
func legacyRigTabFor(n SessionName, env map[string]string) string {
	if rig := env["GC_RIG"]; rig != "" {
		return clampRexLabel(legacyRigTabPrefix + rig)
	}
	if rig := n.Rig(); rig != "" {
		return clampRexLabel(legacyRigTabPrefix + rig)
	}
	return ""
}

// legacyTabLabel is the tab label gc wrote before GCREX-72: "__town" for a
// town-level name, otherwise the bare rig. A record with no stored TabLabel or
// a window that predates the upgrade still carries it, so resolution accepts it
// as well as the derived label.
func legacyTabLabel(n SessionName) string { return legacyTabLabelFor(n, nil) }

// legacyTabLabelFor is the label the pre-GCREX-72 provider wrote for n: a
// non-empty GC_RIG from the start env, otherwise __town for a town-level name
// or the name-derived rig.
func legacyTabLabelFor(n SessionName, env map[string]string) string {
	// The old tabLabelFor read GC_RIG first, so a short pool name such as
	// dog-1 sat in its rig tab, not __town.
	if rig := env["GC_RIG"]; rig != "" {
		return rig
	}
	if n.IsTown() {
		return legacyTownTab
	}
	return n.Rig()
}

// paneLabelFor returns the Rex pane label for a started session. See
// paneLabelForAgent: it is the one writer of the pane grammar.
func paneLabelFor(n SessionName, env map[string]string) string {
	return paneLabelForAgent(n, env, paneLabelAgent(env))
}

// legacyPaneLabelFor is the label the pre-GC_TEMPLATE rule produced: the agent
// came from GC_ALIAS/GC_AGENT, not GC_TEMPLATE, and the suffix was always the
// full GC_SESSION_NAME — the pool short form did not exist yet, so it must not
// be applied here even when the current name shape would verify it. It is the
// label a pane started before the change still carries, so an upgrade can
// recognise it.
func legacyPaneLabelFor(n SessionName, env map[string]string) string {
	if env["GC_RIG"] == "" && n.IsTown() {
		return n.PaneLabel()
	}
	session := strings.TrimSpace(env["GC_SESSION_NAME"])
	agent := paneAgent(env)
	if session == "" || agent == "" {
		return n.PaneLabel()
	}
	// Pre-change truncation: cut the agent without the shortened marker. A
	// pane started before the change carries that unmarked cut, so the legacy
	// label must reproduce it exactly or the upgrade misses the live pane.
	tail := " (" + session + ")"
	budget := maxPaneLabelRunes - utf8.RuneCountInString(tail)
	if budget < 1 {
		return clampRexLabel(session)
	}
	if r := []rune(agent); len(r) > budget {
		agent = string(r[:budget])
	}
	return agent + tail
}

// paneLabelForAgent is the one pane-label writer. A town-level start renders
// "[agent] (<name>)" with no tail. An unaliased pool seat whose name the env
// proves (Sanitize(agent)+"-"+pool identity equals GC_SESSION_NAME, gc's
// PoolSessionName shape) gets the bracketed tag, the marked pool identity
// "(#<bead id>)" in parentheses and the display-only "#<session id>" tail.
// Every other rig-scoped session keeps "[agent] (<GC_SESSION_NAME>) #<id>".
// The two forms shorten differently, so each states its own ladder below.
func paneLabelForAgent(n SessionName, env map[string]string, agent string) string {
	if isTownStart(n, env) {
		return townPaneLabel(agent, n)
	}
	session := strings.TrimSpace(env["GC_SESSION_NAME"])
	if session == "" || agent == "" {
		return n.PaneLabel()
	}
	id := paneSessionID(env)
	// The safety guard: only claim the pool form when the name verifies it.
	// gc has changed the name shape before, so a failed check degrades to the
	// long form rather than to a guess.
	if env["GC_ALIAS"] == "" {
		if pool := panePoolSuffix(env); pool != "" && sanitizeQualified(agent)+"-"+pool == session {
			if label, ok := fitPoolPaneLabel(agent, pool, id); ok {
				return label
			}
			// The pool form cannot fit: the long form names the session by
			// its suffix, so it is the one that survives a shortened agent.
		}
	}
	if label, ok := fitLongPaneLabel(agent, session, id); ok {
		return label
	}
	return clampRexLabel(session)
}

// paneSessionID is the display id a pane label carries after "#": the claim's
// GC_SESSION_ID, never an identity.
func paneSessionID(env map[string]string) string { return strings.TrimSpace(env["GC_SESSION_ID"]) }

// townPaneLabel is a town-level pane label: "[agent] (<name>)". The session
// name is the agent, so there is no separate identity for the parentheses and
// no display id to append. It obeys the same 128-rune limit as the rig branch:
// the long form when it fits, otherwise the agent is shortened, and a label
// that still cannot fit falls back to the clamped bare name.
func townPaneLabel(agent string, n SessionName) string {
	if agent == "" {
		return n.PaneLabel()
	}
	if label, ok := fitLongPaneLabel(agent, n.String(), ""); ok {
		return label
	}
	return clampRexLabel(n.String())
}

// fitAgent fits agent into budget runes, marking a cut with paneLabelShortened
// so the label never reads like a whole agent. ok is false when not even the
// marker fits.
func fitAgent(agent string, budget int) (string, bool) {
	r := []rune(agent)
	if len(r) <= budget {
		return agent, true
	}
	if budget < 2 {
		return "", false
	}
	return string(r[:budget-1]) + paneLabelShortened, true
}

// fitPoolPaneLabel renders the pool form at or under Rex's limit. The agent
// and the parenthesised pool identity both name the session, so neither is cut
// here; the "#<id>" tail is display-only, so it is the one part that may be
// dropped. The identity is marked with paneLabelShortForm, which is what keeps
// the pool form from colliding with a session literally named the pool id. The
// ladder is therefore "[agent] (#pool) #id", then "[agent] (#pool)". A pool
// identity that does not fit returns ok=false and the caller falls back to the
// long form, whose identity is the whole name.
func fitPoolPaneLabel(agent, pool, id string) (string, bool) {
	suffix := paneLabelShortForm + pool
	if id != "" {
		if label := newPaneParts(agent, suffix, id).String(); utf8.RuneCountInString(label) <= maxPaneLabelRunes {
			return label, true
		}
	}
	if label := newPaneParts(agent, suffix, "").String(); utf8.RuneCountInString(label) <= maxPaneLabelRunes {
		return label, true
	}
	return "", false
}

// fitLongPaneLabel renders the long form at or under Rex's limit. Here the
// suffix is the whole session name, so shortening the agent keeps the
// identity and is preferable to dropping the display id. The fixed ladder is
// "[agent] (name) #id", "[agent…] (name) #id", "[agent] (name)",
// "[agent…] (name)": a name that can survive does survive.
func fitLongPaneLabel(agent, name, id string) (string, bool) {
	// A complete session name beginning with the pool mark would read back as
	// the marked pool identity: "[worker] (#seat)" names worker-seat to
	// namesSession, not the session literally called "#seat". The long form
	// cannot carry such a name unambiguously, so it declines; the caller falls
	// back to the bare clamped name, which discovery still matches as the
	// expected pane label (name.PaneLabel()) rather than as a pool seat.
	if strings.HasPrefix(name, paneLabelShortForm) {
		return "", false
	}
	ids := []string{id}
	if id != "" {
		ids = append(ids, "")
	}
	for _, tailID := range ids {
		if label := newPaneParts(agent, name, tailID).String(); utf8.RuneCountInString(label) <= maxPaneLabelRunes {
			return label, true
		}
		overhead := 2 + 3 + utf8.RuneCountInString(name)
		if tailID != "" {
			overhead += 2 + utf8.RuneCountInString(tailID)
		}
		cut, ok := fitAgent(agent, maxPaneLabelRunes-overhead)
		if !ok {
			continue
		}
		if label := newPaneParts(cut, name, tailID).String(); utf8.RuneCountInString(label) <= maxPaneLabelRunes {
			return label, true
		}
	}
	return "", false
}

// paneLabelAgent is the display agent: the agent part of GC_TEMPLATE (the
// readable identity <rig>/<agent>) when the start config carries it, else
// paneAgent. GC_TEMPLATE is not rewritten by pool routing.
func paneLabelAgent(env map[string]string) string {
	if t := strings.TrimSpace(env["GC_TEMPLATE"]); t != "" {
		return path.Base(t)
	}
	return paneAgent(env)
}

// panePoolSuffix is the unaliased pool session's bead id: GC_AGENT (the session
// bead id AssigneeIdentifier stamps), else GC_SESSION_ID, else the BEADS_ACTOR
// segment after the first "/". GC_AGENT comes first because pool routing
// rewrites BEADS_ACTOR to GC_TEMPLATE. A qualified GC_AGENT (containing "/" or
// ".") is an alias/configured identity, never a bead id, so it is skipped.
func panePoolSuffix(env map[string]string) string {
	if v := strings.TrimSpace(env["GC_AGENT"]); v != "" && !strings.ContainsAny(v, "/.") {
		return v
	}
	if v := strings.TrimSpace(env["GC_SESSION_ID"]); v != "" {
		return v
	}
	return paneAgent(map[string]string{"BEADS_ACTOR": env["BEADS_ACTOR"]})
}

// paneAgent resolves the display agent: GC_ALIAS, then GC_AGENT, then the
// BEADS_ACTOR segment after the first "/"; empty when none resolves.
func paneAgent(env map[string]string) string {
	if v := strings.TrimSpace(env["GC_ALIAS"]); v != "" {
		return v
	}
	if v := strings.TrimSpace(env["GC_AGENT"]); v != "" {
		return v
	}
	actor := strings.TrimSpace(env["BEADS_ACTOR"])
	if i := strings.Index(actor, "/"); i >= 0 {
		actor = strings.TrimSpace(actor[i+1:])
	}
	return actor
}

// aliasesFor lists the extra gc session names a started pane answers to,
// derived from the start-config env. Names are sanitized, deduped, and any
// equal to the start name (the primary) are dropped.
func aliasesFor(name SessionName, env map[string]string) []SessionName {
	seen := map[SessionName]bool{name: true}
	var out []SessionName
	add := func(v string) {
		if v == "" {
			return
		}
		sn := SessionName(v)
		if seen[sn] {
			return
		}
		seen[sn] = true
		out = append(out, sn)
	}
	if t := env["GC_TEMPLATE"]; t != "" {
		add(sanitizeQualified(t))
	}
	if a := env["GC_ALIAS"]; a != "" {
		add(sanitizeQualified(a))
	}
	if g := env["GC_AGENT"]; g != "" && strings.ContainsAny(g, "/.") {
		add(sanitizeQualified(g))
	}
	if rig := env["GC_RIG"]; rig != "" && name.IsTown() {
		add(sanitizeQualified(rig + "/" + string(name)))
	}
	return out
}
