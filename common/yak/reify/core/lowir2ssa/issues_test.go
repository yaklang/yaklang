package lowir2ssa

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
)

// convertLowIR parses and converts one LowIR text program, returning the SSAIR
// and pseudo source the two renderers produce.
func convertLowIR(t *testing.T, text string) (string, string) {
	t.Helper()
	ir, err := lowirtext.Parse(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	source, err := ssa2source.Render(context.Background(), result.Program)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return Text(result.Program), source
}

func TestSignedLessThanFoldsSigned(t *testing.T) {
	// Issue 5497: a 32-bit 0xffffffff is -1 for slt and 4294967295 for ult.
	const header = `target: linux/amd64 (64-bit, little-endian)
registers: naming=x86 width=64 sp=rsp return=rax args=rdi,rsi,rdx,rcx,r8,r9
function f @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i32 = constant 0xffffffff
  0x1001  v2:i32 = constant 0x1
`
	for _, testCase := range []struct {
		op   string
		want string
	}{
		{"slt", "return true"},
		{"ult", "return false"},
	} {
		_, source := convertLowIR(t, header+"  0x1002  v3:i1 = "+testCase.op+" v1, v2\n  0x1003  return v3\n")
		if !strings.Contains(source, testCase.want) {
			t.Fatalf("%s source = %q, want %q", testCase.op, source, testCase.want)
		}
	}
}

func TestSignedDivisionAndShiftKeepTheirSign(t *testing.T) {
	// Issue 5501: sdiv/ashr must fold with int64 arithmetic, udiv/lshr without.
	const header = `target: linux/amd64 (64-bit, little-endian)
registers: naming=x86 width=64 sp=rsp return=rax args=rdi,rsi,rdx,rcx,r8,r9
function f @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i64 = constant 0xfffffffffffffff8
  0x1001  v2:i64 = constant 0x2
`
	for _, testCase := range []struct {
		op   string
		want string
	}{
		{"sdiv", "0xfffffffffffffffc"},
		{"udiv", "0x7ffffffffffffffc"},
		{"ashr", "0xfffffffffffffffe"},
		{"lshr", "0x3ffffffffffffffe"},
	} {
		_, source := convertLowIR(t, header+"  0x1002  v3:i64 = "+testCase.op+" v1, v2\n  0x1003  return v3\n")
		if !strings.Contains(source, testCase.want) {
			t.Fatalf("%s source = %q, want %q", testCase.op, source, testCase.want)
		}
	}
}

func TestSignExtensionKeepsTheHighBits(t *testing.T) {
	// Issue 5500: extending 0xffffffff from i32 to i64 must produce all ones,
	// while the zero extension of the same bits stays 0x00000000ffffffff.
	const header = `target: linux/amd64 (64-bit, little-endian)
registers: naming=x86 width=64 sp=rsp return=rax args=rdi,rsi,rdx,rcx,r8,r9
function f @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i32 = constant 0xffffffff
`
	for _, testCase := range []struct {
		op   string
		want string
	}{
		{"sext", "0xffffffffffffffff"},
		{"zext", "0xffffffff"},
	} {
		_, source := convertLowIR(t, header+"  0x1001  v2:i64 = "+testCase.op+" v1\n  0x1002  return v2\n")
		if !strings.Contains(source, testCase.want) {
			t.Fatalf("%s source = %q, want %q", testCase.op, source, testCase.want)
		}

	}
}

// conditionalSource builds one function that compares two arguments through a
// flags value and returns a constant from each arm.
func conditionalSource(t *testing.T, condition string) string {
	t.Helper()
	_, source := convertLowIR(t, `target: darwin/arm64 (64-bit, little-endian)
frame: sp=sp fp=x29 ret=x30 flags=nzcv growth=-1
registers: naming=aarch64 width=64 flags=nzcv flags_width=64 args=x0,x1 return=x0
function compare @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i64 = read_register %x0
  0x1001  v2:i64 = read_register %x1
  0x1002  v3:i4 = intrinsic @flags.cmp, v1, v2 [write_register]
  0x1002  write_register %nzcv, v3 [write_register]
  0x1002  v4:i4 = read_register %nzcv
  0x1002  v5:i1 = intrinsic @cond.`+condition+`, v4
  0x1002  branch v5 -> b2, b3
block b2 @ 0x1004
  0x1004  v6:i32 = constant 0x1
  0x1004  return v6
block b3 @ 0x1008
  0x1008  v7:i32 = constant 0x0
  0x1008  return v7
`)
	return source
}

func TestConditionCodeKeepsComparisonSignedness(t *testing.T) {
	// Issue 5497, part 2: lt is signed and cc (carry clear) is unsigned. The two
	// functions must not collapse to the same comparison.
	signed := conditionalSource(t, "lt")
	unsigned := conditionalSource(t, "cc")
	if !strings.Contains(signed, "(int64_t)arg0 < (int64_t)arg1") {
		t.Fatalf("signed condition = %q", signed)
	}
	if strings.Contains(unsigned, "(int64_t)") {
		t.Fatalf("unsigned condition gained a sign: %q", unsigned)
	}
	if !strings.Contains(unsigned, "arg0 < arg1") {
		t.Fatalf("unsigned condition = %q", unsigned)
	}
}

func TestPointerStoreIsVisibleInSource(t *testing.T) {
	// Issue 5498: a store through a pointer argument is the only effect of the
	// function. The pseudo source has to print it instead of dropping an unused
	// memory variable.
	_, source := convertLowIR(t, `target: darwin/arm64 (64-bit, little-endian)
frame: sp=sp fp=x29 ret=x30 flags=nzcv saved=x19,x20,x21,x22,x23,x24,x25,x26,x27,x28,x29 growth=-1
registers: naming=aarch64 width=64 flags=nzcv flags_width=64 args=x0,x1,x2,x3,x4,x5,x6,x7 return=x0
function write7 @ 0x100
block b1 @ 0x100
  0x100  v1:i32 = constant 0x7
  0x100  write_register %w8, v1 [write_register]
  0x104  v2:i32 = read_register %w8 [read_register]
  0x104  v3:i64 = read_register %x0 [read_register]
  0x104  v4:ptr64 = copy v3
  0x104  store v4, v2 [write_memory]
  0x108  return %x0
`)
	if !strings.Contains(source, "*(uint32_t *)((void *)arg0) = 0x7;") {
		t.Fatalf("store missing from source:\n%s", source)
	}
}

func TestAliasingStoreKeepsTheReload(t *testing.T) {
	// Issue 5499: after a store through arg1, the value cached for arg0 may be
	// stale. The function must return a reload, not the first constant.
	_, source := convertLowIR(t, `target: darwin/arm64 (64-bit, little-endian)
frame: sp=sp fp=x29 ret=x30 flags=nzcv saved=x19,x20,x21,x22,x23,x24,x25,x26,x27,x28,x29 growth=-1
registers: naming=aarch64 width=64 flags=nzcv flags_width=64 args=x0,x1,x2,x3,x4,x5,x6,x7 return=x0
function alias @ 0x100
block b1 @ 0x100
  0x100  v1:i32 = constant 0x7
  0x100  write_register %w8, v1 [write_register]
  0x104  v2:i32 = constant 0xb
  0x104  write_register %w9, v2 [write_register]
  0x108  v3:i32 = read_register %w8 [read_register]
  0x108  v4:i64 = read_register %x0 [read_register]
  0x108  v5:ptr64 = copy v4
  0x108  store v5, v3 [write_memory]
  0x10c  v6:i32 = read_register %w9 [read_register]
  0x10c  v7:i64 = read_register %x1 [read_register]
  0x10c  v8:ptr64 = copy v7
  0x10c  store v8, v6 [write_memory]
  0x110  v9:i64 = read_register %x0 [read_register]
  0x110  v10:ptr64 = copy v9
  0x110  v11:i32 = load v10 [read_memory]
  0x110  write_register %w0, v11 [write_register]
  0x114  return
`)
	if !strings.Contains(source, "return") || !strings.Contains(source, "*(uint32_t *)((void *)arg0)") {
		t.Fatalf("reload missing from source:\n%s", source)
	}
	if strings.Contains(source, "return 0x7;") {
		t.Fatalf("stale constant survived:\n%s", source)
	}
}

func TestBlockStopsAfterTerminator(t *testing.T) {
	// Issue 5507: a malformed block can carry a partial register write after its
	// terminator. Converting it would emit SSA on a finished block, where
	// EmitBinOp returns nil, and the partial-write path dereferenced that nil.
	ssair, source := convertLowIR(t, `target: linux/amd64 (64-bit, little-endian)
registers: naming=x86 width=64 flags=rflags flags_width=64 sp=rsp return=rax args=rdi,rsi,rdx,rcx,r8,r9
function trailing @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i64 = constant 0x1
  0x1000  return v1
  0x1001  v2:i8 = constant 0x2
  0x1001  write_register %ah, v2 [write_register]
`)
	if !strings.Contains(ssair, "ret") {
		t.Fatalf("return missing:\n%s", ssair)
	}
	if strings.Contains(ssair, "0x2") && strings.Contains(ssair, "and-not") {
		t.Fatalf("trailing instruction was converted:\n%s", ssair)
	}
	if strings.Contains(source, "v2") {
		t.Fatalf("trailing instruction reached the source:\n%s", source)
	}
}
