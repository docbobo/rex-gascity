package provider

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// StateDir returns the per-city state directory that holds the sidecar
// stores. GC_REX_STATE_DIR overrides the base; otherwise it is
// $XDG_STATE_HOME/rex-gascity, or ~/.local/state/rex-gascity.
func StateDir(city string) (string, error) {
	if env := os.Getenv("GC_REX_STATE_DIR"); env != "" {
		return filepath.Join(env, safeName(city)), nil
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
	return filepath.Join(base, "rex-gascity", safeName(city)), nil
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
