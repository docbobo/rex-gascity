package launch

import (
	"slices"
	"testing"
)

func TestCommandArgv(t *testing.T) {
	sh := func(c string) []string { return []string{"/bin/sh", "-lc", c} }
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"sleep 300", []string{"sleep", "300"}},
		{`claude  --flag='a b' "c \"d\"" e\ f`, []string{"claude", "--flag=a b", `c "d"`, "e f"}},
		{`echo ''`, []string{"echo", ""}},
		{`echo "unterminated`, sh(`echo "unterminated`)},
		{`echo 'unterminated`, sh(`echo 'unterminated`)},
		{`echo trailing\`, sh(`echo trailing\`)},
		{"a && b", sh("a && b")},
		{"echo $HOME", sh("echo $HOME")},
		{`echo "$HOME"`, sh(`echo "$HOME"`)},
		{`echo '$HOME'`, []string{"echo", "$HOME"}},
		{"FOO=bar claude", sh("FOO=bar claude")},
		{"ls *.go", sh("ls *.go")},
	} {
		if got := CommandArgv(c.in); !slices.Equal(got, c.want) {
			t.Errorf("CommandArgv(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
