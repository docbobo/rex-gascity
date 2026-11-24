package provider

import (
	"errors"
	"fmt"
	"testing"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// TestClassifyInputRule pins the shared rule (#43 TS-2) for one op per
// divergence. The matrix test proves the same words end to end.
func TestClassifyInputRule(t *testing.T) {
	other := errors.New("x")
	for _, c := range []struct {
		op    inputOp
		stage inputStage
		err   error
		want  string // "" = unknown
	}{
		{inputSendKeys, stageValidate, other, "rejected"},
		{inputNudge, stageLookup, fmt.Errorf("%w: a has no pane", ErrSessionNotFound), "gone"},
		{inputInterrupt, stageLookup, rex.ErrUnavailable, "unavailable"},
		{inputNudge, stageLookup, other, ""},
		{inputSendKeys, stageLookup, other, "not_sent"},
		{inputSendKeys, stageGate, ErrSecretRead, "not_sent"},
		{inputNudge, stageGate, ErrSecretRead, "pending_interaction"},
		{inputInterrupt, stageGate, ErrSecretRead, ""},
		{inputNudge, stageGate, ErrNudgeClaimHeld, "busy"},
		{inputNudge, stageGate, ErrNudgeBusy, "busy"},
		{inputNudge, stageGate, ErrNudgeBlocked, "pending_interaction"},
		{inputNudge, stageGate, other, ""},
		{inputClearScrollback, stageSend, rex.ErrNotFound, "gone"},
		{inputSendKeys, stageSend, rex.ErrNotFound, ""},
		{inputInterrupt, stageSend, rex.ErrUnavailable, "unavailable"},
		{inputNudge, stageSend, other, "not_sent"},
		{inputClearScrollback, stageSend, other, ""},
		{inputNudge, stageSubmit, rex.ErrUnavailable, "unavailable"},
		{inputNudge, stageSubmit, other, "not_sent"},
	} {
		got := classifyInput(c.op, c.stage, c.err)
		var outcome *InputOutcomeError
		typed := errors.As(got, &outcome)
		switch {
		case c.want == "" && typed:
			t.Errorf("%s stage %d %v: outcome %q, want unknown", c.op, c.stage, c.err, outcome.Outcome)
		case c.want == "" && got != c.err:
			t.Errorf("%s stage %d %v: got %v, want err unchanged", c.op, c.stage, c.err, got)
		case c.want != "" && !typed:
			t.Errorf("%s stage %d %v: got %v, want outcome %q", c.op, c.stage, c.err, got, c.want)
		case c.want != "" && (outcome.Outcome != c.want || outcome.Detail != c.err.Error() || !errors.Is(got, c.err)):
			t.Errorf("%s stage %d %v: got %+v, want %q with the error text as detail", c.op, c.stage, c.err, outcome, c.want)
		}
	}
	if classifyInput(inputNudge, stageSend, nil) != nil {
		t.Error("classifyInput(nil) != nil")
	}
}

// gcAllowedInputWords is the per-op word set gc accepts on an input-outcome
// line. Copied from the gc working tree at 890a58a3e,
// internal/runtime/exec/outcome.go:45 (inputOutcomeOp) and :58
// (InputOutcomeAllowed): not_sent, rejected, unavailable and gone on all four
// driving ops; busy and pending_interaction on nudge only. Claim status:
// unverified at the v1.5.1 pin, where outcome.go is absent (#43 V3).
var gcAllowedInputWords = map[inputOp][]string{
	inputNudge:           {"not_sent", "rejected", "unavailable", "gone", "busy", "pending_interaction"},
	inputSendKeys:        {"not_sent", "rejected", "unavailable", "gone"},
	inputInterrupt:       {"not_sent", "rejected", "unavailable", "gone"},
	inputClearScrollback: {"not_sent", "rejected", "unavailable", "gone"},
}

// TestInputPoliciesStayInGcAllowList (#43 REQ-009) fails if a policy, or a
// fixed word classifyInput returns on a reachable op/stage pair, puts a word
// on an op that gc would read as malformed (busy on send-keys, say).
func TestInputPoliciesStayInGcAllowList(t *testing.T) {
	allowed := func(op inputOp, word string) bool {
		for _, w := range gcAllowedInputWords[op] {
			if w == word {
				return true
			}
		}
		return false
	}
	for op, policy := range inputPolicies {
		for field, word := range map[string]string{
			"lookupOther":  policy.lookupOther,
			"sendNotFound": policy.sendNotFound,
			"sendOther":    policy.sendOther,
			"secretRead":   policy.secretRead,
		} {
			if word != "" && !allowed(op, word) {
				t.Errorf("inputPolicies[%s].%s = %q, not allowed by gc on %s", op, field, word, op)
			}
		}
		// Fixed words: validate, lookup and send/submit reach every op.
		for _, word := range []string{"rejected", "gone", "unavailable"} {
			if !allowed(op, word) {
				t.Errorf("classifyInput can answer %q on %s, not allowed by gc", word, op)
			}
		}
	}
	// The claim-held, busy and blocked gate errors come only from nudge.
	for _, word := range []string{"busy", "pending_interaction"} {
		if !allowed(inputNudge, word) {
			t.Errorf("nudge gate answers %q, not allowed by gc", word)
		}
	}
	if len(inputPolicies) != len(gcAllowedInputWords) {
		t.Errorf("inputPolicies has %d ops, want the %d driving ops", len(inputPolicies), len(gcAllowedInputWords))
	}
}
