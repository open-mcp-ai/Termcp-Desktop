// Package ns names the resource namespaces termcp keeps off its ordinary
// surfaces.
//
// A namespace is carried on the resource itself (api.Session.Namespace) rather
// than held in a side table, for one reason: a session outlives the process that
// created it. The manager restores the manifests written by the previous run, so
// a tag that lived only in memory would come back untagged and a leftover
// extension session would reappear as an ordinary archived session. The value is
// what the external mapping is keyed on — resource kind + id resolve through the
// record, and this package owns the vocabulary that turns "who owns it" into the
// string everything else filters on.
package ns

import (
	"errors"
	"fmt"
	"strings"
)

// Extensions is the root of every namespace the extension layer owns. A session
// an extension is served by is not a session the user asked for: it is termcp's
// own infrastructure, kept warm on the extension's behalf, and it belongs to the
// extension column rather than on the session and archive cards.
const Extensions = "ext"

// ForExtensions renders the namespace of an extension group's session. The
// suffix is the host the group dials, so the namespace says not only "hidden"
// but which pile of extensions a session belongs to — which is what makes an
// unexpected one identifiable in a log.
//
// Two groups may share a suffix (the same host reached with different
// capabilities are two sessions), so this is a namespace, not an identity: the
// group key is what decides which extensions share a session.
func ForExtensions(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return Extensions
	}
	return Extensions + ":" + target
}

// IsExtensions reports whether a namespace belongs to the extension layer. It is
// the question the reconciler asks about a session it finds on disk: Hidden says
// "is this off the ordinary surfaces", which a second subsystem's namespaces also
// answer, so sweeping on Hidden would let the extension reconciler delete another
// subsystem's restored sessions.
func IsExtensions(namespace string) bool {
	return namespace == Extensions || strings.HasPrefix(namespace, Extensions+":")
}

// Hidden reports whether a namespace keeps its resources off the ordinary
// session surfaces: the session and archive cards, the node cards' session
// counts, and the operator's session_list.
//
// Only infrastructure is hidden. A caller's namespace (see Callers) is not: those
// are ordinary sessions an agent asked for, and the operator is the person who
// owns the instance — hiding their own agent's work from them would be a
// different feature (multi-tenancy) than the one this is (two agents not
// colliding). The tag still rides on the record, so the list can say whose a
// session is.
//
// An empty namespace is the default and is never hidden, so every session that
// existed before namespaces did keeps being listed exactly as it was.
func Hidden(namespace string) bool {
	return IsExtensions(namespace)
}

// Callers is the root of every namespace an MCP caller may claim for itself. A
// caller that names one gets a private slice of the instance: the sessions it
// starts carry its namespace, it lists only those, and every call naming
// somebody else's session is refused rather than merely omitted from a listing.
//
// The root is a prefix and not a bare name on purpose. A caller picks a word
// ("alice", "build-agent-2"), and without a prefix that word would be free to
// collide with a namespace termcp itself owns — an agent that asks for "ext"
// would be handed the extension layer's sessions, which is the opposite of
// isolation. Prefixing makes the two spaces disjoint by construction, so the
// only rule left to enforce is the shape of the name.
const Callers = "ns"

// ForCaller renders the namespace a caller's own resources carry.
func ForCaller(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return Callers + ":" + name
}

// CallerOf reverses ForCaller: the caller name a namespace belongs to, if it is
// one at all. Infrastructure namespaces (the extension layer's) report false, so
// a caller can never be mistaken for one.
func CallerOf(namespace string) (string, bool) {
	name, ok := strings.CutPrefix(namespace, Callers+":")
	if !ok || name == "" {
		return "", false
	}
	return name, true
}

// ValidCallerName reports whether a caller may claim name for its own. The
// charset is deliberately narrow — letters, digits, dash, underscore, dot, up to
// 64 — because the name travels in an HTTP header and lands in a namespace
// string that is stored in a manifest.
func ValidCallerName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil // an absent namespace is the default, not an error
	}
	if len(name) > 64 {
		return errors.New("namespace is longer than 64 characters")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("namespace %q may only contain letters, digits, dash, underscore and dot", name)
		}
	}
	return nil
}

// Reachable reports whether a caller in callerNS may act on a resource carrying
// namespace. It is the by-id half of the rule — the question the guard asks —
// and it is deliberately more permissive than Listable for the operator case.
//
//	caller ""      → the operator: anything, including infrastructure it already
//	                 holds a handle for (an extension session is not listed but
//	                 stays addressable, which is the contract it documents)
//	caller "ns:x"  → exactly the resources tagged ns:x
//
// The empty caller is the operator, not a tenant: the browser, the stdio bridge,
// and every agent that claimed nothing are the person who owns the instance, and
// this feature is about two agents colliding with each other rather than about
// hiding agents from their owner.
func Reachable(namespace, callerNS string) bool {
	callerNS = strings.TrimSpace(callerNS)
	if callerNS == "" {
		return true
	}
	return strings.TrimSpace(namespace) == callerNS
}

// Listable reports whether a resource carrying namespace shows up in a listing
// for a caller in callerNS. Same rule as Reachable, with one addition: the
// operator's listing still hides infrastructure, because a session termcp keeps
// warm on a subsystem's behalf is not a session the user asked for and belongs in
// that subsystem's own column rather than on the session cards.
func Listable(namespace, callerNS string) bool {
	callerNS = strings.TrimSpace(callerNS)
	if callerNS == "" {
		return !Hidden(namespace)
	}
	return strings.TrimSpace(namespace) == callerNS
}

// ForbiddenForeign renders the refusal a namespaced caller gets when it names a
// resource outside its own namespace. The text names the caller's namespace and
// nothing about the resource: an id the caller was handed once must not become a
// way to learn what other namespaces hold, and "not yours" is the whole answer
// either way — the resource may not exist at all.
func ForbiddenForeign(callerNS string) string {
	name, ok := CallerOf(callerNS)
	if !ok {
		name = callerNS
	}
	return "this call names a session outside namespace " + name + ": it belongs to another caller or to termcp's own infrastructure (an extension session, for instance). Start your own with session_start — it is created in your namespace — and address only ids your calls return."
}
