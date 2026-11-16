#!/bin/bash
# Build the example programs for every architecture with clang and decompile main.
#
# The binaries are freestanding: the examples only need puts, printf and friends,
# and a small stub runtime defines them, so no cross libc or system image is
# needed. Only clang with lld is required.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
root="$here/.."
work=$(mktemp -d "${TMPDIR:-/tmp}/reify-cross.XXXXXX")
trap 'rm -rf "$work"' EXIT

reify="$work/reify"
go build -o "$reify" "$root/cmd/reify"

cat >"$work/shim.h" <<'HEADER'
typedef unsigned long size_t;
typedef struct _IO_FILE FILE;
int printf(const char *, ...);
int puts(const char *);
int strcmp(const char *, const char *);
char *strncpy(char *, const char *, size_t);
void *malloc(size_t);
void free(void *);
#define NULL ((void*)0)
HEADER

cat >"$work/runtime.c" <<'RUNTIME'
typedef unsigned long size_t;
static void emit(const char *s) { (void)s; }
int puts(const char *s) { emit(s); return 0; }
int printf(const char *fmt, ...) { emit(fmt); return 0; }
int strcmp(const char *a, const char *b) { while (*a && *a == *b) { a++; b++; } return (int)(unsigned char)*a - (int)(unsigned char)*b; }
char *strncpy(char *d, const char *s, size_t n) { size_t i = 0; for (; i < n && s[i]; i++) d[i] = s[i]; for (; i < n; i++) d[i] = 0; return d; }
void *malloc(size_t n) { static char heap[65536]; static size_t used; void *p = heap + used; used += (n + 15) & ~(size_t)15; return p; }
void free(void *p) { (void)p; }
void *memcpy(void *d, const void *s, size_t n) { char *a = d; const char *b = s; for (size_t i = 0; i < n; i++) a[i] = b[i]; return d; }
void __main(void) {}
void _start(void) {}
RUNTIME

targets=${TARGETS:-"x86_64-linux-gnu i386-linux-gnu aarch64-linux-gnu armv7-linux-gnueabihf riscv64-linux-gnu riscv32-linux-gnu mips-linux-gnu mipsel-linux-gnu mips64-linux-gnuabi64 mips64el-linux-gnuabi64 powerpc-linux-gnu powerpc64-linux-gnu powerpc64le-linux-gnu s390x-linux-gnu loongarch64-linux-gnu x86_64-w64-windows-gnu"}

for triple in $targets; do
  for source in "$root"/test/examples/*.c; do
    name=$(basename "$source" .c)
    sed 's|#include <stdio.h>|#include "shim.h"|; s|#include <stdlib.h>||; s|#include <string.h>||' \
      "$source" >"$work/$name.c"
    case "$triple" in
    *windows*) extra="-Wl,-e,_start -Wl,--subsystem,console -Wl,--export-all-symbols"; output="$work/$name.$triple.exe" ;;
    *)
      # A fixed load address keeps an address in the recovered source comparable
      # with the disassembly. The position-independent build below covers the
      # layout the compiler chooses by default.
      extra="-fno-pic -no-pie -Wl,-e,_start"
      output="$work/$name.$triple"
      ;;
    esac
    # shellcheck disable=SC2086
    clang -target "$triple" -O0 -ffreestanding -fno-stack-protector -nostdlib \
      -fuse-ld=lld $extra -I"$work" -o "$output" "$work/$name.c" "$work/runtime.c" 2>"$work/$name.$triple.log" || {
      echo "== $name $triple: build failed"
      continue
    }
    echo "== $name $triple"
    "$reify" lift --mode source --func main "$output" 2>&1 | sed -n '/^\/\/ boundary/,$p' || true
    case "$triple" in
    *windows*) continue ;;
    esac
    # A position-independent image reaches every string through a table the
    # dynamic linker fills, so it exercises the relocation and program-counter
    # handling that a fixed load address does not.
    clang -target "$triple" -O0 -ffreestanding -fno-stack-protector -nostdlib \
      -fuse-ld=lld -Wl,-e,_start -I"$work" -o "$output.pie" "$work/$name.c" "$work/runtime.c" 2>"$work/$name.$triple.pie.log" || {
      echo "== $name $triple pie: build failed"
      continue
    }
    echo "== $name $triple pie"
    "$reify" lift --mode source --func main "$output.pie" 2>&1 | sed -n '/^\/\/ boundary/,$p' || true
  done
done
