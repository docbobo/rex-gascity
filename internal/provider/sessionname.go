package provider

import (
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

// clampRexLabel truncates s to maxRexLabel runes, replacing the tail with "…"
// so truncation is visible and never splits a UTF-8 sequence.
func clampRexLabel(s string) string {
	// A string of at most maxRexLabel bytes has at most that many runes.
	if len(s) <= maxRexLabel {
		return s
	}
	// Walk the runes once, remembering where the kept prefix ends, instead of
	// materialising a []rune of the whole label.
	count, end := 0, 0
	for i := range s {
		if count == maxRexLabel-1 {
			end = i
		}
		count++
	}
	if count <= maxRexLabel {
		return s
	}
	// Rebuild the prefix rune by rune so invalid bytes become U+FFFD exactly as
	// the []rune round trip did.
	var sb strings.Builder
	sb.Grow(end + len("…"))
	for i, r := range s {
		if i >= end {
			break
		}
		sb.WriteRune(r)
	}
	sb.WriteString("…")
	return sb.String()
}

// PaneLabel is the Rex pane label: the agent.
func (n SessionName) PaneLabel() string { return n.Agent() }

// maxPaneLabelRunes is Rex's label limit for session.set_block_label.
const maxPaneLabelRunes = 128

// paneLabelShortened ends an agent that fitPaneLabel had to cut. The label has
// to carry the cut: without a marker a shortened agent reads like a short one,
// so identityMatch cannot tell "this agent was cut" from "this is the whole
// agent", and would rebuild a name that belongs to another session. Agents are
// rig and agent names; none of them end in this rune.
const paneLabelShortened = "…"

// paneLabelShortForm marks the pool short form's suffix: the "#" says the
// parenthesised text is a bead id, not a session name. A named session's name
// can equal a bead id, so without the mark a short-form label would name it.
const paneLabelShortForm = "#"

// fitPaneLabel renders "<agent> (<suffix>)" at or under Rex's limit. Only the
// agent is shortened: the parenthesised suffix is the discovery identity
// (Binder.findLive), so cutting it would make two sessions differing at the tail
// share a label. A short-form suffix carries paneLabelShortForm, so the mark
// costs one rune of the agent budget. A shortened agent is marked (paneLabelShortened) and names no
// session; ok is false when not even the marked agent and the suffix fit — no
// label within the limit can then name the session.
func fitPaneLabel(agent, suffix string) (string, bool) {
	tail := " (" + suffix + ")"
	budget := maxPaneLabelRunes - utf8.RuneCountInString(tail)
	if budget < 1 {
		return "", false
	}
	r := []rune(agent)
	if len(r) <= budget {
		return string(r) + tail, true
	}
	if budget < 2 {
		return "", false
	}
	return string(r[:budget-1]) + paneLabelShortened + tail, true
}

// identityMatch reports whether label names session n. A short form is
// "<agent> (#<bead id>)": its suffix carries paneLabelShortForm, so n is
// exactly Sanitize(agent)+"-"+bead id. A long form is "<agent> (<suffix>)":
// its suffix is n. Nothing else counts. A short form that fitPaneLabel had to
// shorten keeps the bead id but loses the agent's tail, so a prefix plus the
// suffix names no session: two names can share both while differing in
// between, and a label that fits two sessions is not evidence for either. A
// clamped label is a bare session name and names no session here.
func identityMatch(name SessionName, label string) bool {
	i := strings.LastIndex(label, " (")
	if i < 0 || !strings.HasSuffix(label, ")") {
		return false
	}
	agent, suffix := label[:i], label[i+2:len(label)-1]
	if agent == "" || suffix == "" {
		return false
	}
	if strings.HasPrefix(suffix, paneLabelShortForm) {
		bead := suffix[len(paneLabelShortForm):]
		if bead == "" || strings.HasSuffix(agent, paneLabelShortened) {
			return false
		}
		return sanitizeQualified(agent)+"-"+bead == string(name)
	}
	return suffix == string(name)
}

// String returns the original gc session name.
func (n SessionName) String() string { return string(n) }
