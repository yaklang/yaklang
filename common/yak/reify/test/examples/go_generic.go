//go:build ignore

// A generic fixture: a generic method, a generic function and a closure inside
// a generic function. The compiler writes go.shape. into each instantiation.
package main

type Box[T any] struct{ v T }

//go:noinline
func (b *Box[T]) Get() T { return b.v }

//go:noinline
func Map[T, U any](in []T, f func(T) U) []U {
	out := make([]U, 0, len(in))
	for _, v := range in {
		out = append(out, f(v))
	}
	return out
}

//go:noinline
func Wrap[T any](v T) func() T {
	return func() T { return v }
}

func main() {
	b := &Box[int]{v: 41}
	xs := Map([]int{1, 2}, func(v int) string { return string(rune('a' + v)) })
	f := Wrap(7)
	println(b.Get(), xs[0], f())
}
