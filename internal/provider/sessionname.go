package provider

import (
	"strings"
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

// rexSessionLabel is the canonical Rex session label for city (one session
// per city): "[city] <city>". The tag names the kind; the name is the city
// identity unchanged. The city part is never truncated: clamping "[city] "
// would make two different long city names share one canonical label, and
// SessionLabels would then fold them into one Rex session. A city too long for
// Rex's label limit fails loudly at session creation instead of silently
// reusing another city's session.
func rexSessionLabel(city string) string { return taggedLabel(labelCity, city) }

// legacySessionLabel is the session label providers before the bracketed-tag
// grammar wrote: "gc:<city>". It stays an accepted label forever, so a session
// created by an older provider is still found without recreation.
func legacySessionLabel(city string) string { return "gc:" + city }

// sameCityLabel reports whether two Rex session labels name the same city: the
// canonical "[city] <city>" and the legacy "gc:<city>" are one city's session,
// so a record written under one still settles a view that carries the other.
func sameCityLabel(a, b string) bool {
	if a == b {
		return true
	}
	ca, oka := cityOfSessionLabel(a)
	cb, okb := cityOfSessionLabel(b)
	return oka && okb && ca == cb
}

// cityOfSessionLabel returns the city a session label names under either
// grammar.
func cityOfSessionLabel(label string) (string, bool) {
	if rest, ok := strings.CutPrefix(label, labelTagOpen+labelCity+labelTagClose); ok {
		return rest, true
	}
	if rest, ok := strings.CutPrefix(label, "gc:"); ok {
		return rest, true
	}
	return "", false
}

// townTabName is the town tab's name: the pin itself, or townTabFallback when
// there is no pin. The label is the pin, never "default" merely because
// CityFromEnv fell back to it: town-ness is the pin being empty. The bare name
// is also the pre-change town tab label, so resolution keeps it as a legacy
// rung.
func townTabName(pin string) string {
	if pin == "" {
		return townTabFallback
	}
	return pin
}

// townTabLabel is the canonical town tab label: "[city] <pin>", or
// "[city] Gas City" when there is no pin. No tab is untagged.
func townTabLabel(pin string) string {
	return clampRexLabel(taggedLabel(labelCity, townTabName(pin)))
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

// fitPaneLabel is gone: the pool and long pane forms shorten differently, so
// fitPoolPaneLabel and fitLongPaneLabel each state their own ladder.

// identityMatch reports whether label names session n. The pane grammar has
// one parser (splitPaneLabel) and one identity rule (paneParts.namesSession):
// the suffix is the identity and the "#<id>" tail is display-only. Every pane
// shape the provider ever wrote is accepted, and a label that does not parse
// names nobody.
func identityMatch(name SessionName, label string) bool {
	parts, ok := splitPaneLabel(label)
	if !ok {
		return false
	}
	return parts.namesSession(name)
}

// String returns the original gc session name.
func (n SessionName) String() string { return string(n) }
