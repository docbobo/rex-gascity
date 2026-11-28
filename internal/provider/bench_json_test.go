package provider

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// JSON-heavy provider paths: the claim probe's bd record decode and the
// per-binding state files. Both are tiny next to the process they sit beside;
// these benchmarks size them so that is a measured claim, not an assumption.

var benchClaimSink bool

func BenchmarkClaimRecordHeld(b *testing.B) {
	const sid = "gc-123"
	bigMeta := &strings.Builder{}
	for i := range 40 {
		fmt.Fprintf(bigMeta, `"gc.k%d":"value-%d",`, i, i)
	}
	cases := []struct{ name, out string }{
		{"empty", ``},
		{"empty-list", `[]`},
		{"closed", `[{"status":"closed","assignee":"a","metadata":{}}]`},
		{"held", `[{"status":"in_progress","assignee":"rig/worker","metadata":{"gc.session_id":"gc-123"}}]`},
		{"other-owner", `[{"status":"in_progress","assignee":"rig/worker","metadata":{"gc.session_id":"gc-999"}}]`},
		{"garbled", `not json at all`},
		// A real bd record carries many more fields than the three read here.
		{"large-record", `[{"id":"bd-42","title":"` + strings.Repeat("t", 200) + `","description":"` +
			strings.Repeat("d", 2000) + `","status":"in_progress","assignee":"rig/worker","priority":2,` +
			`"labels":["a","b","c"],"dependencies":[],"metadata":{` + bigMeta.String() + `"gc.session_id":"gc-123"}}]`},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				benchClaimSink = claimRecordHeld(tc.out, sid)
			}
		})
	}
}

func benchBinding(i int) Binding {
	name := SessionName(fmt.Sprintf("rig--agent%d", i))
	return Binding{
		Name:         name,
		SessionID:    rex.SessionID("session:abc"),
		SessionLabel: "gc:city",
		WindowID:     rex.WindowID(fmt.Sprintf("window:%d", i)),
		BlockID:      rex.BlockID(fmt.Sprintf("block:%d", i)),
		TabLabel:     "Rig: rig",
		PaneLabel:    fmt.Sprintf("agent%d (rig--agent%d)", i, i),
		Aliases:      []SessionName{SessionName(fmt.Sprintf("rig/agent%d", i))},
		LaunchMode:   "fresh",
		CreatedAt:    time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC),
		GCBin:        "/usr/local/bin/gc",
		WorkDir:      "/Volumes/Sources/city/rigs/rig",
		Survivors:    []Survivor{{PID: 4242, Name: "node", Start: "Tue Nov 24 12:00:00 2026"}},
	}
}

func BenchmarkFileStore(b *testing.B) {
	b.Run("save", func(b *testing.B) {
		s := NewFileStore(b.TempDir())
		bind := benchBinding(1)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			s.Save(bind)
		}
	})
	b.Run("load", func(b *testing.B) {
		s := NewFileStore(b.TempDir())
		bind := benchBinding(1)
		s.Save(bind)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, ok := s.Load(bind.Name); !ok {
				b.Fatal("binding not found")
			}
		}
	})
	for _, n := range []int{10, 100} {
		b.Run(fmt.Sprintf("list-%d", n), func(b *testing.B) {
			s := NewFileStore(b.TempDir())
			for i := range n {
				s.Save(benchBinding(i))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if got, _ := s.List(); len(got) != n {
					b.Fatalf("List = %d bindings, want %d", len(got), n)
				}
			}
		})
	}
}
