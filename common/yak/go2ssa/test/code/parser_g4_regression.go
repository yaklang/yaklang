//go:build ignore
// +build ignore

// Regression fixtures for GoParser.g4: nil identifier, multiline union types,
// generic methods, and multiline var/composite-literal assignments.
package main

import "fmt"

// builtin-style: nil as a variable name (src/builtin/builtin.go pattern).
type Type int

var nil Type

// Multiline union constraint in an interface (cmp / math/rand style).
type Ordered interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64 |
		~string
}

type Option[T any] struct{}

func (Option[T]) Get() T {
	panic("unimplemented")
}

// Generic method: type parameters after the receiver method name.
func (Option[T]) GetOrDefault[U any](u U) (T, U) {
	panic("unimplemented")
}

type Rand struct{}

type intType interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

func (r *Rand) N[Int intType](n Int) Int {
	return n
}

// Multiline type parameter list on a function.
func rangeNum[T int8 | int16 | int32 | int64 | int |
	uint8 | uint16 | uint32 | uint64 | uint |
	uintptr, N int64 | uint64](num N) N {
	return num
}

type row struct {
	id int
}

// var name = newline [...]Type{ ... } (asm6.go style).
var table =
	[...]row{
		{id: 1},
		{id: 2},
	}

func parserG4Regression() {
	_ = nil
	_ = table
	var opt Option[int]
	_ = opt.Get()
	_, _ = opt.GetOrDefault(0)
	r := &Rand{}
	_ = r.N(3)
	_ = rangeNum[int8](1)
	fmt.Println("ok")
}
