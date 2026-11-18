# Language examples

These sources are the fixtures of the language layer. Every one is compiled by a
test or by the commands below; no binary is stored in the repository.

The language layer sits above the container loader. The container is still ELF,
Mach-O or PE; the layer reads compiler metadata inside it, recovers function
symbols and makes their names readable. See
[../docs/language.md](../../docs/language.md) for the contracts and the limits.

Set `RUN` to an absolute directory under `/mnt/data/scan-outputs/` before
compiling, and export the shared Go caches from the workspace `AGENTS.md`.

## Go

```sh
export GOCACHE=/mnt/data/cache/go-build GOMODCACHE=/mnt/data/cache/go-mod
export GOTMPDIR=/mnt/data/cache/go-tmp TMPDIR=/mnt/data/cache/go-tmp
RUN=/mnt/data/scan-outputs/rust-go-binaries; mkdir -p "$RUN"

# Symbols retained.
CGO_ENABLED=0 go build -gcflags='-N -l' -o "$RUN/go_hello" go_hello.go
# Symbols stripped; .gopclntab and .go.buildinfo remain.
CGO_ENABLED=0 go build -gcflags='-N -l' -ldflags='-s -w' -o "$RUN/go_hello_stripped" go_hello.go
CGO_ENABLED=0 go build -gcflags='-N -l' -o "$RUN/go_interface" go_interface.go

reify inspect "$RUN/go_hello_stripped"
reify lift --mode all --func main.Add "$RUN/go_hello_stripped"
```

`go_hello.go` uses `//go:noinline` so `main.Add` keeps its own body.
`go_interface.go` contains an interface call whose target is an itab entry, which
this layer does not resolve.

## Rust

```sh
RUN=/mnt/data/scan-outputs/rust-go-binaries; mkdir -p "$RUN"
export TMPDIR=/mnt/data/cache/go-tmp

rustc --edition=2021 -C opt-level=0 -C overflow-checks=off \
  -C symbol-mangling-version=v0 -o "$RUN/rust_hello" rust_hello.rs
rustc --edition=2021 -C opt-level=0 -C overflow-checks=off \
  -C symbol-mangling-version=v0 -o "$RUN/rust_trait" rust_trait.rs

reify inspect "$RUN/rust_hello"
reify lift --mode all --func rust_hello::add "$RUN/rust_hello"
reify lift --mode all --func rust_hello::add _RNvCs4FrXkKnA8Dq_10rust_hello3add  # same body by encoded name
```

Rust names live in the symbol table. `-C strip=symbols` removes them and this
layer cannot recover Rust function names from a stripped Rust image.
`rust_trait.rs` contains a trait-object call through a vtable, which this layer
does not resolve.