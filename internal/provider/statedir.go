package provider

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// stateBase returns the directory whose entries are the per-city state dirs.
// GC_EXEC_STATE_DIR is gc's per-op state root and takes precedence; it must be
// absolute, so a relative value is an error rather than a root resolved against
// the caller's cwd. GC_REX_STATE_DIR is the provider's own override and
// fallback. Without either, the base is $XDG_STATE_HOME/rex-gascity, or
// ~/.local/state/rex-gascity.
func stateBase() (string, error) {
	if env := os.Getenv("GC_EXEC_STATE_DIR"); env != "" {
		if !filepath.IsAbs(env) {
			return "", fmt.Errorf("provider: GC_EXEC_STATE_DIR %q is not absolute", env)
		}
		return env, nil
	}
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

// cityNameFromPath names a city by the last component of the path gc serves it
// from. gc's contract uses absolute paths, which filepath.Clean keeps under a
// named directory; an empty base (the naked root) or one that is only dots
// names nothing, so it yields "" and the caller falls through to the next
// source rather than pinning a namespace like "." or "/".
func cityNameFromPath(path string) string {
	name := filepath.Base(filepath.Clean(path))
	switch name {
	case "", ".", "..", string(filepath.Separator):
		return ""
	}
	return name
}

// cityPin returns the explicit city a caller is bound to, or "" when it is
// unpinned. gc's per-op city contract takes precedence: GC_EXEC_CITY_NAME is
// the city's name, and a caller that carries only GC_EXEC_CITY_PATH is named by
// that path's last component. GC_REX_CITY is the provider's own pin for callers
// that run the script without gc, kept as the fallback.
//
// The pin is a city NAME, never a path. Using the raw GC_EXEC_CITY_PATH as the
// pin made the Rex session label gc:<absolute path> and split a city's sidecar
// state into a second, path-encoded namespace the moment gc started setting the
// variable (GCREX-40's review flagged the label; gc edge made it real).
func cityPin() string {
	if v := os.Getenv("GC_EXEC_CITY_NAME"); v != "" {
		return v
	}
	if v := cityNameFromPath(os.Getenv("GC_EXEC_CITY_PATH")); v != "" {
		return v
	}
	return os.Getenv("GC_REX_CITY")
}

// CityFromEnv returns the canonical city namespace for this process. A pinned
// caller (GC_EXEC_CITY_NAME, else basename(GC_EXEC_CITY_PATH), else GC_REX_CITY)
// keeps exactly its pin; without one the namespace is the fixed "default".
//
// RPP v0 carries no city, and the exec runtime provider passes the caller's
// env through unchanged, so different gc callers on one city see a different
// environment: the launchd controller carries no GC_CITY*, while session and
// nudge-poller processes carry GC_CITY_PATH. Resolving the store from
// GC_CITY_PATH/GC_CITY therefore made the provider disagree with itself
// depending on the caller (GCREX-17): start wrote one namespace and the
// nudge poller read another. GC_CITY_PATH/GC_CITY are incidental (the start
// config carries them but the runtime contract does not), so they do not
// select the canonical store. gc's GC_EXEC_CITY_NAME/GC_EXEC_CITY_PATH are its
// own city key and do select it; set GC_REX_CITY to isolate cities that share
// one Rex server when running the script outside gc.
func CityFromEnv() string {
	if v := cityPin(); v != "" {
		return v
	}
	return "default"
}

// SiblingCityNamespaces returns the city namespaces that have a state dir
// beside the caller's, excluding the names in exclude. It lets an unpinned
// lookup (no city pin) discover the pin a sibling controller wrote under:
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
// read across, canonical first. A pin (GC_EXEC_CITY_NAME, then
// basename(GC_EXEC_CITY_PATH), then GC_REX_CITY) is the only candidate, so two
// cities pinned to different values never cross-resolve. Otherwise the
// canonical "default" comes first, followed by the incidental
// basename(GC_CITY_PATH) and GC_CITY, where a record written by an earlier
// version (or an older caller) may still live.
func CityCandidates() []string {
	if v := cityPin(); v != "" {
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

// SessionLabels returns the accepted Rex session labels for cities: every
// canonical "[city] <city>" label first, then every legacy "gc:<city>" label,
// deduplicated in order. The first label is canonical for a new session; the
// legacy labels keep a session written by an older provider reachable, so an
// upgrade never needs a city restart.
func SessionLabels(cities []string) []string {
	labels := make([]string, 0, 2*len(cities))
	for _, city := range cities {
		labels = append(labels, rexSessionLabel(city))
	}
	for _, city := range cities {
		labels = append(labels, legacySessionLabel(city))
	}
	return normalizeSessionLabels(labels)
}
