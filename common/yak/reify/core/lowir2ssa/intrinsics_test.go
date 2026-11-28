package lowir2ssa

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
)

func convertAndRender(t *testing.T, irText string) (string, string) {
	t.Helper()
	ir, err := lowirtext.Parse(irText)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	ssair := Text(res.Program)
	src, err := ssa2source.Render(context.Background(), res.Program)
	if err != nil {
		t.Fatal(err)
	}
	return ssair, src
}

const x86Header = `target: linux/amd64 (64-bit, little-endian)
frame: sp=rsp fp=rbp flags=rflags saved=rbp,rbx,r12,r13,r14,r15 growth=-1
registers: naming=x86 width=64 flags=rflags flags_width=64 args=rdi,rsi,rdx,rcx,r8,r9 return=rax
`

const arm64Header = `target: linux/arm64 (64-bit, little-endian)
frame: sp=sp fp=x29 link=x30 flags=nzcv saved=x19,x20,x21,x22,x23,x24,x25,x26,x27,x28,x29,x30 growth=-1
registers: naming=aarch64 width=64 flags=nzcv flags_width=32 args=x0,x1,x2,x3,x4,x5,x6,x7 return=x0
`

func TestLowerX86RotateIntrinsics(t *testing.T) {
	irText := x86Header + `function rot_test @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i32 = read_register %edi
  0x1001  v2:i32 = read_register %esi
  0x1002  v3:i32 = intrinsic @x86.rol, v1, v2
  0x1003  v4:i32 = constant 0x12345678
  0x1004  v5:i32 = constant 0x8
  0x1005  v6:i32 = intrinsic @x86.ror, v4, v5
  0x1006  v7:i32 = add v3, v6
  0x1007  return v7`
	ssair, src := convertAndRender(t, irText)
	if strings.Contains(ssair, "x86.rol") || strings.Contains(src, "x86.rol") {
		t.Fatalf("x86.rol remained opaque:\n%s\n%s", ssair, src)
	}
	if strings.Contains(ssair, "x86.ror") || strings.Contains(src, "x86.ror") {
		t.Fatalf("x86.ror remained opaque:\n%s\n%s", ssair, src)
	}
	// Constant ror(0x12345678, 8) == 0x78123456.
	if !strings.Contains(ssair, "2014458966") && !strings.Contains(src, "78123456") {
		t.Fatalf("constant x86.ror did not fold to 0x78123456:\n%s\n%s", ssair, src)
	}
}

func TestLowerX86DivAndIdivSoundness(t *testing.T) {
	irText := x86Header + `function div_ok @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i32 = constant 0x0
  0x1001  v2:i32 = read_register %eax
  0x1002  v3:i32 = read_register %ecx
  0x1003  v4:i32 = intrinsic @x86.div.rem, v1, v2, v3
  0x1004  return v4
function idiv_cdq @ 0x2000
block b1 @ 0x2000
  0x2000  v1:i32 = read_register %eax
  0x2001  v2:i32 = constant 0x1f
  0x2002  v3:i32 = ashr v1, v2
  0x2003  write_register %edx, v3
  0x2004  v4:i32 = read_register %edx
  0x2005  v5:i32 = read_register %eax
  0x2006  v6:i32 = read_register %ecx
  0x2007  v7:i32 = intrinsic @x86.idiv.rem, v4, v5, v6
  0x2008  return v7
function div_unknown_high @ 0x3000
block b1 @ 0x3000
  0x3000  v1:i32 = read_register %edx
  0x3001  v2:i32 = read_register %eax
  0x3002  v3:i32 = read_register %ecx
  0x3003  v4:i32 = intrinsic @x86.div.quot, v1, v2, v3
  0x3004  return v4`
	ssair, src := convertAndRender(t, irText)
	if strings.Contains(ssair, "x86.div.rem") {
		t.Fatalf("zero-high x86.div.rem stayed opaque:\n%s", ssair)
	}
	if strings.Contains(ssair, "x86.idiv.rem") {
		t.Fatalf("CDQ x86.idiv.rem stayed opaque:\n%s", ssair)
	}
	if !strings.Contains(src, "%") {
		t.Fatalf("div/idiv remainder did not render '%%':\n%s", src)
	}
	// Unknown high must remain opaque for soundness.
	if !strings.Contains(ssair, "x86.div.quot") {
		t.Fatalf("div with unknown high word was unsoundly lowered:\n%s", ssair)
	}
}

func TestLowerARM64BitfieldAndRotate(t *testing.T) {
	irText := arm64Header + `function bf_test @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i32 = read_register %w0
  0x1001  v2:i8 = constant 0x8
  0x1002  v3:i8 = constant 0x8
  0x1003  v4:i32 = intrinsic @arm64.ubfx, v1, v2, v3
  0x1004  v5:i32 = intrinsic @arm64.ubfiz, v4, v2, v3
  0x1005  v6:i32 = intrinsic @arm64.sbfx, v1, v2, v3
  0x1006  v7:i32 = intrinsic @arm64.sbfiz, v6, v2, v3
  0x1007  v8:i32 = constant 0x3
  0x1008  v9:i32 = intrinsic @arm64.ror, v5, v8
  0x1009  v10:i32 = add v7, v9
  0x100a  return v10`
	ssair, src := convertAndRender(t, irText)
	for _, op := range []string{"arm64.ubfx", "arm64.ubfiz", "arm64.sbfx", "arm64.sbfiz", "arm64.ror"} {
		if strings.Contains(ssair, op) || strings.Contains(src, op) {
			t.Fatalf("%s remained opaque:\n%s\n%s", op, ssair, src)
		}
	}
}

func TestIndexedStackStoreNotElidedAndSingleBlockSelfLoop(t *testing.T) {
	irText := x86Header + `function indexed_loop @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i64 = constant 0x0
  0x1001  write_register %rcx, v1
  0x1002  jump -> b2
block b2 @ 0x1010
  0x1010  v2:i64 = read_register %rcx
  0x1011  v3:i64 = read_register %rsp
  0x1012  v4:i64 = add v3, v2
  0x1013  v5:i64 = constant 0x18
  0x1014  v6:i64 = add v4, v5
  0x1015  store v6, v2
  0x1016  v7:i64 = constant 0x1
  0x1017  v8:i64 = add v2, v7
  0x1018  write_register %rcx, v8
  0x1019  v9:i64 = constant 0x100
  0x101a  v10:i1 = s_lt v8, v9
  0x101b  branch v10 -> b2, b3
block b3 @ 0x1030
  0x1030  v11:i64 = read_register %rcx
  0x1031  return v11`
	ssair, src := convertAndRender(t, irText)
	if !strings.Contains(ssair, "store") {
		t.Fatalf("indexed stack store was elided:\n%s", ssair)
	}
	if !strings.Contains(src, "while (") {
		t.Fatalf("single-block self-loop lost while:\n%s", src)
	}
}
