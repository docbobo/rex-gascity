package main

import (
	"encoding/json"
	"runtime/debug"
	"testing"
)

// TestVersionReportFrom pins the `version` op's resolution: the module version
// and VCS revision come from the embedded build info, an -ldflags override
// wins, and the Go placeholder "(devel)" reads as unknown.
func TestVersionReportFrom(t *testing.T) {
	bi := &debug.BuildInfo{
		Main: debug.Module{Path: "github.com/docbobo/rex-gascity", Version: "v1.2.3"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abc123def456"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	t.Run("module version and vcs from build info", func(t *testing.T) {
		rep := versionReportFrom(bi, "")
		if rep.Module != "github.com/docbobo/rex-gascity" {
			t.Errorf("module = %q", rep.Module)
		}
		if rep.Version != "v1.2.3" {
			t.Errorf("version = %q, want v1.2.3", rep.Version)
		}
		if rep.Revision != "abc123def456" {
			t.Errorf("revision = %q, want abc123def456", rep.Revision)
		}
		if !rep.Modified {
			t.Error("modified = false, want true")
		}
	})
	t.Run("ldflags override wins", func(t *testing.T) {
		rep := versionReportFrom(bi, "0.1.0")
		if rep.Version != "0.1.0" {
			t.Errorf("version = %q, want 0.1.0", rep.Version)
		}
	})
	t.Run("devel reads as unknown", func(t *testing.T) {
		rep := versionReportFrom(&debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "")
		if rep.Version != "" {
			t.Errorf("version = %q, want empty", rep.Version)
		}
	})
	t.Run("nil build info with override", func(t *testing.T) {
		rep := versionReportFrom(nil, "0.1.0")
		if rep.Version != "0.1.0" || rep.Module != "" {
			t.Errorf("report = %+v, want version 0.1.0 and empty module", rep)
		}
	})
}

// TestVersionOp proves the `version` op prints one JSON line carrying the
// stamped build version, and needs neither a city nor a Rex client.
func TestVersionOp(t *testing.T) {
	h, _ := newHarness(t)
	old := buildVersion
	buildVersion = "0.1.0"
	t.Cleanup(func() { buildVersion = old })

	r := h.must("", "version")
	var rep versionReport
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("version output %q is not one JSON object: %v", r.stdout, err)
	}
	if rep.Version != "0.1.0" {
		t.Errorf("version = %q, want 0.1.0", rep.Version)
	}
}
