package ssautil

import "testing"

// A released version keeps only the instruction id; GetValue materializes a
// fresh object for it. A later Replace made with the original pointer must
// still reach the binding, otherwise a late import/member fixup is dropped.
func TestRetainedReplacementWinsAfterDetach(t *testing.T) {
	for _, reloadBefore := range []bool{false, true} {
		name := "no-read"
		if reloadBefore {
			name = "read-then-replace"
		}
		t.Run(name, func(t *testing.T) {
			old, replacement := &dummySSAValue{id: 10}, &dummySSAValue{id: 11}
			v := NewVersioned[*dummySSAValue](1, "binding", false, nil).(*Versioned[*dummySSAValue])
			if err := v.Assign(old); err != nil {
				t.Fatal(err)
			}
			v.DetachRetained(old.id, func(id int64) (*dummySSAValue, bool) {
				return &dummySSAValue{id: id}, true
			})
			if reloadBefore {
				if got := v.GetValue().GetId(); got != old.id {
					t.Fatalf("reload id = %d, want %d", got, old.id)
				}
			}
			v.Replace(old, replacement)
			if got := v.GetValue().GetId(); got != replacement.id {
				t.Fatalf("replacement was lost: got id %d, want %d", got, replacement.id)
			}
			if got := v.GetId(); got != replacement.id {
				t.Fatalf("GetId after replace = %d, want %d", got, replacement.id)
			}
		})
	}
}
