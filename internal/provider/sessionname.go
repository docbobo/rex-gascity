package provider

import "strings"

// townRig is the tab label for town-level agents, which have no rig.
const townRig = "__town"

// nameSep separates the rig from the agent in a gc session name.
const nameSep = "--"

// SessionName is a gc session name. Rig agents are "<rig>--<agent>"; a name
// without "--" is town-level. It is comparable and usable as a map key.
type SessionName string

// Rig returns the rig part, or "__town" for town-level names.
func (n SessionName) Rig() string {
	if i := strings.Index(string(n), nameSep); i >= 0 {
		return string(n)[:i]
	}
	return townRig
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

// TabLabel is the Rex tab label: the rig.
func (n SessionName) TabLabel() string { return n.Rig() }

// PaneLabel is the Rex pane label: the agent.
func (n SessionName) PaneLabel() string { return n.Agent() }

// String returns the original gc session name.
func (n SessionName) String() string { return string(n) }
