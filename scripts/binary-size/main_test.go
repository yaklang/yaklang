package main

import (
	"os"
	"runtime"
	"testing"
)

func TestInspectRunningExecutable(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("ELF/Mach-O inventory")
	}
	name, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r, err := inspect(name, report{BuildSettings: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if r.SectionBytes+r.OtherFileBytes != uint64(r.FileBytes) || r.GoCodeBytes == 0 {
		t.Fatalf("invalid file/code accounting: %+v", r)
	}
	var packageBytes, subsystemBytes uint64
	for _, p := range r.Packages {
		packageBytes += p.CodeBytes
	}
	for _, p := range r.Subsystems {
		subsystemBytes += p.CodeBytes
	}
	if packageBytes != r.GoCodeBytes || subsystemBytes != r.GoCodeBytes {
		t.Fatal("package/subsystem code totals must partition Go function spans")
	}
	var pclnBytes, subtableBytes uint64
	for _, s := range r.Sections {
		if s.Name == ".gopclntab" || s.Name == "__DATA_CONST/__gopclntab" {
			pclnBytes = s.Bytes
		}
	}
	for _, s := range r.PCLNSubtables {
		subtableBytes += s.Bytes
	}
	if len(r.PCLNSubtables) > 0 && pclnBytes != subtableBytes {
		t.Fatalf("pclntab must not be counted twice: section=%d subtables=%d", pclnBytes, subtableBytes)
	}
}

func TestSlimDependencyBoundary(t *testing.T) {
	const root = "github.com/yaklang/yaklang/"
	for _, name := range []string{"common/yak/csharp/parser", "common/yak/typescript/frontend/ast", "common/yak/ssa_compile"} {
		if !forbiddenSlimPackage(root + name) {
			t.Errorf("missed compiler dependency %s", name)
		}
	}
	for _, name := range []string{"common/yak/antlr4yak/parser", "common/yak/yak2ssa", "common/bin-parser/parser", "common/yak/ssaapi", "common/yak/c2ssa/preprocess"} {
		if forbiddenSlimPackage(root + name) {
			t.Errorf("must retain script/completion/protocol dependency %s", name)
		}
	}
}

func TestPCLNMalformedOrUnknown(t *testing.T) {
	for _, data := range [][]byte{nil, {0xf1, 0xff, 0xff, 0xff}, make([]byte, 80)} {
		if got := pclnSubtables(data); got != nil {
			t.Fatalf("unexpected table for unknown/truncated input: %v", got)
		}
	}
}
