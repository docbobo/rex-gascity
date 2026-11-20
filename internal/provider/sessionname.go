package provider

import "strings"

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

// tabLabel derives the Rex tab label for a started session: the town label for
// a town-level name, otherwise "Rig: <rig>" with GC_RIG preferred over the
// name split. Every result is clamped to maxRexLabel.
func tabLabel(name SessionName, env map[string]string, town string) string {
	if name.IsTown() {
		return clampRexLabel(town)
	}
	rig := env["GC_RIG"]
	if rig == "" {
		rig = name.Rig()
	}
	return clampRexLabel("Rig: " + rig)
}

// legacyTabLabel is the tab label gc wrote before GCREX-72: "__town" for a
// town-level name, otherwise the bare rig. A record with no stored TabLabel or
// a window that predates the upgrade still carries it, so resolution accepts it
// as well as the derived label.
func legacyTabLabel(n SessionName) string { return legacyTabLabelFor(n, nil) }

// legacyTabLabelFor is the label the pre-GCREX-72 provider wrote for n: the
// town label for a town-level name, otherwise the start env's GC_RIG (which gc
// 1.5.1 only supplied once the rig was known) or the name-derived rig.
func legacyTabLabelFor(n SessionName, env map[string]string) string {
	if n.IsTown() {
		return legacyTownTab
	}
	rig := env["GC_RIG"]
	if rig == "" {
		rig = n.Rig()
	}
	return rig
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

// String returns the original gc session name.
func (n SessionName) String() string { return string(n) }
