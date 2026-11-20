package provider

import (
	"path"
	"strings"
	"unicode/utf8"
)

// nameSep separates the rig from the agent in a gc session name.
const nameSep = "--"

// townTabFallback labels the town tab when the process has no city pin.
const townTabFallback = "Gas City"

// legacyTownTab is the tab label town-level sessions had before GCREX-72.
const legacyTownTab = "__town"

// maxRexLabel is Rex's limit for session/window/block labels.
const maxRexLabel = 128

// SessionName is a gc session name. Rig agents are "<rig>--<agent>"; a name
// without "--" is town-level. It is comparable and usable as a map key.
type SessionName string

// Rig returns the rig part, or "" for town-level names.
func (n SessionName) Rig() string {
	if i := strings.Index(string(n), nameSep); i >= 0 {
		return string(n)[:i]
	}
	return ""
}

// Agent returns the agent part: everything after the first "--", or the whole
// name for town-level names.
func (n SessionName) Agent() string {
	if i := strings.Index(string(n), nameSep); i >= 0 {
		return string(n)[i+len(nameSep):]
	}
	return string(n)
}

// IsTown reports whether the name is town-level (has no "--").
func (n SessionName) IsTown() bool {
	return !strings.Contains(string(n), nameSep)
}

// rexSessionLabel is the Rex session label for city (one session per city).
func rexSessionLabel(city string) string { return "gc:" + city }

// townTabLabel returns the town-level tab label: the pin itself, or
// townTabFallback when there is no pin. The label is the pin, never "default"
// merely because CityFromEnv fell back to it: town-ness is the pin being empty.
func townTabLabel(pin string) string {
	if pin == "" {
		return townTabFallback
	}
	return pin
}

// tabLabel derives the Rex tab label for a started session: "Rig: <GC_RIG>"
// whenever GC_RIG is non-empty, even for a town-style name with no "--" (a wisp
// instance name). Only with no GC_RIG does the name shape decide: the town label
// for a town-level name, otherwise "Rig: " plus the name split. Every result is
// clamped to maxRexLabel.
func tabLabel(name SessionName, env map[string]string, town string) string {
	if rig := env["GC_RIG"]; rig != "" {
		return clampRexLabel("Rig: " + rig)
	}
	if name.IsTown() {
		return clampRexLabel(town)
	}
	return clampRexLabel("Rig: " + name.Rig())
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

// clampRexLabel truncates s to maxRexLabel runes, replacing the tail with "…"
// so truncation is visible and never splits a UTF-8 sequence.
func clampRexLabel(s string) string {
	r := []rune(s)
	if len(r) <= maxRexLabel {
		return s
	}
	return string(r[:maxRexLabel-1]) + "…"
}

// PaneLabel is the Rex pane label: the agent.
func (n SessionName) PaneLabel() string { return n.Agent() }

// maxPaneLabelRunes is Rex's label limit for session.set_block_label.
const maxPaneLabelRunes = 128

// paneLabelFor returns the Rex pane label for a started session. An unaliased
// pool seat whose name the env proves (Sanitize(agent)+"-"+bead id equals
// GC_SESSION_NAME, gc's PoolSessionName shape) gets the short
// "<agent> (<bead id>)" form; every other rig-scoped session keeps
// "<agent> (<GC_SESSION_NAME>)", and a town-level session keeps the
// name-derived label.
func paneLabelFor(n SessionName, env map[string]string) string {
	if env["GC_RIG"] == "" && n.IsTown() {
		return n.PaneLabel()
	}
	session := strings.TrimSpace(env["GC_SESSION_NAME"])
	agent := paneLabelAgent(env)
	if session == "" || agent == "" {
		return n.PaneLabel()
	}
	// The safety guard: only claim the short form when the name verifies it.
	// gc has changed the name shape before, so a failed check degrades to the
	// long form rather than to a guess.
	if env["GC_ALIAS"] == "" {
		if suffix := panePoolSuffix(env); suffix != "" && sanitizeQualified(agent)+"-"+suffix == session {
			if short := agent + " (" + suffix + ")"; utf8.RuneCountInString(short) <= maxPaneLabelRunes {
				return short
			}
		}
	}
	label := agent + " (" + session + ")"
	if utf8.RuneCountInString(label) <= maxPaneLabelRunes {
		return label
	}
	// Truncate only the agent: the parenthesised session name is the
	// discovery identity (Binder.scan), so cutting it would make two long
	// sessions differing at the tail share a label. When the suffix alone
	// cannot fit, fall back to the bare session name; identity past 128
	// runes cannot be represented at all.
	suffix := " (" + session + ")"
	if len([]rune(suffix)) >= maxPaneLabelRunes {
		return clampRexLabel(session)
	}
	return string([]rune(agent)[:maxPaneLabelRunes-len([]rune(suffix))]) + suffix
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
// rewrites BEADS_ACTOR to GC_TEMPLATE.
func panePoolSuffix(env map[string]string) string {
	if v := strings.TrimSpace(env["GC_AGENT"]); v != "" && !strings.ContainsAny(v, "/.") {
		return v
	}
	if v := strings.TrimSpace(env["GC_SESSION_ID"]); v != "" {
		return v
	}
	return paneAgent(map[string]string{"BEADS_ACTOR": env["BEADS_ACTOR"]})
}

// identityMatch reports whether label names session n: its parenthesised suffix
// is n (legacy and named labels), or n is exactly Sanitize(agent)+"-"+suffix
// (the pool short form). A bare label or a prefix-only match never counts.
func identityMatch(name SessionName, label string) bool {
	i := strings.LastIndex(label, " (")
	if i < 0 || !strings.HasSuffix(label, ")") {
		return false
	}
	agent, suffix := label[:i], label[i+2:len(label)-1]
	if agent == "" || suffix == "" {
		return false
	}
	return suffix == string(name) || sanitizeQualified(agent)+"-"+suffix == string(name)
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

// String returns the original gc session name.
func (n SessionName) String() string { return string(n) }
