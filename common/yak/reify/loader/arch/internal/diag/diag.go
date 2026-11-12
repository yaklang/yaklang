// Package diag holds the diagnostics bookkeeping shared by the architecture
// parsers.
package diag

import "fmt"

// Undecodable aggregates contiguous undecodable words into one diagnostic. A
// code region often contains a literal pool or a jump table; reporting every
// word would bury the real findings under hundreds of identical messages.
type Undecodable struct {
	start uint64
	count int
	size  int
}

// Add records one undecodable word at address. A non-contiguous address starts
// a new run.
func (u *Undecodable) Add(address uint64, size int) {
	if u.count > 0 && address == u.start+uint64(u.count*u.size) {
		u.count++
		return
	}
	u.count = 1
	u.start = address
	u.size = size
}

// Flush emits the pending run as one diagnostic, then resets the run.
func (u *Undecodable) Flush(add func(code, message string)) {
	if u.count == 0 {
		return
	}
	unit := "words"
	if u.count == 1 {
		unit = "word"
	}
	add("decode-error", fmt.Sprintf(
		"%d %s at 0x%x are not instructions; the bytes are data inside the code region",
		u.count, unit, u.start))
	u.count = 0
}
