package provider

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// stateBase returns the directory whose entries are the per-city state dirs.
// GC_REX_STATE_DIR overrides the base; otherwise it is
// $XDG_STATE_HOME/rex-gascity, or ~/.local/state/rex-gascity.
func stateBase() (string, error) {
	if env := os.Getenv("GC_REX_STATE_DIR"); env != "" {
		return env, nil
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if home == "" {
			return "", errors.New("provider: cannot resolve state dir: no home directory")
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "rex-gascity"), nil
}

// StateDir returns the per-city state directory that holds the sidecar
// stores.
func StateDir(city string) (string, error) {
	base, err := stateBase()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, safeName(city)), nil
}

// safeName maps a city name to a single path element. It percent-encodes the
// name as a byte string: [a-z0-9-] stay as is and every other byte
// (including uppercase letters, '_', '.', path separators, '%' and UTF-8
// continuation bytes) becomes %XX with uppercase hex. The mapping is
// injective, so distinct cities never share a directory. The output holds
// only lowercase letters, digits, '-' and %XX escapes, so two outputs that
// differ only by letter case come from the same name: the mapping stays
// injective on case-insensitive filesystems too ("Acme" is "%41cme"). Dots
// and separators are always encoded, so the result is never "." or "..". An
// empty name becomes "%00".
func safeName(s string) string {
	if s == "" {
		return "%00"
	}
	const hexdigits = "0123456789ABCDEF"
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			sb.WriteByte(c)
		default:
			sb.WriteByte('%')
			sb.WriteByte(hexdigits[c>>4])
			sb.WriteByte(hexdigits[c&15])
		}
	}
	return sb.String()
}

// CityFromEnv returns the canonical city namespace for this process.
// GC_REX_CITY is an explicit per-city pin; without it the namespace is the
// fixed "default".
//
// RPP v0 carries no city, and the exec runtime provider passes the caller's
// env through unchanged, so different gc callers on one city see a different
// environment: the launchd controller carries no GC_CITY*, while session and
// nudge-poller processes carry GC_CITY_PATH. Resolving the store from
// GC_CITY_PATH/GC_CITY therefore made the provider disagree with itself
// depending on the caller (GCREX-17): start wrote one namespace and the
// nudge poller read another. GC_CITY_PATH/GC_CITY are incidental (the start
// config carries them but the runtime contract does not), so they do not
// select the canonical store. Set GC_REX_CITY to isolate cities that share
// one Rex server.
func CityFromEnv() string {
	if v := os.Getenv("GC_REX_CITY"); v != "" {
		return v
	}
	return "default"
}

// SiblingCityNamespaces returns the city namespaces that have a state dir
// beside the caller's, excluding the names in exclude. It lets an unpinned
// lookup (no GC_REX_CITY) discover the pin a sibling controller wrote under:
// the caller and the controller share one machine state base, and the city
// name is recoverable from the directory name because safeName is injective.
// Only a name that re-encodes to the directory name is trusted, so a stray or
// hand-made directory never becomes a lookup namespace. The result is sorted
// for a stable, documented precedence; an unreadable base yields no siblings.
func SiblingCityNamespaces(exclude []string) []string {
	base, err := stateBase()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	seen := make(map[string]bool, len(exclude))
	for _, city := range exclude {
		seen[city] = true
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		city, ok := decodeSafeName(name)
		if !ok || safeName(city) != name || seen[city] || hasControl(city) {
			continue
		}
		seen[city] = true
		out = append(out, city)
	}
	sort.Strings(out)
	return out
}

// decodeSafeName reverses safeName's percent encoding. It only accepts the
// bytes safeName emits (lowercase letters, digits, '-' and uppercase %XX
// escapes) so a directory name that safeName would never produce is rejected.
func decodeSafeName(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '%' {
			if i+2 >= len(s) {
				return "", false
			}
			hi, ok := hexVal(s[i+1])
			if !ok {
				return "", false
			}
			lo, ok := hexVal(s[i+2])
			if !ok {
				return "", false
			}
			out = append(out, hi<<4|lo)
			i += 2
			continue
		}
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			out = append(out, c)
		default:
			return "", false
		}
	}
	return string(out), true
}

// hasControl reports whether s contains a C0 control byte. safeName encodes
// the empty city as %00, which decodes to NUL; no real namespace has one, so
// discovery drops it.
func hasControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 {
			return true
		}
	}
	return false
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	default:
		return 0, false
	}
}

// CityCandidates returns the ordered, deduplicated namespaces a lookup should
// read across, canonical first. GC_REX_CITY is a pin: when set it is the only
// candidate, so two cities pinned to different values never cross-resolve.
// Otherwise the canonical "default" comes first, followed by the incidental
// basename(GC_CITY_PATH) and GC_CITY, where a record written by an earlier
// version (or an older caller) may still live.
func CityCandidates() []string {
	if v := os.Getenv("GC_REX_CITY"); v != "" {
		return []string{v}
	}
	var out []string
	seen := map[string]bool{}
	add := func(v string) {
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	add("default")
	if v := os.Getenv("GC_CITY_PATH"); v != "" {
		add(filepath.Base(v))
	}
	add(os.Getenv("GC_CITY"))
	return out
}

// StateDirs resolves each city namespace to its per-city state directory, in
// the order given (canonical first). It fails if any directory cannot be
// resolved, so a caller never silently drops a namespace.
func StateDirs(cities []string) ([]string, error) {
	dirs := make([]string, 0, len(cities))
	for _, city := range cities {
		dir, err := StateDir(city)
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, dir)
	}
	return dirs, nil
}

// SessionLabels returns the Rex session labels for cities, in order and
// deduplicated. The first is the canonical label used for a new session.
func SessionLabels(cities []string) []string {
	out := make([]string, 0, len(cities))
	seen := map[string]bool{}
	for _, city := range cities {
		label := rexSessionLabel(city)
		if seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	return out
}
