package provider

import "strings"

// This file is the one place that states the label grammar: a bracketed kind
// tag, then the name, and for a pane a parenthesised identity plus a
// display-only "#<session id>" tail. A session or tab tag is a literal prefix
// (nothing reads it back), so it is written and compared as a string. A pane
// label is read by identityMatch, so the pane grammar has one writer
// (newPaneParts) and one reader (splitPaneLabel) here.
const (
	labelCity = "city"
	labelRig  = "rig"

	labelTagOpen    = "["
	labelTagClose   = "] "
	labelTailSep    = " #"
	labelSuffixOpen = " ("
	labelSuffixEnd  = ")"

	// legacyRigTabPrefix is the rig tab prefix providers before the bracketed
	// tags wrote: "Rig: <rig>". Nothing new is written with it; it is the
	// compatibility rung a window started by an older build still carries.
	legacyRigTabPrefix = "Rig: "

	// paneSuffixEscape marks a suffix that begins with the pool marker but is a
	// complete session name, not a pool identity: "(\#seat)" names the session
	// "#seat", while "(#seat)" names the pool seat Sanitize(agent)+"-seat".
	// The writer puts one escape in front of a name that starts with the pool
	// marker or with the escape itself, and the reader strips exactly one, so
	// every suffix decodes to one session.
	paneSuffixEscape = `\`
)

// taggedLabel renders a session or tab label: "[city] Shady_Sands".
func taggedLabel(kind, name string) string { return labelTagOpen + kind + labelTagClose + name }

// paneTag renders a pane label's agent tag: "[gc.implementation-worker]".
func paneTag(agent string) string { return labelTagOpen + agent + "]" }

// tagReadable reports whether name can follow a tag without moving where the
// tag ends. The tag ends at the first labelTagClose, so a name containing it
// cannot be tagged and is rendered in the legacy untagged shape instead.
func tagReadable(name string) bool { return !strings.Contains(name, labelTagClose) }

// paneParts is a parsed pane label:
//
//	pane   := prefix labelSuffixOpen suffix labelSuffixEnd [ labelTailSep id ]
//	prefix := paneTag(agent) | agent
//
// The untagged prefix is the legacy label's bare agent. suffix is the
// parenthesised identity verbatim, so a legacy "#<bead id>" suffix keeps its
// mark. id is the display-only tail and never an identity.
type paneParts struct {
	agent  string
	suffix string
	id     string
	tagged bool
}

// newPaneParts is the only writer of paneParts. It derives the form from the
// agent, so a tagged pane whose tag the reader would misplace cannot be
// constructed: an unreadable or blank agent falls back to the legacy shape.
func newPaneParts(agent, suffix, id string) paneParts {
	return paneParts{
		agent:  agent,
		suffix: suffix,
		id:     id,
		tagged: agent != "" && tagReadable(agent),
	}
}

// String renders the label the parts describe.
func (p paneParts) String() string {
	var sb strings.Builder
	if p.tagged {
		sb.WriteString(paneTag(p.agent))
	} else {
		sb.WriteString(p.agent)
	}
	sb.WriteString(labelSuffixOpen)
	sb.WriteString(p.suffix)
	sb.WriteString(labelSuffixEnd)
	if p.id != "" {
		sb.WriteString(labelTailSep)
		sb.WriteString(p.id)
	}
	return sb.String()
}

// splitPaneLabel parses a pane label. ok is false when the label was not
// written by this provider: no tail is read unless it follows a parenthesised
// non-empty suffix, neither suffix nor agent may be empty, and a tag must end
// at the first labelTagClose with nothing after it. A label that does not
// parse names no session; the caller never guesses.
func splitPaneLabel(label string) (paneParts, bool) {
	if label == "" {
		return paneParts{}, false
	}
	rest, id := label, ""
	if !strings.HasSuffix(rest, labelSuffixEnd) {
		i := strings.LastIndex(rest, labelTailSep)
		if i < 0 {
			return paneParts{}, false
		}
		id = rest[i+len(labelTailSep):]
		if id == "" || strings.Contains(id, " ") {
			return paneParts{}, false
		}
		rest = rest[:i]
		if !strings.HasSuffix(rest, labelSuffixEnd) {
			return paneParts{}, false
		}
	}
	i := strings.LastIndex(rest, labelSuffixOpen)
	if i < 0 {
		return paneParts{}, false
	}
	prefix := rest[:i]
	suffix := rest[i+len(labelSuffixOpen) : len(rest)-len(labelSuffixEnd)]
	if prefix == "" || suffix == "" {
		return paneParts{}, false
	}
	var p paneParts
	p.suffix, p.id = suffix, id
	if strings.HasPrefix(prefix, labelTagOpen) {
		// The tag's separator is the " (" the label uses, so the leftover
		// prefix is "[agent]" and the tag ends at the agent's final "]".
		if !strings.HasSuffix(prefix, "]") || len(prefix) < len(labelTagOpen)+1+1 {
			return paneParts{}, false
		}
		p.agent, p.tagged = prefix[len(labelTagOpen):len(prefix)-1], true
		// A first-close separator inside the agent means the tag was not
		// written by this provider: the reader would end it earlier.
		if p.agent == "" || strings.Contains(p.agent, labelTagClose) {
			return paneParts{}, false
		}
	} else {
		p.agent = prefix
	}
	return p, true
}

// paneSuffix returns name as a pane label suffix: the name verbatim, escaped
// when it begins with the pool marker or with the escape so the reader cannot
// mistake it for a pool identity.
func paneSuffix(name string) string {
	if name == "" {
		return ""
	}
	if name[0] == paneLabelShortForm[0] || name[0] == paneSuffixEscape[0] {
		return paneSuffixEscape + name
	}
	return name
}

// unescapePaneSuffix returns the session name a suffix holds and whether the
// writer escaped it. An escaped suffix is always a complete name, never a pool
// identity.
func unescapePaneSuffix(suffix string) (string, bool) {
	if rest, ok := strings.CutPrefix(suffix, paneSuffixEscape); ok {
		return rest, true
	}
	return suffix, false
}

// namesSession reports whether the label names session name. The suffix is the
// identity, never the "#<id>" tail: a marked suffix (the short pool form) means
// name is Sanitize(agent)+"-"+marker-tail; an unmarked suffix is a complete
// session name (the long and town forms). A shortened agent keeps the identity,
// because the parenthesis survives the cut.
//
// Only the unescaped marked form is read as a pool identity. Reading an
// unmarked tagged suffix as Sanitize(agent)+"-"+suffix would let one label name
// two sessions: the pool seat and a session literally named the suffix (the
// town form "[worker] (seat)" for session "seat" is byte-for-byte the unmarked
// pool form for "worker-seat"). The marker, and the escape that takes a name
// out from under it, is what keeps each label to one session.
func (p paneParts) namesSession(name SessionName) bool {
	suffix, escaped := unescapePaneSuffix(p.suffix)
	if !escaped && strings.HasPrefix(suffix, paneLabelShortForm) {
		// Short pool form: the parentheses hold the marked bead id.
		bead := suffix[len(paneLabelShortForm):]
		if bead == "" || p.agent == "" || strings.HasSuffix(p.agent, paneLabelShortened) {
			return false
		}
		return sanitizeQualified(p.agent)+"-"+bead == string(name)
	}
	return suffix == string(name)
}
