//go:build ignore
// +build ignore

// Comments inside if/simple statements must not terminate the statement early.
package main

func ifConditionComments(a, b string) {
	// if init; cond — comment between init and condition
	if x := 1; x /* comment before compare */ == 1 {
		_ = x
	}

	// comment inside condition expression (not assignment)
	if a == "a" && b /* comment before string literal */ == "b" {
	}

	// comment between logical operands
	if true || /* comment between || operands */ false {
	}

	_ = a
}
