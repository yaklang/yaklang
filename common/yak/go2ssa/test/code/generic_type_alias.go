//go:build ignore
// +build ignore

// Go 1.23+ generic type alias syntax.
package main

type G[P any] struct {
	v P
}

type A[P any] = G[P]

type AliasRHS[P any, Q ~int] struct {
	p P
	q Q
}

type PartialAlias[P any] = AliasRHS[P, int]

func genericTypeAlias() {
	var x A[int]
	_ = x
}
