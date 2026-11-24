package provider_test

import (
	"context"
	"testing"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// TestNudgeBaselineScreenIsReadBeforeTyping proves that, for a pane with no
// program_status record, the nudge reads its baseline screen before BlockWrite
// (issue #44 REQ-010). The fake pane shows the typed text only once it is
// written. A baseline read after typing would match every poll, and the nudge
// would go unconfirmed.
func TestNudgeBaselineScreenIsReadBeforeTyping(t *testing.T) {
	fake := faketest.New()
	p := nudgeStarted(t, fake)
	written := false
	fake.WriteFunc = func(rex.SessionID, rex.BlockID, string) { written = true }
	fake.FormatFunc = func(rex.BlockID) string {
		if written {
			return "prompt> hello"
		}
		return "prompt>"
	}

	report, err := p.NudgeReport(context.Background(), activityName, "hello", provider.NudgeModeWait)
	if err != nil {
		t.Fatalf("NudgeReport: %v", err)
	}
	if !report.Delivered || !report.Confirmed {
		t.Errorf("report = %+v, want delivered and confirmed by the format change", report)
	}
	if report.Source != provider.NudgeSourceFormatPoll {
		t.Errorf("Source = %q, want format_poll", report.Source)
	}
}
