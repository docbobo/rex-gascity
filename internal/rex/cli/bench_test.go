package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// The CLI client pays one `rex` process per call, so these benchmarks go
// through a stub binary that only cats a fixture. The stub's spawn cost is the
// floor (BenchmarkCallFloor); what a reply adds on top of it is the JSON work.

func stubRex(b *testing.B, reply any) *Client {
	b.Helper()
	dir := b.TempDir()
	fixture := filepath.Join(dir, "reply.json")
	body := []byte("{}")
	if reply != nil {
		var err error
		if body, err = json.Marshal(reply); err != nil {
			b.Fatal(err)
		}
	}
	if err := os.WriteFile(fixture, body, 0o644); err != nil {
		b.Fatal(err)
	}
	bin := filepath.Join(dir, "rex")
	script := "#!/bin/sh\ncat '" + fixture + "'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		b.Fatal(err)
	}
	return &Client{Bin: bin}
}

// layoutFor builds a balanced split tree over the given block ids.
func layoutFor(ids []string, rect [4]float64) map[string]any {
	if len(ids) == 1 {
		return map[string]any{"rect": rect, "block_id": ids[0]}
	}
	mid := len(ids) / 2
	left := [4]float64{rect[0], rect[1], rect[2] / 2, rect[3]}
	right := [4]float64{rect[0] + rect[2]/2, rect[1], rect[2] / 2, rect[3]}
	return map[string]any{
		"rect": rect,
		"split": map[string]any{
			"before": layoutFor(ids[:mid], left),
			"after":  layoutFor(ids[mid:], right),
		},
	}
}

func viewReply(windows, panesPer int) map[string]any {
	var wins []map[string]any
	for w := range windows {
		ids := make([]string, panesPer)
		blocks := make([]map[string]any, panesPer)
		for p := range panesPer {
			ids[p] = fmt.Sprintf("block:%d-%d", w, p)
			blocks[p] = map[string]any{
				"block_id": ids[p],
				"label":    fmt.Sprintf("agent%d (rig--agent%d)", p, p),
				"owner":    "gc",
			}
		}
		wins = append(wins, map[string]any{
			"window_id":        fmt.Sprintf("window:%d", w),
			"label":            fmt.Sprintf("Rig: rig%d", w),
			"focused_block_id": ids[0],
			"layers": []map[string]any{{
				"layout": layoutFor(ids, [4]float64{0, 0, 1, 1}),
				"blocks": blocks,
			}},
		})
	}
	return map[string]any{
		"session_id": "session:abc", "label": "gc:city",
		"active_window_id": "window:0", "revision": 7, "windows": wins,
	}
}

// BenchmarkCallFloor is a call with no reply to decode: process spawn plus
// marshalling a small argument map. Everything else is measured against it.
func BenchmarkCallFloor(b *testing.B) {
	c := stubRex(b, nil)
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if err := c.DestroySession(ctx, "session:abc"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSessionView(b *testing.B) {
	for _, shape := range []struct{ windows, panes int }{{1, 5}, {4, 25}, {10, 50}} {
		name := fmt.Sprintf("%dw-%dp", shape.windows, shape.panes)
		b.Run(name, func(b *testing.B) {
			c := stubRex(b, viewReply(shape.windows, shape.panes))
			ctx := context.Background()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				v, err := c.SessionView(ctx, "session:abc")
				if err != nil {
					b.Fatal(err)
				}
				if len(v.Windows) != shape.windows {
					b.Fatalf("windows = %d, want %d", len(v.Windows), shape.windows)
				}
			}
		})
	}
}

func BenchmarkListSessions(b *testing.B) {
	for _, n := range []int{1, 20, 200} {
		sessions := make([]map[string]any, n)
		for i := range n {
			sessions[i] = map[string]any{"session_id": fmt.Sprintf("session:%d", i), "label": fmt.Sprintf("gc:city%d", i)}
		}
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			c := stubRex(b, map[string]any{"sessions": sessions})
			ctx := context.Background()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				got, err := c.ListSessions(ctx)
				if err != nil || len(got) != n {
					b.Fatalf("ListSessions = %d, %v; want %d", len(got), err, n)
				}
			}
		})
	}
}

// BenchmarkLayoutBlockRects isolates the pure part of SessionView: walking the
// decoded layout tree.
func BenchmarkLayoutBlockRects(b *testing.B) {
	for _, panes := range []int{5, 50, 200} {
		ids := make([]string, panes)
		for i := range ids {
			ids[i] = fmt.Sprintf("block:%d", i)
		}
		raw, err := json.Marshal(layoutFor(ids, [4]float64{0, 0, 1, 1}))
		if err != nil {
			b.Fatal(err)
		}
		var tree layoutTree
		if err := json.Unmarshal(raw, &tree); err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("%dp", panes), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if got := tree.blockRects(); len(got) != panes {
					b.Fatalf("rects = %d, want %d", len(got), panes)
				}
			}
		})
	}
}

// BenchmarkBlockFormat decodes a format reply of increasing size: the reply
// content is one JSON string, so escapes and length drive the cost.
func BenchmarkBlockFormat(b *testing.B) {
	line := "\x1b[32mok\x1b[0m  github.com/example/pkg  0.123s \"quoted\" tab\there\n"
	for _, kb := range []int{1, 64, 900} {
		content := ""
		for len(content) < kb*1024 {
			content += line
		}
		b.Run(fmt.Sprintf("%dKiB", kb), func(b *testing.B) {
			c := stubRex(b, map[string]any{"content": content, "id": 0, "total": len(content), "eof": true})
			ctx := context.Background()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := c.BlockFormat(ctx, rex.SessionID("session:abc"), rex.BlockID("block:1")); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkSessionViewDecode is SessionView without the process spawn: decode
// the reply and convert it, the part the client's own code controls.
func BenchmarkSessionViewDecode(b *testing.B) {
	for _, shape := range []struct{ windows, panes int }{{1, 5}, {4, 25}, {10, 50}} {
		raw, err := json.Marshal(viewReply(shape.windows, shape.panes))
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("%dw-%dp", shape.windows, shape.panes), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var reply sessionViewReply
				if err := json.Unmarshal(raw, &reply); err != nil {
					b.Fatal(err)
				}
				if v := reply.view(); len(v.Windows) != shape.windows {
					b.Fatalf("windows = %d, want %d", len(v.Windows), shape.windows)
				}
			}
		})
	}
}
