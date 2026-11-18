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

// CityFromEnv returns the primary city name for this process. The RPP
// invocation carries no city, so the environment decides: GC_REX_CITY if set,
// else the basename of GC_CITY_PATH, else GC_CITY, else "default". Set
// GC_REX_CITY to isolate cities that share one Rex server.
func CityFromEnv() string {
	return CityCandidates()[0]
}

// CityCandidates returns the ordered, deduplicated city namespaces this
// process should consider, most specific first. GC_REX_CITY is an explicit
// pin: when set it is the only candidate, so two cities pinned to different
// values never cross-resolve. Otherwise the candidates are the GC_CITY_PATH
// basename, GC_CITY, and finally "default".
//
// RPP v0 carries no city, and the exec runtime provider passes the caller's
// env through unchanged, so different gc callers on one city see different
// candidates: the launchd controller sees none and resolves "default", while
// session and nudge-poller processes carry GC_CITY_PATH and resolve the city
// basename. Searching the whole candidate set keeps those callers agreeing on
// one store.
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
	if v := os.Getenv("GC_CITY_PATH"); v != "" {
		add(filepath.Base(v))
	}
	add(os.Getenv("GC_CITY"))
	add("default")
	return out
}
