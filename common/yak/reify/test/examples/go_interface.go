//go:build ignore

// Build from the worktree root. Output must stay under /mnt/data.
// export GOCACHE=/mnt/data/cache/go-build GOMODCACHE=/mnt/data/cache/go-mod
// export GOTMPDIR=/mnt/data/cache/go-tmp TMPDIR=/mnt/data/cache/go-tmp
// RUN=/mnt/data/scan-outputs/rust-go-binaries; mkdir -p "$RUN"
// CGO_ENABLED=0 go build -gcflags='-N -l' -o "$RUN/go_interface" common/yak/reify/test/examples/go_interface.go
//
// The call through Shape.Area is an interface call: the concrete target is
// loaded from an itab at run time, so no direct call edge exists to follow.
package main

type Shape interface{ Area() int }

type square struct{ side int }

//go:noinline
func (s square) Area() int { return s.side * s.side }

//go:noinline
func total(shapes []Shape) int {
	sum := 0
	for _, shape := range shapes {
		sum += shape.Area()
	}
	return sum
}

func main() {
	println(total([]Shape{square{side: 6}}))
}
