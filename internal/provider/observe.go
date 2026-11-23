package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/docbobo/rex-gascity/internal/rex"
)

// The observe.v1 profile's read and kill ops. The handshake declares the
// observe.v1 token unconditionally (GCREX-111).

// Observe states, matching gc's wire vocabulary.
const (
	ObserveAbsent  = "absent"
	ObserveRunning = "running"
	ObserveDead    = "dead"
	ObserveUnknown = "unknown"
)

// ObserveObject is a box's identity on the backend: stable for its life and
// new for every new box under the name.
type ObserveObject struct {
	ID      string `json:"id"`
	Created string `json:"created"`
}

// ObserveAnswer is one `observe` answer. Object is present only for a box that
// holds the name (running or dead). Identity has no omitempty because nil and
// an empty map are different answers: nil marshals as null, "identity not yet
// readable" to gc (rule 5), and an empty map as {}, gc's "a box with no gc
// identity (one an operator made)".
type ObserveAnswer struct {
	State    string            `json:"state"`
	Object   *ObserveObject    `json:"object,omitempty"`
	Agent    string            `json:"agent,omitempty"`
	AgentPID string            `json:"agent_pid,omitempty"`
	Identity map[string]string `json:"identity"`
}

// InventoryEntry is one session in an `inventory` snapshot. A session whose
// attributes cannot be read carries only its name. Identity, like
// ObserveAnswer's, marshals {} for a box with no gc identity and null for one
// whose identity is not yet readable.
type InventoryEntry struct {
	Name         string            `json:"name"`
	State        string            `json:"state,omitempty"`
	Object       *ObserveObject    `json:"object,omitempty"`
	Agent        string            `json:"agent,omitempty"`
	Identity     map[string]string `json:"identity"`
	LastActivity string            `json:"last_activity,omitempty"`
}

// InventoryAnswer is the `inventory` wire object.
type InventoryAnswer struct {
	Complete bool             `json:"complete"`
	Sessions []InventoryEntry `json:"sessions"`
}

// KillObjectRequest is `kill-object`'s stdin.
type KillObjectRequest struct {
	Object   ObserveObject `json:"object"`
	Mode     string        `json:"mode"`
	AgentPID string        `json:"agent_pid,omitempty"`
}

// observeObjectFor maps a stored binding onto its wire object. The pane's
// block is the object; a provisional record without a block falls back to its
// window. The creation time is the binding's record time, formatted so a
// second observe or a kill-object re-check compares equal.
func observeObjectFor(b Binding) ObserveObject {
	id := string(b.BlockID)
	if id == "" {
		id = string(b.WindowID)
	}
	created := observeCreated(b.CreatedAt)
	// A record the provider holds is one incarnation: Rex may reuse a block id
	// for a relaunched pane under the same labels, so the object id pairs the
	// block with the creation time. The pair is byte-identical across calls for
	// one box (both come from the binding) and differs for every new start,
	// which is what gc's object check compares (RPP-OBSERVE-005). A discovered
	// pane has no record and no creation time, so its block id stands alone.
	if b.CreatedAt.IsZero() {
		return ObserveObject{ID: id, Created: created}
	}
	return ObserveObject{ID: id + "/" + created, Created: created}
}

const (
	// observeProbeBound is rule 11's answer bound: gc kills an observe call at
	// 4 s and reads the kill as unknown.
	observeProbeBound = 4 * time.Second
	// observeMargin is the time Observe keeps back for the dispatcher's own
	// work (stdin drain, answer encode, answer write), so it answers unknown
	// itself instead of being killed mid-call.
	observeMargin = 250 * time.Millisecond
)

// observeBudget caps an Observe op's Rex calls at the probe bound, or at the
// deadline gc set on the op when that is sooner. gc sets GC_EXEC_DEADLINE_MS
// (whole milliseconds left until the kill) on every op under a deadline and
// reads an unanswered call as unknown, so a slow backend must end in unknown
// here rather than in an error or an overrun (RPP-OBSERVE-013).
func observeBudget(ctx context.Context) (context.Context, context.CancelFunc) {
	budget := observeProbeBound - observeMargin
	if ms, err := strconv.ParseInt(os.Getenv("GC_EXEC_DEADLINE_MS"), 10, 64); err == nil && ms > 0 {
		if d := time.Duration(ms)*time.Millisecond - observeMargin; d < budget {
			budget = max(d, time.Millisecond)
		}
	}
	return context.WithTimeout(ctx, budget)
}

// observeCreated formats a binding's creation time as the wire's created
// string. A zero time reads as the zero instant rather than an empty field,
// because a present box must carry a non-empty object.
func observeCreated(t time.Time) string {
	if t.IsZero() {
		return time.Time{}.UTC().Format(time.RFC3339)
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// observeIdentity reads the identity the meta sidecar holds for the box. It
// serves the same keys Start seeds and get-meta answers, so observe reports one
// identity with get-meta, including a value an operator set-meta'd after the
// launch. It returns nil when a key cannot be read: the wire reads null as "not
// yet readable" (rule 5), and a partial identity must not pass for the whole
// one, which would end the read early with keys missing. It returns an empty
// set only when every key was readable and the start config carried no
// identity.
func (p *Provider) observeIdentity(n SessionName) map[string]string {
	id := map[string]string{}
	for _, key := range startMetaKeys() {
		v, ok, err := p.metaGet(n, key)
		if err != nil {
			return nil
		}
		if ok && v != "" {
			id[key] = v
		}
	}
	return id
}

// Observe answers `observe <name>`: absent when nothing holds the name,
// running or dead for the box that holds it, unknown when Rex cannot be
// reached. names are gc's process-name hints from stdin, with which a running
// box reports its agent. It is a read and never mutates the session; a vanished
// record is left for the next Start or Stop to replace.
func (p *Provider) Observe(ctx context.Context, name string, names []string) (ObserveAnswer, error) {
	ctx, cancel := observeBudget(ctx)
	defer cancel()
	n := SessionName(name)
	bd, ok := p.Binding(name)
	// Scan without adopting when there is no recorded block yet. In
	// particular, a provisional start has labels but no block ID to probe.
	if !ok || bd.BlockID == "" {
		var err error
		bd, ok, err = p.binder.find(ctx, n, bd, nil)
		if err != nil {
			if observeUnreadable(err) {
				return ObserveAnswer{State: ObserveUnknown}, nil
			}
			return ObserveAnswer{}, err
		}
		if !ok {
			return ObserveAnswer{State: ObserveAbsent}, nil
		}
	}
	state, holder, err := p.boxState(ctx, bd)
	if err != nil {
		if observeUnreadable(err) {
			return ObserveAnswer{State: ObserveUnknown}, nil
		}
		return ObserveAnswer{}, err
	}
	if state == ObserveAbsent {
		return ObserveAnswer{State: ObserveAbsent}, nil
	}
	// Start can make the pane visible while the backend read is in flight.
	// Recheck the store before deciding a pane found by label is ownerless.
	current, recorded := p.Binding(name)
	sameSession := recorded && current.SessionID == holder.SessionID
	sameBlock := sameSession && current.BlockID == holder.BlockID
	if sameSession && current.BlockID == "" && current.Attempt != "" {
		// A pane is visible, but Start has not confirmed its block ID.
		// Its object and identity cannot yet be attributed to this launch.
		return ObserveAnswer{State: ObserveUnknown}, nil
	}
	if sameBlock {
		holder = current
	}
	obj := observeObjectFor(holder)
	ans := ObserveAnswer{State: state, Object: &obj}
	if sameBlock && !current.Adopted {
		ans.Identity = p.observeIdentity(current.Name)
	} else {
		ans.Identity = map[string]string{}
	}
	if state == ObserveDead {
		ans.Agent = "dead"
	}
	// Agent liveness is a verdict gc acts on, so only a certain answer is
	// reported (RPP-OBSERVE-006): the box root's process snapshot must both be
	// readable and hold one of the names.
	sameProbedBlock := holder.SessionID == bd.SessionID && holder.BlockID == bd.BlockID
	if state == ObserveRunning && len(names) > 0 && (sameBlock || sameProbedBlock) {
		if root := p.foregroundPID(ctx, holder); root > 0 {
			if agent, pid := p.agentState(ctx, root, expandProcessNames(names, bindingGCBin(holder))); agent != agentUnknown {
				ans.Agent = agent
				if pid > 0 {
					ans.AgentPID = strconv.Itoa(pid)
				}
			}
		}
	}
	return ans, nil
}

// foregroundPID is the pid of the box's foreground process, the root of the
// agent subtree. It answers 0 when Rex cannot name it, which makes the agent
// unknown rather than dead.
func (p *Provider) foregroundPID(ctx context.Context, bd Binding) int {
	info, err := p.client.BlockProcess(ctx, bd.SessionID, bd.BlockID)
	if err != nil || info.Foreground == nil {
		return 0
	}
	return info.Foreground.PID
}

// observeUnreadable reports a Rex answer Observe cannot get: the backend is
// unreachable, or the op's own deadline ended the call. Both read as unknown on
// the wire (rule 11).
func observeUnreadable(err error) bool {
	return errors.Is(err, rex.ErrUnavailable) || errors.Is(err, context.DeadlineExceeded)
}

// retireStale finishes the cleanup of a record whose block is already gone: the
// meta sidecar and activity of that incarnation, then the record itself. It
// never touches Rex, so it cannot disturb a replacement; the caller checked the
// record still describes the object that was asked about.
func (p *Provider) retireStale(bd Binding) error {
	if err := p.clearIncarnations(bd.Name); err != nil {
		return err
	}
	p.binder.Forget(bd.Name)
	return nil
}

// boxState reports what holds the name a binding records. Rex proving the
// recorded block gone does not prove the name is free (observe.v1 rule 7): a
// restore or an operator can put another pane under the same labels, so the
// live scan decides. It claims no state of its own; unknown is the caller's
// answer for an unreadable backend. It is a read: a vanished record is left for
// the next Start or Stop to replace.
func (p *Provider) boxState(ctx context.Context, bd Binding) (string, Binding, error) {
	info, err := p.client.BlockProcess(ctx, bd.SessionID, bd.BlockID)
	switch {
	case errors.Is(err, rex.ErrNotFound):
		// Find a live pane under the record's labels, without ResolveLive's
		// stale-record drop or its save: Observe is a read, and only Start or
		// Stop replaces the record.
		live, ok, err := p.binder.find(ctx, bd.Name, bd, nil)
		if err != nil {
			return "", Binding{}, err
		}
		if !ok {
			return ObserveAbsent, Binding{}, nil
		}
		info, err := p.client.BlockProcess(ctx, live.SessionID, live.BlockID)
		if errors.Is(err, rex.ErrNotFound) {
			return ObserveAbsent, Binding{}, nil
		}
		if err != nil {
			return "", Binding{}, err
		}
		if info.LastExit != nil {
			return ObserveDead, live, nil
		}
		return ObserveRunning, live, nil
	case err != nil:
		return "", Binding{}, err
	case info.LastExit != nil:
		return ObserveDead, bd, nil
	default:
		return ObserveRunning, bd, nil
	}
}

// Inventory snapshots every session the provider tracks for the city. It is a
// read: no meta is written and no session changes. Each entry carries the state,
// identity and last activity observe or get-last-activity would report for it,
// so gc need not read every box. The activity comes from the provider's store;
// a box gc has not polled yet reports its start time as the floor.
//
// A box whose liveness cannot be read is listed with no state and the answer
// claims no completeness, because a backend fault proves nothing absent. A pane
// under a city label that names no binding proves nothing about this store: it
// may belong to another store of the same city, or to a stop still closing, and
// neither can be told apart from here. So it does not affect the answer.
//
// ponytail: one Rex block-process call and, for a running box, one
// program_status read per session, since Rex has no bulk liveness or status
// query (session.view carries no process data). The status read is what keeps
// last_activity equal to get-last-activity's value (review comment 3); drop it
// for a bulk query or a trusted last_exit cache if a city grows past the
// inventory call's time budget.
func (p *Provider) Inventory(ctx context.Context) (InventoryAnswer, error) {
	lister, ok := p.bindings.(Lister[Binding])
	if !ok {
		return InventoryAnswer{Complete: false, Sessions: []InventoryEntry{}}, nil
	}
	bds := lister.List()
	complete := true
	listed := make(map[SessionName]bool, len(bds))
	sessions := make([]InventoryEntry, 0, len(bds))
	for _, bd := range bds {
		if listed[bd.Name] {
			continue
		}
		if bd.BlockID == "" {
			// A record written before its pane exists (a start in flight) is
			// not a box: the name is not running yet, and Rex rejects the empty
			// block id.
			continue
		}
		state, holder, err := p.boxState(ctx, bd)
		switch {
		case errors.Is(err, rex.ErrUnavailable):
			complete = false
			// The entry still carries the binding's object; only its state is
			// unknown.
			holder = bd
		case err != nil:
			return InventoryAnswer{}, err
		case state == ObserveAbsent:
			// Rex proved nothing holds the name: omit it, as rule 7 allows.
			continue
		}
		obj := observeObjectFor(holder)
		e := InventoryEntry{
			Name:   string(bd.Name),
			State:  state,
			Object: &obj,
		}
		if holder.BlockID == bd.BlockID && !bd.Adopted {
			e.Identity = p.observeIdentity(bd.Name)
		} else {
			e.Identity = map[string]string{}
		}
		switch state {
		case ObserveDead:
			e.Agent = "dead"
		case ObserveRunning:
			e.LastActivity = p.inventoryActivity(ctx, holder)
		}
		listed[bd.Name] = true
		sessions = append(sessions, e)
	}
	return InventoryAnswer{Complete: complete, Sessions: sessions}, nil
}

// inventoryActivity stamps a running session's last activity with the value
// GetLastActivity would report for it, without writing anything. A
// program_status record decides it, as it does for GetLastActivity; otherwise
// the activity store answers, floored by the binding's start time so a session
// gc has not polled yet still answers one. A record from another incarnation is
// ignored, and an unreadable status leaves the store's value standing: a
// listing is a floor, never a claim that a box is active.
//
// The program_status read costs one more Rex call per running session than the
// liveness probe alone (see Inventory's budget note). Skipping it would report
// a working or blocked agent's stale store stamp, which an idle check would
// then read as an idle box.
func (p *Provider) inventoryActivity(ctx context.Context, b Binding) string {
	rec, found := p.activityRead(b.Name)
	at := b.CreatedAt
	if found && rec.Identity == string(b.BlockID) {
		at = laterOf(at, laterOf(rec.At, rec.InputAt))
	}
	if statusAt, ok, err := p.statusActivity(ctx, b); err == nil && ok {
		// The status answer already carries the input floor, and GetLastActivity
		// returns it as is; the store's stamp must not make inventory newer.
		at = statusAt
	}
	if at.IsZero() {
		// An adopted pane has no stored start time; now is the only floor that
		// cannot call an idle box stale.
		at = time.Now().UTC()
	}
	return at.UTC().Format(time.RFC3339Nano)
}

// KillObject re-checks that the exact object still holds the name and, if so,
// kills it. It never kills by name: an object that names no box, or whose id
// or creation time does not match the stored binding, answers `gone`.
//
// The whole check-then-kill runs under the name lock, so a start cannot slip a
// new box under the name between the re-check and the close. Corpse mode kills
// only a box nothing runs in and no client has open, and answers `live`
// otherwise. Zombie mode needs the box running, unattached, and its agent
// provably dead by the RPP-OBSERVE-006 snapshot; anything less answers
// `changed`.
func (p *Provider) KillObject(ctx context.Context, name string, req KillObjectRequest) (string, error) {
	if strings.TrimSpace(req.Object.ID) == "" || strings.TrimSpace(req.Object.Created) == "" {
		return "", errors.New("kill-object: object id and created are required")
	}
	if req.Mode != "corpse" && req.Mode != "zombie" {
		return "", fmt.Errorf("kill-object: mode %q, want corpse or zombie", req.Mode)
	}
	var res string
	err := p.withNameLock(ctx, SessionName(name), func() error {
		var err error
		res, err = p.killObjectLocked(ctx, name, req)
		return err
	})
	if err != nil {
		return "", err
	}
	return res, nil
}

// killObjectLocked is KillObject's re-check and kill, run under the name lock.
func (p *Provider) killObjectLocked(ctx context.Context, name string, req KillObjectRequest) (string, error) {
	bd, ok := p.Binding(name)
	if !ok {
		return "gone", nil
	}
	obj := observeObjectFor(bd)
	if req.Object.ID != obj.ID || req.Object.Created != obj.Created {
		return "gone", nil
	}
	state, holder, err := p.boxState(ctx, bd)
	if err != nil {
		return "", err
	}
	exact := holder.BlockID == bd.BlockID && holder.WindowID == bd.WindowID
	if !exact {
		// The recorded block is gone or a new box holds the name: the object
		// names no box, and the exact-binding close has nothing to close. If
		// this record is still ours, finish the retirement a previous
		// kill-object could not complete (closed the block, then failed
		// cleanup): a retry must not answer `gone` over stale meta/activity.
		if state == ObserveAbsent {
			// A previous stop may have recorded survivors it could not reap
			// before the pane vanished, so the record is their only name: reap
			// them before retiring it, or a retry answers gone over live pids.
			if _, err := p.withSameBinding(ctx, name, bd, func(cur Binding) error {
				// The state-lock read may carry survivors a canonical Stop
				// recorded after the check above; reaping the earlier snapshot
				// would leave them and then delete their only record.
				if err := p.reapSurvivors(ctx, cur, survivorProcs(cur.Survivors)); err != nil {
					return err
				}
				return p.retireStale(cur)
			}); err != nil {
				return "", err
			}
		}
		return "gone", nil
	}
	if req.Mode == "corpse" && state == ObserveRunning {
		// corpse reaps only a box nothing runs in: one that came back between
		// gc's observe and this kill is still live.
		return "live", nil
	}
	if req.Mode == "zombie" && state == ObserveDead {
		// A pane whose program exited is no zombie: nothing runs there for a
		// killed agent to come back to, and gc reaps it with corpse.
		return "changed", nil
	}
	attached, err := p.attached(ctx, bd)
	if err != nil {
		// An unreadable attachment is not proof nobody is watching.
		return "", err
	}
	if attached {
		// zombie kills a dead box nothing watches; corpse reaps only an
		// unwatched one, and a watched box is still live to gc.
		if req.Mode == "zombie" {
			return "changed", nil
		}
		return "live", nil
	}
	if req.Mode == "zombie" {
		// The liveness answer comes after the attachment read, so a shell that
		// started a matching agent meanwhile is seen and answers changed, not
		// reaped by the close below. The window left is the one between this
		// snapshot and the close.
		dead, err := p.deadAgent(ctx, bd, req.AgentPID)
		if err != nil {
			return "", err
		}
		if !dead {
			// Not provably dead: a killed box whose agent came back would be
			// gc's zombie reaped alive, so answer changed and kill nothing.
			return "changed", nil
		}
	}
	// Close exactly the object that was re-checked, never by name: a fresh
	// resolve could close a pane that took the name's labels after this one
	// vanished. The name lock is already held, and withSameBinding re-proves
	// under the state lock that the record still names this object before the
	// close, so a stop or start of the canonical name (a different name lock)
	// cannot swap the binding in between.
	same, err := p.withSameBinding(ctx, name, bd, func(Binding) error {
		return p.stopBinding(ctx, SessionName(name), bd)
	})
	if errors.Is(err, errBlockGone) {
		// Another client closed the pane between the corpse check and the
		// close; the retirement finished, so this object is gone, not killed.
		return "gone", nil
	}
	if err != nil {
		return "", err
	}
	if !same {
		// A start took the name after the check: the record is the
		// replacement's, and it stays untouched.
		return "gone", nil
	}
	return "killed", nil
}

// deadAgent proves the box's agent is gone, for zombie mode's kill. The start
// meta names the agent processes; the box's root pid must still be readable, and
// a fresh host snapshot must walk the root's subtree without finding one of the
// names. agentPID is the pid the caller saw Observe report: that is the matched
// agent's pid, which can be a descendant of the root, so it is re-checked only
// while the host still shows it, and a live pid outside the root's subtree names
// another box. Anything it cannot prove is not dead: an adopted binding whose
// sidecar belongs to an earlier incarnation, an absent record, or an unreadable
// snapshot answers false.
func (p *Provider) deadAgent(ctx context.Context, bd Binding, agentPID string) (bool, error) {
	if bd.Adopted {
		return false, nil
	}
	raw, ok, err := p.metaGet(bd.Name, "GT_PROCESS_NAMES")
	if err != nil || !ok {
		return false, err
	}
	names := expandProcessNames(strings.Split(raw, ","), bindingGCBin(bd))
	root := p.foregroundPID(ctx, bd)
	if root <= 0 || len(names) == 0 {
		return false, nil
	}
	table, err := p.processHost.Table(ctx, true)
	if err != nil {
		return false, nil
	}
	if agentPID != "" {
		pid, err := strconv.Atoi(agentPID)
		if err != nil || pidOutsideSubtree(table, root, pid) {
			return false, nil
		}
	}
	agent, _ := snapshotAgent(table, root, names)
	return agent == agentDead, nil
}

// withSameBinding runs fn under the state lock only while the name still
// records bd: same block and window and the same creation time. A start that
// took the name since the caller's check leaves a different binding, and fn
// does not run. Start writes its provisional binding under the state lock, so
// a replacement cannot land between the check and the mutation. fn receives
// the record read under the lock, which can carry state (survivors) the
// caller's earlier copy does not. It reports whether fn ran.
func (p *Provider) withSameBinding(ctx context.Context, name string, bd Binding, fn func(Binding) error) (bool, error) {
	same := false
	err := p.withStateLock(ctx, func() error {
		got, ok := p.Binding(name)
		if !ok || !bd.sameIncarnation(got) {
			return nil
		}
		same = true
		return fn(got)
	})
	return same, err
}
