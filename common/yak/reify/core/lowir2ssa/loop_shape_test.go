package lowir2ssa

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
)

func TestLoopShape5496(t *testing.T) {
	ir, err := lowirtext.Parse(`target: darwin/arm64 (64-bit, little-endian)
frame: sp=sp fp=x29 ret=x30 flags=nzcv saved=x19,x20,x21,x22,x23,x24,x25,x26,x27,x28,x29 growth=-1
registers: naming=aarch64 width=64 flags=nzcv flags_width=64 args=x0,x1,x2,x3,x4,x5,x6,x7 return=x0
function accumulate @ 0x100
block b1 @ 0x100
  0x100  v1:i32 = constant 0x0
  0x100  write_register %w9, v1 [write_register]
  0x100  v2:i32 = constant 0x0
  0x100  write_register %w10, v2 [write_register]
  0x100  jump -> b2
block b2 @ 0x104
  0x104  v3:i32 = read_register %w10 [read_register]
  0x104  v4:i32 = constant 0x5
  0x104  v5:i32 = sub v3, v4
  0x104  v6:i4 = intrinsic @flags.sub, v3, v4, v5 [write_register]
  0x104  write_register %nzcv, v6 [write_register]
  0x104  v7:i4 = read_register %nzcv [read_register]
  0x104  v8:i1 = intrinsic @cond.ge, v7 [read_register]
  0x104  branch v8 -> b7, b8
block b7 @ 0x108
  0x108  v9:i32 = constant 0x1
  0x108  write_register %w8, v9 [write_register]
  0x108  jump -> b9
block b8 @ 0x10c
  0x10c  v10:i32 = constant 0x0
  0x10c  write_register %w8, v10 [write_register]
  0x10c  jump -> b9
block b9 @ 0x110
  0x110  v11:i32 = read_register %w8 [read_register]
  0x110  branch v11 -> b6, b3
block b6 @ 0x114
  0x114  v12:i32 = read_register %w9 [read_register]
  0x114  return v12
block b3 @ 0x118
  0x118  v13:i32 = read_register %w9 [read_register]
  0x118  v14:i32 = read_register %w10 [read_register]
  0x118  v15:i32 = add v13, v14
  0x118  write_register %w9, v15 [write_register]
  0x118  v16:i32 = constant 0x1
  0x118  v17:i32 = add v14, v16
  0x118  write_register %w10, v17 [write_register]
  0x118  jump -> b2`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Convert(context.Background(), ir)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SSAIR:\n%s", Text(result.Program))
	source, err := ssa2source.Render(context.Background(), result.Program)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("source:\n%s", source)
	if !strings.Contains(source, "while (") {
		t.Fatal("no while loop")
	}
}
