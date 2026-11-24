package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// inputOp names one of the four RPP driving ops. The values are the RPP op
// words, so a detail or a policy reads the same as the wire.
type inputOp string

const (
	inputNudge           inputOp = "nudge"
	inputSendKeys        inputOp = "send-keys"
	inputInterrupt       inputOp = "interrupt"
	inputClearScrollback inputOp = "clear-scrollback"
)

// inputStage is where in an input op a failure happened. The stage decides
// the outcome word; the op's policy fills in the per-op gaps.
type inputStage int

const (
	// stageValidate is the check of the caller's input before any Rex call.
	stageValidate inputStage = iota
	// stageLookup is the resolve of the name to a live pane.
	stageLookup
	// stageGate is a refusal by a guard or gate before anything is typed.
	stageGate
	// stageSend is the Rex call that delivers the input.
	stageSend
	// stageSubmit is nudge's Enter after its text was typed. It follows the
	// stageSend rule today and is its own stage so that rule can split later.
	stageSubmit
)

// inputPolicy is how one op departs from the shared rule. An empty word means
// the failure stays unknown: a plain error, exit 1 on the wire. For secretRead
// an empty word means the op is not guarded at all.
type inputPolicy struct {
	// lookupOther answers a lookup failure that is neither gone nor
	// unavailable.
	lookupOther string
	// sendNotFound answers rex.ErrNotFound on the send. Empty falls through to
	// sendOther.
	sendNotFound string
	// sendOther answers a send failure that is not unavailable (and not a
	// mapped ErrNotFound).
	sendOther string
	// secretRead answers a pane reading a secret (#48). Empty: not guarded.
	secretRead string
	// notesInput stamps the input floor after a delivered send.
	notesInput bool
}

// inputPolicies holds every per-op divergence from the shared rule (#43,
// design §2.1). Each field keeps today's behavior; changing one is a behavior
// change that needs its own two-source check (requirements Q1-Q4).
var inputPolicies = map[inputOp]inputPolicy{
	// Q1: any other lookup failure stays unknown, a deliberate pass-through.
	// Q3: a failed Enter after typed text is not_sent.
	inputNudge: {lookupOther: "", sendNotFound: "", sendOther: "not_sent", secretRead: "pending_interaction", notesInput: true},
	// Q2: a send failure other than unavailable may have pressed the key, so
	// it stays unknown.
	inputSendKeys: {lookupOther: "not_sent", sendNotFound: "", sendOther: "", secretRead: "not_sent", notesInput: true},
	// Q4: interrupt stamps no input floor.
	inputInterrupt: {lookupOther: "not_sent", sendNotFound: "", sendOther: "", secretRead: "", notesInput: false},
	// GCREX-57: a pane that vanished mid-clear is gone.
	inputClearScrollback: {lookupOther: "not_sent", sendNotFound: "gone", sendOther: "", secretRead: "", notesInput: false},
}

// classifyInput turns an input-op failure at stage into its RPP outcome. It is
// the only code that builds an *InputOutcomeError for an input op. nil stays
// nil; an unknown outcome returns err unchanged; a typed outcome wraps err with
// err.Error() as its detail, so the wire detail is the error text.
func classifyInput(op inputOp, stage inputStage, err error) error {
	if err == nil {
		return nil
	}
	if word := inputOutcomeWord(inputPolicies[op], stage, err); word != "" {
		return &InputOutcomeError{Outcome: word, Detail: err.Error(), Err: err}
	}
	return err
}

// inputOutcomeWord is the rule behind classifyInput. "" means unknown.
func inputOutcomeWord(policy inputPolicy, stage inputStage, err error) string {
	switch stage {
	case stageValidate:
		return "rejected"
	case stageLookup:
		switch {
		case errors.Is(err, ErrSessionNotFound):
			return "gone"
		case errors.Is(err, rex.ErrUnavailable):
			return "unavailable"
		default:
			return policy.lookupOther
		}
	case stageGate:
		switch {
		case errors.Is(err, ErrSecretRead):
			return policy.secretRead
		case errors.Is(err, ErrNudgeClaimHeld), errors.Is(err, ErrNudgeBusy):
			return "busy"
		case errors.Is(err, ErrNudgeBlocked):
			return "pending_interaction"
		default:
			return ""
		}
	case stageSend, stageSubmit:
		switch {
		case errors.Is(err, rex.ErrNotFound) && policy.sendNotFound != "":
			return policy.sendNotFound
		case errors.Is(err, rex.ErrUnavailable):
			return "unavailable"
		default:
			return policy.sendOther
		}
	}
	return ""
}

// resolveInputPane resolves name to a live pane for op, with a lookup failure
// already classified.
func (p *Provider) resolveInputPane(ctx context.Context, op inputOp, name string) (Binding, error) {
	binding, err := p.livePane(ctx, name)
	if err != nil {
		return Binding{}, classifyInput(op, stageLookup, err)
	}
	return binding, nil
}

// inputCall is one send-keys, interrupt or clear-scrollback delivery. send
// makes the Rex call and wraps its error with the op and name.
type inputCall struct {
	op    inputOp
	name  string
	guard bool
	send  func(ctx context.Context, b Binding) error
}

// deliverInput runs the shared skeleton: resolve the pane, refuse a pane
// reading a secret when guard is set (#48), send, then stamp the input floor
// when the op's policy says so. Each failure is classified at its stage.
func (p *Provider) deliverInput(ctx context.Context, c inputCall) error {
	binding, err := p.resolveInputPane(ctx, c.op, c.name)
	if err != nil {
		return err
	}
	if c.guard && p.readingSecret(ctx, binding) {
		return classifyInput(c.op, stageGate, fmt.Errorf("%s %s: %w", c.op, c.name, ErrSecretRead))
	}
	if err := c.send(ctx, binding); err != nil {
		return classifyInput(c.op, stageSend, err)
	}
	// Input pressed into the pane is input even when the screen does not
	// change, so record the floor for idle detection.
	if inputPolicies[c.op].notesInput {
		p.noteInput(binding)
	}
	return nil
}
