package provider

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// drainDiagTimeout bounds each git call made for the drain diagnostic, so a
// slow or wedged work tree cannot delay Stop.
const drainDiagTimeout = 3 * time.Second

// drainDiagnostic is the JSON written when a worker is stopped while its work
// tree holds uncommitted edits. It names the worker, its session and the file
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
// stopped binding's work dir is a git work tree with uncommitted changes.
// gc's drain/retire path calls provider stop; a worker drained while it still
// held edits is otherwise silent. It is best effort: every error is swallowed
// and the git calls are time bounded, so it never fails or delays Stop.
func (p *Provider) writeDrainDiagnostic(ctx context.Context, b Binding) {
	if b.WorkDir == "" || p.stateDir == "" {
		return
	}
	root, ok := gitOutput(ctx, b.WorkDir, "rev-parse", "--show-toplevel")
	if !ok {
		return
	}
	root = strings.TrimSpace(root)
	status, ok := gitOutput(ctx, b.WorkDir, "status", "--porcelain", "-z", "--untracked-files=all")
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
	if sid, _, err := p.meta.Get(b.Name, "GC_SESSION_ID"); err == nil {
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
	_ = writeJSON(filepath.Join(p.stateDir, "diagnostics", name+".json"), d)
}

// gitOutput runs git in dir with a bounded timeout, returning its stdout.
func gitOutput(ctx context.Context, dir string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, drainDiagTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return string(out), true
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
