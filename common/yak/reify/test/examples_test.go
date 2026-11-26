package test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/asm2lowir"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowir2ssa"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	"github.com/yaklang/yaklang/common/yak/reify/loader"
)

// TestRelocatableObjectsLiftTheirFirstFunction checks an object file whose first
// function sits at offset zero inside its section. A linker assigns section
// addresses, so the object records zero everywhere; an address of zero is a real
// function start there and must not be dropped as a missing value.
func TestRelocatableObjectsLiftTheirFirstFunction(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc is not installed")
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "unit.c")
	if err := os.WriteFile(source, []byte(
		"int add(int a, int b) { return a + b; }\n"+
			"int sub(int a, int b) { return a - b; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	object := filepath.Join(directory, "unit.o")
	compile := exec.Command("gcc", "-O1", "-c", "-o", object, source)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Skipf("cannot build object: %v\n%s", err, output)
	}
	artifact, err := loader.Default.OpenFile(context.Background(), object)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	result, err := asm2lowir.Parse(context.Background(), artifact, asm2lowir.WithAll())
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool, len(result.IR.Functions))
	for _, function := range result.IR.Functions {
		names[function.Name] = true
	}
	for _, want := range []string{"add", "sub"} {
		if !names[want] {
			t.Fatalf("function %q not recovered: %v", want, names)
		}
	}
}

// TestCOFFObjectLiftsEveryFunction checks a COFF object from a real assembler.
// The names are longer than the eight inline bytes, so the loader must read the
// string table, and the first function sits at section offset zero.
func TestCOFFObjectLiftsEveryFunction(t *testing.T) {
	if _, err := exec.LookPath("llvm-mc"); err != nil {
		t.Skip("llvm-mc is not installed")
	}
	directory := t.TempDir()
	source := filepath.Join(directory, "unit.s")
	assembly := ".text\n" +
		".globl add_numbers\nadd_numbers:\n\tmovl %ecx, %eax\n\taddl %edx, %eax\n\tret\n" +
		".globl sub_numbers\nsub_numbers:\n\tmovl %ecx, %eax\n\tsubl %edx, %eax\n\tret\n"
	if err := os.WriteFile(source, []byte(assembly), 0o644); err != nil {
		t.Fatal(err)
	}
	object := filepath.Join(directory, "unit.obj")
	compile := exec.Command("llvm-mc", "-triple=i686-pc-windows-msvc", "-filetype=obj", "-o", object, source)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Skipf("cannot assemble: %v\n%s", err, output)
	}
	artifact, err := loader.Default.OpenFile(context.Background(), object)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	if artifact.Format != core.FormatCOFF {
		t.Fatalf("format = %s", artifact.Format)
	}
	result, err := asm2lowir.Parse(context.Background(), artifact, asm2lowir.WithAll())
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool, len(result.IR.Functions))
	for _, function := range result.IR.Functions {
		names[function.Name] = true
	}
	for _, want := range []string{"add_numbers", "sub_numbers"} {
		if !names[want] {
			t.Fatalf("function %q not recovered: %v", want, names)
		}
	}
}

func TestHelloExampleCompilesAndLifts(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc is not installed")
	}
	binary := filepath.Join(t.TempDir(), "hello")
	compile := exec.Command("gcc", "-O0", "-fno-pie", "-no-pie", "-fno-stack-protector", "-o", binary, "examples/hello.c")
	if output, err := compile.CombinedOutput(); err != nil {
		t.Skipf("cannot build example: %v\n%s", err, output)
	}
	artifact, err := loader.Default.OpenFile(context.Background(), binary)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	result, err := asm2lowir.Parse(context.Background(), artifact, asm2lowir.WithFunction("main"))
	if err != nil {
		t.Fatal(err)
	}
	converted, err := lowir2ssa.Convert(context.Background(), result.IR)
	if err != nil {
		t.Fatal(err)
	}
	source, err := ssa2source.Render(context.Background(), converted.Program)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"main(", "return "} {
		if !strings.Contains(source, fragment) {
			t.Fatalf("source lacks %q:\n%s", fragment, source)
		}
	}
}

// TestEveryCExampleLiftsWithoutPanic lifts every C example end to end. A panic in
// the converter or the renderer becomes an error, so a clean run proves the
// examples stay inside the instruction forms they model. Diagnostics are not part
// of the check: a region boundary depends on the toolchain, and a block that ends
// at the end of the decoded region is a benign artifact of the cut.
func TestEveryCExampleLiftsWithoutPanic(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc is not installed")
	}
	sources, err := filepath.Glob("examples/*.c")
	if err != nil || len(sources) == 0 {
		t.Fatalf("no C examples: %v", err)
	}
	for _, source := range sources {
		t.Run(filepath.Base(source), func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), "unit")
			compile := exec.Command("gcc", "-O0", "-fno-pie", "-no-pie", "-fno-stack-protector", "-o", binary, source)
			if output, err := compile.CombinedOutput(); err != nil {
				t.Skipf("cannot build example: %v\n%s", err, output)
			}
			result, err := reify.Lift(context.Background(), binary, reify.WithMode(reify.ModeSource))
			if err != nil {
				t.Fatalf("lift failed: %v", err)
			}
			defer result.Close()
			if result.Functions == 0 {
				t.Fatal("no function recovered")
			}
			if strings.TrimSpace(result.Source) == "" {
				t.Fatal("no source produced")
			}
		})
	}
}
