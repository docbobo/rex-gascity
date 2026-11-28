package provider

import (
	"errors"
	"fmt"
)

// ErrLockOrder is returned when a lock section is entered in an order the
// provider does not permit, or when a non-reentrant lock is taken twice. Every
// lock section in lock.go opens through lockSection, so the guard sees every
// lock the runtime takes.
var ErrLockOrder = errors.New("provider: lock taken out of order")

// lockKind names the provider's four kinds of lock.
type lockKind uint8

const (
	lockState lockKind = iota
	lockName
	lockLaunch
	lockTab
	// lockKindCount bounds the per-kind arrays below.
	lockKindCount
)

func (k lockKind) String() string {
	switch k {
	case lockState:
		return "state"
	case lockName:
		return "name"
	case lockLaunch:
		return "launch"
	case lockTab:
		return "tab"
	}
	return "unknown"
}

// lockNests lists the pairs of locks the provider holds at once, as
// outer -> inner. It is the transitive closure of the nestings the callers
// take, because lockSection checks each pair it finds held directly. A pair
// absent here is a lock-order bug and lockSection refuses it with ErrLockOrder.
//
// The four kinds do NOT form a single total order. These two nestings have no
// rank that fits both:
//
//   - name holds state: start seeds its provisional binding under the name
//     lock (launch_pane.go:104); retire clears identity under the name lock
//     (retire.go:280 -> state lock at :289); KillObject calls withSameBinding
//     under the name lock (observe.go:431, :488), and withSameBinding takes the
//     state lock (observe.go:556).
//   - state holds launch, never the reverse: start's failed-launch cleanup
//     takes the state lock (launch_pane.go:135) and calls
//     destroyEmptyCitySession, which takes the launch lock (retire.go:346). The
//     state lock is deliberately never held inside a blocking launch
//     (lock_scope_test.go).
//
// A rank would have to put state either always outside name (wrongly allowing
// state -> name to slip inside a launch) or always inside it (wrongly
// rejecting start's name -> state). The table states the real rule.
//
// The remaining pairs are the transitive closure of name -> launch -> tab:
// Start holds the name lock across launchOnce, which holds the launch lock and
// then the tab lock. The direct edges are named; the closure follows from them.
var lockNests = map[[2]lockKind]bool{
	// start: seed (launch_pane.go:104), retire clearIdentity (retire.go:289),
	// KillObject withSameBinding (observe.go:431).
	{lockName, lockState}: true,
	// Start/Stop hold the name lock across launchOnce and destroyEmptyCitySession
	// (retire.go:346).
	{lockName, lockLaunch}: true,
	// closure of name -> launch -> tab: launchOnce holds the tab lock under the
	// name lock Start holds (launch_pane.go:166), and recordOrDiscard retires
	// under it.
	{lockName, lockTab}: true,
	// start's failed-launch cleanup: state then launch (launch_pane.go:135,
	// retire.go:346).
	{lockState, lockLaunch}: true,
	// closure of state -> launch -> tab.
	{lockState, lockTab}: true,
	// launchOnce: launch then tab (launch_pane.go:160, :166).
	{lockLaunch, lockTab}: true,
}

// The pairs the guard refuses are exactly those absent from lockNests. Three
// absences carry intent worth stating:
//
//   - launch -> state: a launch must not take the city-wide lock. This is what
//     keeps a slow launch from stalling every stop; lock_scope_test.go depends
//     on it.
//   - tab -> anything: tab is innermost; nothing nests inside it.
//   - state -> name: the code takes name -> state and never the reverse.

// lockReentrant lists the kinds a section may take while already holding the
// same kind. Only the per-name lock nests: Stop holds one name lock per primary
// of an alias group, one inside the last (lifecycle.go:35). The state, launch
// and tab locks do not nest in the code, so a second section of any of them is
// refused rather than silently doubling a hold.
var lockReentrant = map[lockKind]bool{
	lockName: true,
}

// lockTracking records, per kind, how many sections this provider holds. It
// exists so lockSection can refuse a pair lockNests does not allow. It is a
// count, not a bool, so a reentrant kind unwinds exactly.
type lockTracking struct {
	held [lockKindCount]int
}

// lockOrderEnabled turns the check on. It is false in the built provider: a
// lock-order violation is a bug in this package, not hostile input, and the
// guard must not turn a correct call into a runtime error. The package tests
// set it through armLockOrder.
var lockOrderEnabled bool

// lockSection opens a section of the given kind and returns a function that
// ends it, or an ErrLockOrder naming both kinds when the section may not be
// held under what this provider already holds. Every lock in lock.go opens
// through it, before it takes its flock, so the guard runs whether or not the
// flock is taken (a provider with no state dir still nests sections).
func (p *Provider) lockSection(taking lockKind) (end func(), err error) {
	if !lockOrderEnabled {
		return func() {}, nil
	}
	t := &p.lockTracking
	if t.held[taking] > 0 && !lockReentrant[taking] {
		return nil, fmt.Errorf("%w: %s lock already held", ErrLockOrder, taking)
	}
	for outer := lockState; outer < lockKindCount; outer++ {
		if outer == taking || t.held[outer] == 0 {
			continue
		}
		if !lockNests[[2]lockKind{outer, taking}] {
			return nil, fmt.Errorf("%w: %s lock while holding the %s lock", ErrLockOrder, taking, outer)
		}
	}
	t.held[taking]++
	return func() { t.held[taking]-- }, nil
}

// endOnReturn wraps fn so the section end runs when fn returns, after withFlock
// has released the flock on its own defer.
func (p *Provider) endOnReturn(end func(), fn func() error) func() error {
	return func() error {
		defer end()
		return fn()
	}
}

// releaseBoth returns one release that drops the flock and then ends the
// section, in that order.
func (p *Provider) releaseBoth(release, end func()) func() {
	return func() {
		release()
		end()
	}
}
