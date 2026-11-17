package provider

import (
	"context"
	"testing"

	"github.com/docbobo/rex-gas-city/internal/rex"
	"github.com/docbobo/rex-gas-city/internal/rex/faketest"
)

func TestNextSplit(t *testing.T) {
	blk := func(id string, r [4]float64) rex.Block { return rex.Block{ID: rex.BlockID(id), Rect: r} }

	tests := []struct {
		name       string
		blocks     []rex.Block
		wantAnchor rex.BlockID
		wantDir    string
		wantOK     bool
	}{
		{
			name: "largest of three",
			blocks: []rex.Block{
				blk("block:a", [4]float64{0, 0, 0.25, 1}),
				blk("block:b", [4]float64{0.25, 0, 0.75, 0.5}),
				blk("block:c", [4]float64{0.25, 0.5, 0.75, 0.5}),
			},
			wantAnchor: "block:b", wantDir: "horizontal", wantOK: true,
		},
		{
			name: "tall pane splits vertically",
			blocks: []rex.Block{
				blk("block:a", [4]float64{0, 0, 0.5, 1}),
				blk("block:b", [4]float64{0.5, 0, 0.5, 0.5}),
				blk("block:c", [4]float64{0.5, 0.5, 0.5, 0.5}),
			},
			wantAnchor: "block:a", wantDir: "vertical", wantOK: true,
		},
		{
			name: "square pane splits horizontally",
			blocks: []rex.Block{
				blk("block:a", [4]float64{0, 0, 1, 1}),
			},
			wantAnchor: "block:a", wantDir: "horizontal", wantOK: true,
		},
		{
			name: "tie goes to the lowest block id",
			blocks: []rex.Block{
				blk("block:c", [4]float64{0.5, 0, 0.5, 0.5}),
				blk("block:b", [4]float64{0.5, 0.5, 0.5, 0.5}),
				blk("block:a", [4]float64{0, 0, 0.5, 1}),
			},
			wantAnchor: "block:a", wantDir: "vertical", wantOK: true,
		},
		{
			name: "no geometry falls back to the first block",
			blocks: []rex.Block{
				{ID: "block:z"}, {ID: "block:a"},
			},
			wantAnchor: "block:z", wantDir: "horizontal", wantOK: true,
		},
		{
			name: "implausible rect is ignored",
			blocks: []rex.Block{
				blk("block:a", [4]float64{0, 0, 5, 5}),
				blk("block:b", [4]float64{0, 0, 0.5, 0.25}),
			},
			wantAnchor: "block:b", wantDir: "horizontal", wantOK: true,
		},
		{
			name: "block without a rect is skipped when others have one",
			blocks: []rex.Block{
				{ID: "block:a"},
				blk("block:b", [4]float64{0, 0, 0.25, 0.5}),
			},
			wantAnchor: "block:b", wantDir: "vertical", wantOK: true,
		},
		{name: "empty window", blocks: nil, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			anchor, dir, ok := nextSplit(rex.Window{Blocks: tt.blocks})
			if anchor != tt.wantAnchor || dir != tt.wantDir || ok != tt.wantOK {
				t.Errorf("nextSplit = (%q, %q, %v), want (%q, %q, %v)",
					anchor, dir, ok, tt.wantAnchor, tt.wantDir, tt.wantOK)
			}
		})
	}
}

func TestStartSplitsTheLargestPaneAlongItsLongerAxis(t *testing.T) {
	fake := faketest.New()
	fake.Sessions = []rex.Session{{ID: "session:gc:acme", Label: "gc:acme"}}
	fake.View = rex.SessionView{
		Windows: []rex.Window{{
			ID:    "window:Indigo",
			Label: "Indigo",
			Blocks: []rex.Block{
				{ID: "block:dog-1", Label: "dog-1", Rect: [4]float64{0, 0, 0.25, 1}},
				{ID: "block:dog-2", Label: "dog-2", Rect: [4]float64{0.25, 0, 0.75, 0.25}},
				{ID: "block:dog-3", Label: "dog-3", Rect: [4]float64{0.25, 0.25, 0.75, 0.75}},
			},
		}},
	}
	p := New("acme", fake)

	if err := p.Start(context.Background(), "Indigo--dog-4", Config{Argv: []string{"claude"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if len(fake.CreatedSplits) != 1 {
		t.Fatalf("created splits = %d, want 1", len(fake.CreatedSplits))
	}
	s := fake.CreatedSplits[0]
	if s.Anchor != "block:dog-3" || s.Direction != "horizontal" || s.Side != "after" {
		t.Errorf("split = %s %s %s, want block:dog-3 horizontal after", s.Anchor, s.Direction, s.Side)
	}
	if s.Spec.Focus {
		t.Error("split took focus, want false")
	}
}
