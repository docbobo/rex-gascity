package provider_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/docbobo/rex-gascity/internal/provider"
	"github.com/docbobo/rex-gascity/internal/rex"
	"github.com/docbobo/rex-gascity/internal/rex/faketest"
)

// Matrix answers that are not an RPP word.
const (
	wantNil     = "nil"     // the op succeeded
	wantUnknown = "unknown" // a plain error with no *InputOutcomeError (exit 1)
)

// Matrix faults. Each names one way an input op can fail (or not).
const (
	faultNone              = "none"
	faultNoBinding         = "no-binding"
	faultDeadProgram       = "dead-program"
	faultLookupUnavailable = "lookup-unavailable"
	faultLookupOther       = "lookup-other"
	faultSendUnavailable   = "send-unavailable"
	faultSendNotFound      = "send-not-found"
	faultSendOther         = "send-other"
	faultSecretRead        = "secret-read"
)

// inputMatrixRow is one op x fault cell of the design §2.1 matrix.
type inputMatrixRow struct {
	op    string
	fault string
	want  string
}

// inputMatrixRows is the locked matrix (#43 REQ-008). The success rows for
// interrupt and clear-scrollback lock "no input floor": a refactor that
// stamped the floor after a successful call would fail them. Neither op is
// guarded, so a secret read is a success row too (E15b).
var inputMatrixRows = []inputMatrixRow{
	{"clear-scrollback", faultNoBinding, "gone"},
	{"clear-scrollback", faultDeadProgram, "gone"},
	{"clear-scrollback", faultLookupUnavailable, "unavailable"},
	{"clear-scrollback", faultLookupOther, "not_sent"},
	{"clear-scrollback", faultSendUnavailable, "unavailable"},
	{"clear-scrollback", faultSendNotFound, "gone"},
	{"clear-scrollback", faultSendOther, wantUnknown},
	{"interrupt", faultNone, wantNil},
	{"clear-scrollback", faultNone, wantNil},
	{"interrupt", faultSecretRead, wantNil},
	{"clear-scrollback", faultSecretRead, wantNil},
}

// TestInputOutcomeMatrix runs every row against a started pane and checks the
// answer, the input floor, and for a success row the one expected Rex call.
func TestInputOutcomeMatrix(t *testing.T) {
	for _, row := range inputMatrixRows {
		t.Run(row.op+"/"+row.fault, func(t *testing.T) {
			fake := faketest.New()
			store := provider.NewMemoryActivityStore()
			p := nudgeStarted(t, fake, provider.WithActivityStore(store))
			fake.SentKeys = nil
			name := activityName
			if row.fault == faultNoBinding {
				name = "Indigo--ghost"
			}
			applyInputFault(t, fake, row.op, row.fault)
			floor := inputFloor(store)

			err := callInputOp(p, row.op, name)

			if got := matrixAnswer(err); got != row.want {
				t.Errorf("answer = %s (err %v), want %s", got, err, row.want)
			}
			if got := inputFloor(store); !got.Equal(floor) {
				t.Errorf("input floor moved from %v to %v, want unchanged", floor, got)
			}
			if row.want == wantNil {
				checkMatrixRexCall(t, fake, row.op)
			}
		})
	}
}

// callInputOp runs one input op the way the dispatcher does.
func callInputOp(p *provider.Provider, op, name string) error {
	ctx := context.Background()
	switch op {
	case "nudge":
		return p.Nudge(ctx, name, "hello")
	case "send-keys":
		return p.SendKeys(ctx, name, "a")
	case "interrupt":
		return p.Interrupt(ctx, name)
	case "clear-scrollback":
		return p.ClearScrollback(ctx, name)
	}
	panic("unknown input op " + op)
}

// applyInputFault arms fake with one fault. A send fault goes on the Rex call
// the op makes: BlockWrite for nudge, SendKeys for send-keys and interrupt,
// BlockClear for clear-scrollback.
func applyInputFault(t *testing.T, fake *faketest.Fake, op, fault string) {
	t.Helper()
	switch fault {
	case faultNone, faultNoBinding:
	case faultDeadProgram:
		fake.Processes = map[rex.BlockID]rex.ProcessInfo{activityBlock: {LastExit: &rex.ExitStatus{ExitCode: 0}}}
	case faultLookupUnavailable:
		fake.ProcessErr = rex.ErrUnavailable
	case faultLookupOther:
		fake.ProcessErr = errors.New("rex: protocol error")
	case faultSecretRead:
		fake.InputModes = map[rex.BlockID]rex.InputMode{activityBlock: secretReadMode}
	case faultSendUnavailable:
		setSendErr(fake, op, rex.ErrUnavailable)
	case faultSendNotFound:
		setSendErr(fake, op, rex.ErrNotFound)
	case faultSendOther:
		setSendErr(fake, op, errors.New("rex: connection reset"))
	default:
		t.Fatalf("unknown fault %q", fault)
	}
}

// setSendErr puts err on the Rex call op uses to deliver its input.
func setSendErr(fake *faketest.Fake, op string, err error) {
	switch op {
	case "nudge":
		fake.WriteErr = err
	case "send-keys", "interrupt":
		fake.SendKeysErr = err
	case "clear-scrollback":
		fake.ClearErr = err
	}
}

// matrixAnswer names what err means on the wire: nil, an RPP word, or unknown.
func matrixAnswer(err error) string {
	if err == nil {
		return wantNil
	}
	var outcome *provider.InputOutcomeError
	if errors.As(err, &outcome) {
		return outcome.Outcome
	}
	return wantUnknown
}

// inputFloor reads the pane's input floor from the activity store.
func inputFloor(store provider.Store[provider.SessionName, provider.ActivityRecord]) time.Time {
	rec, _, _ := store.Read(activityName)
	return rec.InputAt
}

// checkMatrixRexCall asserts a successful interrupt pressed ctrl+c once and a
// successful clear-scrollback cleared the pane once.
func checkMatrixRexCall(t *testing.T, fake *faketest.Fake, op string) {
	t.Helper()
	switch op {
	case "interrupt":
		if len(fake.SentKeys) != 1 || fake.SentKeys[0].BlockID != activityBlock ||
			len(fake.SentKeys[0].Keys) != 1 || fake.SentKeys[0].Keys[0] != "ctrl+c" {
			t.Errorf("sent keys = %+v, want one [ctrl+c] on %s", fake.SentKeys, activityBlock)
		}
	case "clear-scrollback":
		clears := 0
		for _, c := range fake.BlockCalls {
			if c.Op == "clear" && c.BlockID == activityBlock {
				clears++
			}
		}
		if clears != 1 {
			t.Errorf("block calls = %v, want one clear on %s", ops(fake), activityBlock)
		}
	}
}
