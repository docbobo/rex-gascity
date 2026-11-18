package provider

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// drainDiagTimeout bounds the git calls made for the drain diagnostic, taken
// together, so a slow or wedged work tree cannot delay Stop or hold the state
// lock for long. It is a var only so tests can shorten it.
var drainDiagTimeout = 3 * time.Second

// drainDiagnostic is the JSON written when a worker is stopped while its work
// tree holds modified tracked files. It names the worker, its session and the file
// edited last, so an operator can tell which seat lost work.
type drainDiagnostic struct {
	Worker         string    `json:"worker"`
	Aliases        []string  `json:"aliases,omitempty"`
	SessionID      string    `json:"session_id,omitempty"`
	WorkDir        string    `json:"work_dir"`
	ChangedFiles   []string  `json:"changed_files"`
	LastEditedFile string    `json:"last_edited_file,omitempty"`
	LastEditedAt   time.Time `json:"last_edited_at,omitzero"`
	StoppedAt      time.Time `json:"stopped_at"`
}

var unsafeNameChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// writeDrainDiagnostic leaves <stateDir>/diagnostics/<name>.json when the
// stopped binding's work dir is a git work tree with modified tracked files.
// Untracked files are ignored: a dir such as .pi/ is always present and would
// be a permanent false positive. gc's drain/retire path calls provider stop; a
// worker drained while it still held edits is otherwise silent. It is best
// effort: every error is swallowed and both git calls share one time budget, so
// it never fails or delays Stop.
func (p *Provider) writeDrainDiagnostic(ctx context.Context, b Binding) {
	if b.WorkDir == "" || p.ns.CanonicalDir() == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, drainDiagTimeout)
	defer cancel()
	root, ok := p.gitOutput(ctx, b.WorkDir, "rev-parse", "--show-toplevel")
	if !ok {
		return
	}
	root = strings.TrimRight(root, "\r\n")
	status, ok := p.gitOutput(ctx, b.WorkDir, "status", "--porcelain", "-z", "--untracked-files=no")
	if !ok {
		return
	}
	changed := parsePorcelainZ(status)
	if len(changed) == 0 {
		return
	}

	d := drainDiagnostic{
		Worker:       string(b.Name),
		WorkDir:      b.WorkDir,
		ChangedFiles: changed,
		StoppedAt:    time.Now().UTC(),
	}
	for _, a := range b.Aliases {
		d.Aliases = append(d.Aliases, string(a))
	}
	if sid, _, err := p.metaGet(b.Name, "GC_SESSION_ID"); err == nil {
		d.SessionID = sid
	}
	for _, f := range changed {
		info, err := os.Stat(filepath.Join(root, f))
		if err != nil {
			continue // deleted files have no mtime
		}
		if d.LastEditedFile == "" || info.ModTime().After(d.LastEditedAt) {
			d.LastEditedFile, d.LastEditedAt = f, info.ModTime().UTC()
		}
	}
	name := unsafeNameChars.ReplaceAllString(string(b.Name), "_")
	_ = writeJSON(filepath.Join(p.ns.CanonicalDir(), "diagnostics", name+".json"), d)
}

// gitOutput runs git in dir under ctx, returning its stdout. It sets
// GIT_OPTIONAL_LOCKS=0 (like --no-optional-locks) so `git status` never takes
// .git/index.lock and cannot race a worker's `git add` or `git commit` in a
// shared work tree.
func (p *Provider) gitOutput(ctx context.Context, dir string, args ...string) (string, bool) {
	out, err := p.runner.Run(ctx, dir, []string{"GIT_OPTIONAL_LOCKS=0"}, "git", args...)
	if err != nil {
		return "", false
	}
	return out, true
}

// parsePorcelainZ returns the paths in `git status --porcelain -z` output.
// Rename and copy entries carry the old path as a following entry, which is
// skipped.
func parsePorcelainZ(s string) []string {
	var out []string
	entries := strings.Split(s, "\x00")
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if len(e) < 4 {
			continue
		}
		out = append(out, e[3:])
		if e[0] == 'R' || e[0] == 'C' || e[1] == 'R' || e[1] == 'C' {
			i++
		}
	}
	return out
}
