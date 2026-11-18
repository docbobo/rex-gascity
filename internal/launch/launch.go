// Package launch owns the translation from a gc start config to a Rex launch:
// splitting the command string into an argv, applying the environment at the
// command boundary, merging environments, and projecting pool routing. The
// Rex wire shape (block layout) stays in internal/rex/cli.
package launch

import (
	"sort"
	"strings"
)

// CommandArgv turns the start config's command string into an argv. A plain
// command is split with shell quoting rules; anything a plain split cannot
// represent faithfully (unbalanced quotes, operators, expansions, globs, env
// assignments) runs under /bin/sh -lc instead. An empty command yields nil:
// the pane starts its default shell.
func CommandArgv(command string) []string {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	argv, ok := Split(command)
	if !ok || len(argv) == 0 || strings.Contains(argv[0], "=") {
		return []string{"/bin/sh", "-lc", command}
	}
	return argv
}

// Split splits s into words using POSIX shell quoting: single quotes
// are literal, double quotes allow backslash escapes of $ ` " \ and newline,
// and an unquoted backslash escapes the next byte. It reports false when s
// does not parse (unterminated quote, trailing backslash) or needs a shell
// (an unquoted operator, glob or comment, or an expansion outside single
// quotes).
func Split(s string) ([]string, bool) {
	var (
		words  []string
		cur    strings.Builder
		inWord bool
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		case c == '\\':
			if i+1 >= len(s) {
				return nil, false
			}
			i++
			if s[i] != '\n' { // backslash-newline is a line continuation
				cur.WriteByte(s[i])
			}
			inWord = true
		case c == '\'':
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				return nil, false
			}
			cur.WriteString(s[i+1 : i+1+end])
			i += end + 1
			inWord = true
		case c == '"':
			i++
			for ; i < len(s) && s[i] != '"'; i++ {
				switch s[i] {
				case '$', '`':
					return nil, false
				case '\\':
					if i+1 < len(s) && strings.IndexByte("$`\"\\\n", s[i+1]) >= 0 {
						i++
						if s[i] != '\n' {
							cur.WriteByte(s[i])
						}
						continue
					}
				}
				cur.WriteByte(s[i])
			}
			if i >= len(s) {
				return nil, false
			}
			inWord = true
		case strings.IndexByte("|&;<>()$`*?[]{}~#!\n", c) >= 0:
			return nil, false
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, true
}

// Command returns argv with the environment applied at the command
// boundary, since Rex terminal launch options have no per-pane env. With no
// env, or no command, argv is returned unchanged: a bare shell cannot be
// seeded (a documented limitation), and an empty argv must stay empty so the
// pane starts its default shell. Keys are sorted for deterministic output,
// and the caller's slice is never mutated.
func Command(argv []string, env map[string]string) []string {
	if len(argv) == 0 || len(env) == 0 {
		return argv
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(env)+1+len(argv))
	out = append(out, "env")
	// `env` stops parsing options at the first KEY=V assignment, so every
	// -u option must precede every assignment.
	for _, k := range keys {
		if env[k] == "" {
			// Empty means unset, like RPP and gc's tmux adapter; a literal
			// empty variable would mask gc's own fallbacks.
			out = append(out, "-u", k)
		}
	}
	for _, k := range keys {
		if env[k] != "" {
			out = append(out, k+"="+env[k])
		}
	}
	return append(out, argv...)
}

// ProjectPoolRouting makes an unaliased pool or demand seat's work query reach
// its routed work without giving up the per-seat claim identity.
//
// gc 1.5.1 ships GC_ALIAS empty and GC_AGENT/BEADS_ACTOR as the session bead
// id for an unaliased pool seat, and expects `gc hook --claim` to find work
// routed to GC_TEMPLATE. It does not: hookSessionAgentForQuery
// (cmd/gc/cmd_hook.go:818 at v1.5.1) falls back to BEADS_ACTOR/GC_AGENT (the
// bead id), and hookWorkQueryStores (cmd/gc/hook_cross_store.go:107) then
// cannot resolve the rig from that agent query, so the query never reads the
// store the routed step lives in (gastownhall/gascity#5716).
//
// Routing the agent query back to the template fixes discovery. GC_ALIAS stays
// empty, so gc's claim writer hookClaimAssigneeIdentity
// (cmd/gc/cmd_hook.go:866) records the claim under GC_SESSION_ID, the concrete
// session bead id: a live session answers to it (sessionBeadAssigneeIdentities,
// cmd/gc/pool_session_name.go:29) and every pool slot has its own, so seats
// single-flight. Setting GC_ALIAS to the template instead (the GCREX-18
// deviation) made every slot claim under one shared string no session bead
// answers to, so a second seat adopted the first seat's in_progress bead and
// releaseOrphanedPoolAssignments (cmd/gc/pool_session_name.go:211) reopened it
// (GCREX-32).
//
// It applies only when GC_ALIAS is empty, GC_TEMPLATE is set and differs from
// GC_AGENT, and GC_SESSION_ID is set: only BEADS_ACTOR is set to GC_TEMPLATE.
// A non-empty GC_ALIAS, GC_AGENT, GC_SESSION_ID, the tokens and the caller's
// map are never changed. With no GC_SESSION_ID there is no durable claim
// identity to protect, so the env is passed through unchanged.
func ProjectPoolRouting(env map[string]string) map[string]string {
	if env["GC_ALIAS"] != "" {
		return env
	}
	tmpl := env["GC_TEMPLATE"]
	if tmpl == "" || tmpl == env["GC_AGENT"] {
		return env
	}
	if env["GC_SESSION_ID"] == "" || env["BEADS_ACTOR"] == tmpl {
		return env
	}
	out := make(map[string]string, len(env)+1)
	for k, v := range env {
		out[k] = v
	}
	out["BEADS_ACTOR"] = tmpl
	return out
}

// MergeEnv merges the base environment slice with an overlay map, returning a
// new sorted slice. Overlay entries replace base entries with the same key.
func MergeEnv(base []string, overlay map[string]string) []string {
	m := make(map[string]string, len(base)+len(overlay))
	for _, kv := range base {
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			m[kv[:idx]] = kv[idx+1:]
		}
	}
	for k, v := range overlay {
		m[k] = v
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}
