package pass_test

import (
	"context"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowir2ssa"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source/pass"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestProgramSummaryMatchesWholeProgramApplyDefault(t *testing.T) {
	src := `target: linux/amd64 (64-bit, little-endian)
frame: sp=rsp fp=rbp flags=rflags saved=rbp,rbx,r12,r13,r14,r15 growth=-1
registers: naming=x86 width=64 flags=rflags flags_width=64 args=rdi,rsi,rdx,rcx,r8,r9 return=rax
function make_ptr @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i64 = constant 0x10
  0x1001  write_register %rdi, v1
  0x1002  call @malloc [call, may_trap, unknown]
  0x1007  v2:i64 = read_register %rax [read_register]
  0x1008  return v2
function wrap_ptr @ 0x2000
block b2 @ 0x2000
  0x2000  call @make_ptr [call, may_trap, unknown]
  0x2005  v3:i64 = read_register %rax [read_register]
  0x2006  v4:i64 = constant 0x4
  0x2007  v5:i64 = add v3, v4
  0x2008  return v5
function caller @ 0x3000
block b3 @ 0x3000
  0x3000  v6:i64 = constant 0x200000
  0x3001  write_register %rdi, v6
  0x3002  call @puts [call, may_trap, unknown]
  0x3007  call @wrap_ptr [call, may_trap, unknown]
  0x300c  v7:i64 = read_register %rax [read_register]
  0x300d  return v7
`
	stringAt := func(addr uint64) (string, bool) {
		if addr == 0x200000 {
			return "hello", true
		}
		return "", false
	}

	irWhole, err := lowirtext.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	convWhole, err := lowir2ssa.Convert(context.Background(), irWhole)
	if err != nil {
		t.Fatal(err)
	}
	pass.ApplyDefault(convWhole.Program, stringAt)
	wantSource, err := ssa2source.Render(context.Background(), convWhole.Program)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate streaming Pass 1 (per-function isolated batches) + Finalize + Pass 2.
	irFull, err := lowirtext.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	summary := pass.NewProgramSummary()
	for _, fnIR := range irFull.Functions {
		singleIR := *irFull
		singleIR.Functions = []core.Function{fnIR}
		convBatch, err := lowir2ssa.Convert(context.Background(), &singleIR)
		if err != nil {
			t.Fatal(err)
		}
		summary.CollectProgram(convBatch.Program)
	}
	summary.Finalize()

	if !summary.Externs["malloc"] || !summary.Externs["puts"] {
		t.Fatalf("expected malloc/puts in Externs, got: %+v", summary.Externs)
	}
	for _, internal := range []string{"make_ptr", "wrap_ptr", "caller"} {
		if summary.Externs[internal] {
			t.Fatalf("internal function %q leaked into Externs: %+v", internal, summary.Externs)
		}
	}

	irPass2, err := lowirtext.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	convPass2, err := lowir2ssa.Convert(context.Background(), irPass2)
	if err != nil {
		t.Fatal(err)
	}
	var funcs []*ssa.Function
	convPass2.Program.Funcs.ForEach(func(_ string, fn *ssa.Function) bool {
		if fn != nil && !fn.IsExtern() {
			funcs = append(funcs, fn)
		}
		return true
	})
	if err := summary.ApplyFunctions(funcs, stringAt); err != nil {
		t.Fatal(err)
	}
	gotSource, err := ssa2source.Render(context.Background(), convPass2.Program)
	if err != nil {
		t.Fatal(err)
	}
	if gotSource != wantSource {
		t.Fatalf("summary pass mismatch:\n--- want ---\n%s\n--- got ---\n%s", wantSource, gotSource)
	}

	// Verify concurrent ApplyFunctions calls against finalized summary are race-free.
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			irConcurrent, err := lowirtext.Parse(src)
			if err != nil {
				t.Error(err)
				return
			}
			convConcurrent, err := lowir2ssa.Convert(context.Background(), irConcurrent)
			if err != nil {
				t.Error(err)
				return
			}
			var cFuncs []*ssa.Function
			convConcurrent.Program.Funcs.ForEach(func(_ string, fn *ssa.Function) bool {
				if fn != nil && !fn.IsExtern() {
					cFuncs = append(cFuncs, fn)
				}
				return true
			})
			if err := summary.ApplyFunctions(cFuncs, stringAt); err != nil {
				t.Error(err)
				return
			}
			if _, err := ssa2source.Render(context.Background(), convConcurrent.Program); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
