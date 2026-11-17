package provider

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// StateDir returns the per-city state directory that holds the sidecar
// stores. GC_REX_STATE_DIR overrides the base; otherwise it is
// $XDG_STATE_HOME/rex-gas-city, or ~/.local/state/rex-gas-city.
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
	return filepath.Join(base, "rex-gas-city", safeName(city)), nil
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

// CityFromEnv returns the city name for this process. The RPP invocation
// carries no city, so the environment decides: GC_REX_CITY if set, else the
// basename of GC_CITY_PATH, else GC_CITY, else "default". Set GC_REX_CITY to
// isolate cities that share one Rex server.
func CityFromEnv() string {
	if v := os.Getenv("GC_REX_CITY"); v != "" {
		return v
	}
	if v := os.Getenv("GC_CITY_PATH"); v != "" {
		return filepath.Base(v)
	}
	if v := os.Getenv("GC_CITY"); v != "" {
		return v
	}
	return "default"
}
