// Package envdiff computes semantic, key-level diffs between env files and
// masks secret values for display.
package envdiff

import "github.com/volodymyrsokha/envbridge/internal/envfile"

type Kind int

const (
	Added Kind = iota
	Removed
	Changed
)

// Change describes one key's difference going from an old file to a new one.
type Change struct {
	Kind Kind
	Key  string
	Old  string
	New  string
}

// Diff reports what changed going from `from` to `to`: removed and changed
// keys in `from`'s order, then keys added by `to` in its order. Comment,
// ordering, and formatting differences are invisible here — the diff is
// purely semantic.
func Diff(from, to *envfile.File) []Change {
	var changes []Change
	for _, key := range from.Keys() {
		oldVal, _ := from.Get(key)
		newVal, ok := to.Get(key)
		switch {
		case !ok:
			changes = append(changes, Change{Kind: Removed, Key: key, Old: oldVal})
		case oldVal != newVal:
			changes = append(changes, Change{Kind: Changed, Key: key, Old: oldVal, New: newVal})
		}
	}
	fromKeys := make(map[string]bool)
	for _, key := range from.Keys() {
		fromKeys[key] = true
	}
	for _, key := range to.Keys() {
		if !fromKeys[key] {
			newVal, _ := to.Get(key)
			changes = append(changes, Change{Kind: Added, Key: key, New: newVal})
		}
	}
	return changes
}
