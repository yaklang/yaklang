//go:build ignore

// Build from the worktree root. RUN must be under /mnt/data/scan-outputs/.
// export GOCACHE=/mnt/data/cache/go-build GOMODCACHE=/mnt/data/cache/go-mod
// export GOTMPDIR=/mnt/data/cache/go-tmp TMPDIR=/mnt/data/cache/go-tmp
// RUN=/mnt/data/scan-outputs/rust-go-binaries; mkdir -p "$RUN"
// CGO_ENABLED=0 go build -gcflags='-N -l' -o "$RUN/go_hello" common/yak/reify/test/examples/go_hello.go
// CGO_ENABLED=0 go build -gcflags='-N -l' -ldflags='-s -w' -o "$RUN/go_hello_stripped" common/yak/reify/test/examples/go_hello.go
package main

// Greeting is a package-level string header: pointer and length. The strings
// command reads it at the main.Greeting symbol.
var Greeting = "hello from reify"

//go:noinline
func Add(a, b int) int { return a + b }

func main() { println(Greeting, Add(40, 2)) }
