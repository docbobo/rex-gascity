package fakerex_test

import (
	"strings"
	"testing"
)

// parseProcState returns the single-letter state field of a /proc/<pid>/stat
// line, or 0 when the line is malformed. The field follows the parenthesized
// comm, which may itself contain spaces and parentheses, so find the last ')'.
func parseProcState(b []byte) byte {
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 || i+2 >= len(b) || b[i+1] != ' ' {
		return 0
	}
	return b[i+2]
}

func TestParseProcState(t *testing.T) {
	cases := []struct {
		line string
		want byte
	}{
		{"1234 (sh) Z 1 1234 1234 0 -1", 'Z'},
		{"42 (tmux: server) S 1 42 42 0 -1", 'S'},
		{"42 (weird )name) R 1 42", 'R'},
		{"garbage", 0},
		{"42 (sh)X 1", 0},
	}
	for _, c := range cases {
		if got := parseProcState([]byte(c.line)); got != c.want {
			t.Errorf("parseProcState(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}
