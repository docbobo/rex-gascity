package provider

import "os"

// Namespace is the city namespace a Provider works in: where its state lives
// and which Rex session labels it accepts. It holds two ordered lists, both
// canonical first:
//
//   - dirs: the per-city state dirs. The first takes writes and the
//     cross-process lock; reads fall back through the rest, and deletes cover
//     all of them.
//   - labels: the Rex session labels. The first is the label new sessions
//     carry; lookups and binding revalidation accept every one, so a record
//     written under an older namespace still resolves.
//
// This is the one place that states the canonical-first rule. The zero value
// has no dirs (state stays in memory, no lock) and no labels (the provider
// falls back to gc:<city>).
type Namespace struct {
	dirs   []string
	labels []string
}

// NewNamespace builds the Namespace for the ordered city namespaces cities
// (canonical first): each city's state dir and Rex session label. Labels are
// deduplicated. It fails if any state dir cannot be resolved, so a caller never
// silently drops a namespace.
func NewNamespace(cities []string) (Namespace, error) {
	dirs, err := StateDirs(cities)
	if err != nil {
		return Namespace{}, err
	}
	return Namespace{dirs: dirs, labels: SessionLabels(cities)}, nil
}

// NamespaceForCaller resolves the city and namespace for this process from
// its environment. RPP v0 carries no city, so the same city can reach us with
// different incidental GC_CITY_PATH/GC_CITY values: it resolves one canonical
// city and reads across the candidates, so start and every lookup op agree on
// one store.
//
// A pinned caller (GC_REX_CITY set) keeps exactly its pin, so cities stay
// isolated from each other. An unpinned caller (the gc CLI fallback provider
// carries no city env) cannot be told which pin wrote a binding, so the
// sibling city namespaces a pinned controller wrote under are appended after
// the canonical candidates.
func NamespaceForCaller() (city string, ns Namespace, err error) {
	city = CityFromEnv()
	cities := CityCandidates()
	if os.Getenv("GC_REX_CITY") == "" {
		cities = append(cities, SiblingCityNamespaces(cities)...)
	}
	ns, err = NewNamespace(cities)
	return city, ns, err
}

// CanonicalDir returns the state dir that takes writes and the lock, or ""
// when the namespace has no state dir.
func (n Namespace) CanonicalDir() string {
	if len(n.dirs) == 0 {
		return ""
	}
	return n.dirs[0]
}

// Dirs returns the state dirs, canonical first. The caller must not modify it.
func (n Namespace) Dirs() []string { return n.dirs }

// CanonicalLabel returns the label new sessions carry, or "" when the
// namespace has no labels.
func (n Namespace) CanonicalLabel() string {
	if len(n.labels) == 0 {
		return ""
	}
	return n.labels[0]
}

// Labels returns the accepted session labels, canonical first. The caller must
// not modify it.
func (n Namespace) Labels() []string { return n.labels }
