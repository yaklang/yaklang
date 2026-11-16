package test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/asm2lowir"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowir2ssa"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowir2ssa/pass"
	"github.com/yaklang/yaklang/common/yak/reify/loader"
)

// assemble writes one assembly source and assembles it for the given llvm-mc
// triple. It skips the test when llvm-mc is unavailable, because the fixture has
// to be a real object file for the loader to read a real section table.
func assemble(t *testing.T, triple, source string) string {
	t.Helper()
	if _, err := exec.LookPath("llvm-mc"); err != nil {
		t.Skip("llvm-mc is not installed")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "unit.s")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	object := filepath.Join(directory, "unit.o")
	compile := exec.Command("llvm-mc", "-triple="+triple, "-filetype=obj", "-o", object, path)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Skipf("cannot assemble for %s: %v\n%s", triple, err, output)
	}
	return object
}

// liftOneFunction loads one object and decodes its whole text section as a single
// function. The frame and canary passes work per function, so a fixture that the
// boundary scanner would split is not a useful input.
func liftOneFunction(t *testing.T, object string) *core.LowIR {
	t.Helper()
	artifact, err := loader.Default.OpenFile(context.Background(), object)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = artifact.Close() })
	if len(artifact.Hints.ExecSections) == 0 {
		t.Fatal("object has no executable section")
	}
	region := artifact.Hints.ExecSections[0]
	result, err := asm2lowir.Parse(context.Background(), artifact,
		asm2lowir.WithRegion(region), asm2lowir.WithExact())
	if err != nil {
		t.Fatal(err)
	}
	return result.IR
}

// TestFrameRemovalAcrossArchitectures checks that one frame pass serves every
// architecture. Each fixture is the standard prologue and epilogue of the
// architecture, assembled by the architecture's own assembler.
func TestFrameRemovalAcrossArchitectures(t *testing.T) {
	cases := []struct {
		name   string
		triple string
		source string
	}{
		{
			name:   "amd64",
			triple: "x86_64-linux-gnu",
			source: ".text\n.globl work\nwork:\n\tpushq %rbp\n\tmovq %rsp, %rbp\n\tpushq %rbx\n\tsubq $16, %rsp\n\taddq $16, %rsp\n\tpopq %rbx\n\tpopq %rbp\n\tret\n",
		},
		{
			name:   "386",
			triple: "i386-linux-gnu",
			source: ".text\n.globl work\nwork:\n\tpushl %ebp\n\tmovl %esp, %ebp\n\tpushl %ebx\n\tsubl $16, %esp\n\taddl $16, %esp\n\tpopl %ebx\n\tpopl %ebp\n\tret\n",
		},
		{
			name:   "arm64",
			triple: "aarch64-linux-gnu",
			source: ".text\n.globl work\nwork:\n\tstp x29, x30, [sp, #-32]!\n\tmov x29, sp\n\tstp x19, x20, [sp, #16]\n\tldp x19, x20, [sp, #16]\n\tldp x29, x30, [sp], #32\n\tret\n",
		},
		{
			name:   "arm",
			triple: "armv7-linux-gnueabihf",
			source: ".text\n.globl work\nwork:\n\tpush {r4, r11, lr}\n\tmov r11, sp\n\tsub sp, sp, #16\n\tadd sp, sp, #16\n\tpop {r4, r11, pc}\n",
		},
		{
			name:   "s390x",
			triple: "s390x-linux-gnu",
			source: ".text\n.globl work\nwork:\n\tstmg %r11, %r15, 0(%r15)\n\tlay %r15, -160(%r15)\n\tlay %r15, 160(%r15)\n\tlmg %r11, %r14, 8(%r15)\n\tbr %r14\n",
		},
		{
			name:   "mips",
			triple: "mips-linux-gnu",
			source: ".text\n.globl work\nwork:\n\taddiu $sp, $sp, -24\n\tsw $ra, 20($sp)\n\tsw $fp, 16($sp)\n\tmove $fp, $sp\n\tmove $sp, $fp\n\tlw $fp, 16($sp)\n\tlw $ra, 20($sp)\n\taddiu $sp, $sp, 24\n\tjr $ra\n",
		},
		{
			name:   "ppc64",
			triple: "powerpc64le-linux-gnu",
			source: ".text\n.globl work\nwork:\n\tmflr 0\n\tstdu 1, -32(1)\n\tstd 0, 16(1)\n\taddi 1, 1, 32\n\tld 0, 16(1)\n\tmtlr 0\n\tblr\n",
		},
		{
			name:   "riscv64",
			triple: "riscv64-linux-gnu",
			source: ".text\n.globl work\nwork:\n\taddi sp, sp, -32\n\tsd ra, 24(sp)\n\tsd s0, 16(sp)\n\taddi s0, sp, 32\n\tld ra, 24(sp)\n\tld s0, 16(sp)\n\taddi sp, sp, 32\n\tret\n",
		},
		{
			name:   "loong64",
			triple: "loongarch64-linux-gnu",
			source: ".text\n.globl work\nwork:\n\taddi.d $sp, $sp, -32\n\tst.d $ra, $sp, 24\n\tst.d $fp, $sp, 16\n\taddi.d $fp, $sp, 32\n\tld.d $ra, $sp, 24\n\tld.d $fp, $sp, 16\n\taddi.d $sp, $sp, 32\n\tjr $ra\n",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			object := assemble(t, testCase.triple, testCase.source)
			ir := liftOneFunction(t, object)
			if ir.Frame.StackPointer == "" {
				t.Fatalf("%s parser did not report a stack pointer", testCase.name)
			}
			removed := 0
			for index := range ir.Functions {
				removed += pass.RemoveFrame(&ir.Functions[index], ir.Facts())
			}
			if removed == 0 {
				t.Fatalf("%s frame pass removed nothing (stack pointer %q)", testCase.name, ir.Frame.StackPointer)
			}
		})
	}
}

// TestCanaryRemovalAcrossArchitectures checks the two guard read shapes the
// pass understands. A segment base folds the offset into the marker; a thread
// pointer register adds the offset afterwards.
func TestCanaryRemovalAcrossArchitectures(t *testing.T) {
	cases := []struct {
		name   string
		triple string
		source string
	}{
		{
			name:   "amd64",
			triple: "x86_64-linux-gnu",
			source: ".text\n.globl work\nwork:\n\tpushq %rbp\n\tmovq %rsp, %rbp\n\tsubq $32, %rsp\n\tmovq %fs:40, %rax\n\tmovq %rax, -8(%rbp)\n\txorl %eax, %eax\n\tmovq -8(%rbp), %rdx\n\tsubq %fs:40, %rdx\n\tje done\n\tcall __stack_chk_fail\ndone:\n\tleave\n\tret\n",
		},
		{
			name:   "arm64",
			triple: "aarch64-linux-gnu",
			source: ".text\n.globl work\nwork:\n\tstp x29, x30, [sp, #-32]!\n\tmov x29, sp\n\tmrs x8, tpidr_el0\n\tldr x8, [x8, #40]\n\tstr x8, [sp, #24]\n\tldr x9, [sp, #24]\n\tldr x8, [x8, #40]\n\tcmp x9, x8\n\tb.ne fail\n\tldp x29, x30, [sp], #32\n\tret\nfail:\n\tbl __stack_chk_fail\n\tret\n",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			object := assemble(t, testCase.triple, testCase.source)
			ir := liftOneFunction(t, object)
			if ir.Canary.Kind == "" {
				t.Fatalf("%s parser did not report a canary model", testCase.name)
			}
			removed := 0
			for index := range ir.Functions {
				removed += pass.RemoveStackCanary(&ir.Functions[index], ir.Facts())
			}
			if removed == 0 {
				t.Fatalf("%s canary pass removed nothing", testCase.name)
			}
		})
	}
}

// TestArgumentBindingAcrossArchitectures checks that a function which reads the
// first argument register gets a parameter. The ABI facts are data now, so a
// target that is missing them comes out with no parameters at all — which is what
// happened to every architecture except the four the converter knew about.
func TestArgumentBindingAcrossArchitectures(t *testing.T) {
	cases := []struct {
		name   string
		triple string
		source string
	}{
		{"amd64", "x86_64-linux-gnu", ".text\n.globl work\nwork:\n\tmovl %edi, %eax\n\tret\n"},
		{"arm64", "aarch64-linux-gnu", ".text\n.globl work\nwork:\n\tmov w0, w1\n\tret\n"},
		{"arm", "armv7-linux-gnueabihf", ".text\n.globl work\nwork:\n\tmov r0, r1\n\tbx lr\n"},
		{"riscv64", "riscv64-linux-gnu", ".text\n.globl work\nwork:\n\taddi a1, a0, 1\n\tret\n"},
		{"s390x", "s390x-linux-gnu", ".text\n.globl work\nwork:\n\tlgr %r2, %r3\n\tbr %r14\n"},
		{"ppc64", "powerpc64le-linux-gnu", ".text\n.globl work\nwork:\n\tmr 3, 4\n\tblr\n"},
		{"mips", "mips-linux-gnu", ".text\n.globl work\nwork:\n\tmove $v0, $a0\n\tjr $ra\n"},
		{"loong64", "loongarch64-linux-gnu", ".text\n.globl work\nwork:\n\tor $a0, $a1, $zero\n\tjr $ra\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			object := assemble(t, testCase.triple, testCase.source)
			ir := liftOneFunction(t, object)
			converted, err := lowir2ssa.Convert(context.Background(), ir)
			if err != nil {
				t.Fatal(err)
			}
			// The entry function is the only function in the region, whatever
			// name the symbol table gave it.
			function, ok := converted.Program.Funcs.GetByIndex(0)
			if !ok {
				t.Fatal("no function was converted")
			}
			if len(function.Params) == 0 {
				t.Fatalf("%s bound no parameter for a function that reads its first argument register", testCase.name)
			}
		})
	}
}
