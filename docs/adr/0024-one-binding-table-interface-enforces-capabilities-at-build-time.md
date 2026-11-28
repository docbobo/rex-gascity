# 0024. one binding table interface enforces capabilities at build time

- Status: Proposed
- Date: 2026-11-28

## Context

Three sidecar record kinds (bindings, activity fingerprints, metadata) each had
their own type parameters over one `Store` seam in
`internal/provider/stores.go`, and the capabilities beyond `Read`/`Write`/`Drop`
were optional:

- `Lister[V]` (`internal/provider/stores.go:46-49`) was asserted at run time by
  `Provider.ListRunning` (`internal/provider/provider.go:419`) and by
  `bindingAdapters` in `internal/provider/store_contract_test.go`, which failed
  the call with "binding store cannot list" when the assertion missed. A store
  without `List` still compiled, so the rejection happened per op.
- `clearer` (`internal/provider/stores.go:52-55`) was asserted by
  `Provider.metaClear` (`internal/provider/meta.go:43-48`), which returned
  `nil` when a store did not implement it: the meta sweep silently became a
  no-op, and retired incarnations kept their `GC_SESSION_ID` keys.
- The binding records had two independently typed paths: `Provider.bindings`
  (`internal/provider/provider.go:168`) and `Binder.store`
  (`internal/provider/binding.go:132`). A meta store and a binding store could
  drift apart, and every alias lookup re-implemented the primary-then-alias scan
  in `Binder.lookup` (`internal/provider/binding.go:192-213`).
- Store reads swallowed errors. `Provider.Binding` (`internal/provider/provider.go:352-355`)
  discarded the `Read` error, `Binder.lookup` had no error at all, and
  `StartNew`'s `fresh_only` branch (`internal/provider/provider.go:394`) read an
  unreadable store as "the name is free". `FileStore.List` skipped corrupt
  record files entirely, so an unreadable `bindings/*.json` read as absence.

The consequence was a whole class of bugs the compiler could not catch: a
capability missing at run time, a sweep that quietly did nothing, and a read
failure that looked like a free name.

## Decision

We define one `BindingTable` interface in `internal/provider/stores.go` and hold
one value of it in both the `Provider` and the `Binder`:

```go
type bindingRecords interface {
	Store[SessionName, Binding]
	List() ([]Binding, error)
}

type MetaStore interface {
	Store[metaKey, string]
	Clear(name SessionName) error
}

type BindingTable interface {
	Store[SessionName, Binding]
	List() ([]Binding, error)
	Lookup(name SessionName) ([]Binding, error)
	ClearMeta(name SessionName) error
}
```

`bindingTable` is the only implementation; it owns the records and the meta
store, so `Provider.metaClear` and the `Binder` alias scan both run inside it.
`New` builds the table after the options run and hands the same value to
`NewBinderWithLabels`. `Lister` and `clearer` are deleted: enumeration and the
meta sweep are required capabilities, and a type without them does not compile
as a store.

Every read returns an error: `Binder.lookup`, `Provider.Binding` and
`metaName` propagate it; `Forget` returns it; `fresh_only` returns it before
considering a collision. `Store`'s comment states the rule: absence is
`(zero, false, nil)`, an unreadable store is an error.

## Consequences

- A missing capability is a compile error, not a per-op failure or a silent
  no-op. `TestBindingTableContract` exercises `Read`, `Write`, `Drop`, `List`,
  alias `Lookup`, duplicates and `ClearMeta` over the memory, file and composite
  adapters.
- A store read failure reaches the caller from `StartNew`, `Start`,
  `Binding`, `Forget`, `ListRunning`, `Inventory` and the retirement sweep. The
  binding is kept when the sweep fails, so a retry has a name to sweep.
- `FileStore.List` now reports a corrupt record as an error naming its path while
  still returning the healthy records; `compositeBindingStore.List` joins every
  namespace's error.
- A caller that wants absence semantics must handle the error, not ignore it.
  The deferred failed-launch cleanup (`internal/provider/launch_pane.go:91-96`)
  and `dropStaleIfUnchanged` discard the error explicitly because they run on an
  already-failing best-effort path; both say so in a comment.
- `TestFileStoreOnDiskBytes` still pins the on-disk shape, so the table change
  did not move record bytes.

## Alternatives rejected

- **Keep optional capability interfaces with a run-time assertion** (the
  previous `Lister`/`clearer` shape). Rejected: a store that cannot list still
  compiles, and `metaClear` returned `nil` when the assertion missed, so the
  sweep failed open. The failure surfaces per op, long after the store was
  installed, and the compiler has no say.
- **Fail at op time with a capability error** instead of at build time.
  Rejected: `ListRunning` and the retirement sweep then need an error path for a
  configuration mistake that a type check prevents. It also keeps the
  fail-open `metaClear` possible for any adapter that forgets `Clear`.
- **Keep two access paths** (`Provider.bindings` and `binder.store`) with the
  alias scan in the `Binder`. Rejected: the meta sweep and the binding records
  then have two owners, a meta store can diverge from the binding store it is
  paired with, and every alias-lookup caller re-implements the ordering.
- **Keep `List` returning no error and skip corrupt files.** Rejected: a
  corrupt record is then indistinguishable from an absent one, so `Inventory`
  and `ListRunning` report the session as gone and a `fresh_only` start can
  reuse a held name.
