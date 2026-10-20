package ssa

import (
	"fmt"
	"testing"
)

// AddUser deduplicates by id. Hot values (e.g. the object of a struct default
// construction) accumulate one user per field, so the historic linear
// slices.Index scan made building a K-field struct cost O(K^2) in the user
// count. The userSet map keeps it O(1) once the list grows past
// addUserSetThreshold while small lists keep the zero-map fast path.
func BenchmarkValueAddUserDedup(b *testing.B) {
	for _, n := range []int{8, 64, 1024, 16384} {
		b.Run(fmt.Sprintf("users=%d", n), func(b *testing.B) {
			target := NewValue()
			target.SetId(1)
			users := make([]User, n)
			for i := range users {
				u := NewValue()
				u.SetId(int64(i + 2))
				users[i] = &Undefined{anValue: u}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// two passes: n fresh adds, then n duplicate adds that must be
				// rejected without growing the list
				for _, u := range users {
					target.AddUser(u)
				}
				for _, u := range users {
					target.AddUser(u)
				}
			}
		})
	}
}