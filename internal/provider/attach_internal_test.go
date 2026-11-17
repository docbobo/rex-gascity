package provider

import (
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
)

func TestWatchesAgent(t *testing.T) {
	b := Binding{WindowID: "w1", BlockID: "b1"}
	view := func(active rex.WindowID, blocks ...rex.Block) rex.SessionView {
		return rex.SessionView{ActiveWindowID: active, Windows: []rex.Window{{ID: "w1", Blocks: blocks}}}
	}
	if !watchesAgent(view("w1", rex.Block{ID: "b1"}), b) {
		t.Error("active window containing block should be true")
	}
	if watchesAgent(view("w1", rex.Block{ID: "other"}), b) {
		t.Error("active window without block should be false")
	}
	if watchesAgent(view("w2", rex.Block{ID: "b1"}), b) {
		t.Error("different active window should be false")
	}
}
