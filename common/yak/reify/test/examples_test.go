package test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core/asm2lowir"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowir2ssa"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	"github.com/yaklang/yaklang/common/yak/reify/loader"
)

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
