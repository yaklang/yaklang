package obfuscation

import (
	"path/filepath"
	"testing"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/core"
)

type fakeRuntimeObf struct {
	name string
	deps []core.RuntimeDep
}

func (f fakeRuntimeObf) Name() string         { return f.name }
func (f fakeRuntimeObf) Kind() Kind           { return KindHybrid }
func (f fakeRuntimeObf) Apply(*Context) error { return nil }
func (f fakeRuntimeObf) RuntimeDeps() []core.RuntimeDep {
	return f.deps
}

func TestCollectRuntimeDeps_VirtualizeRegistered(t *testing.T) {
	deps := CollectRuntimeDeps([]string{"virtualize", "xor", "unknown"})
	if len(deps) != 1 {
		t.Fatalf("len(deps) = %d, want 1", len(deps))
	}
	dep := deps[0]
	if dep.ObfName != "virtualize" {
		t.Errorf("ObfName = %q, want virtualize", dep.ObfName)
	}
	if dep.ArchiveName != "virtualize" {
		t.Errorf("ArchiveName = %q, want virtualize", dep.ArchiveName)
	}
	if !dep.FallbackToMain {
		t.Error("expected FallbackToMain == true for virtualize")
	}
	if len(dep.Symbols) != 1 || dep.Symbols[0] != "yak_runtime_invoke_vm" {
		t.Errorf("Symbols = %v, want [yak_runtime_invoke_vm]", dep.Symbols)
	}
}

func TestRuntimeDepArchiveFileName(t *testing.T) {
	dep := &RuntimeDep{ArchiveName: "foo"}
	if got := dep.ArchiveFileName(); got != "libyakobf_foo.a" {
		t.Errorf("ArchiveFileName() = %q, want libyakobf_foo.a", got)
	}
}

func TestCollectRuntimeDeps_Empty(t *testing.T) {
	deps := CollectRuntimeDeps(nil)
	if len(deps) != 0 {
		t.Errorf("expected empty deps, got %d", len(deps))
	}
}

func TestExtraRuntimeArchivePaths(t *testing.T) {
	deps := []*RuntimeDep{
		{ObfName: "a", ArchiveName: "shared", FallbackToMain: false},
		{ObfName: "b", ArchiveName: "shared", FallbackToMain: false},
	}
	paths := ExtraRuntimeArchivePaths(deps, "/w")
	if len(paths) != 1 {
		t.Fatalf("expected 1 deduped path, got %d: %v", len(paths), paths)
	}
	want := filepath.Join("/w", "libyakobf_shared.a")
	if paths[0] != want {
		t.Errorf("path = %q, want %q", paths[0], want)
	}
}

func TestAllRuntimeSymbols(t *testing.T) {
	syms := AllRuntimeSymbols()
	found := false
	for _, s := range syms {
		if s == "yak_runtime_invoke_vm" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected yak_runtime_invoke_vm in AllRuntimeSymbols()")
	}
}

func TestCollectRuntimeDeps_DeterministicOrder(t *testing.T) {
	fake := fakeRuntimeObf{
		name: "aaa_test_obf",
		deps: []core.RuntimeDep{{
			ArchiveName:    "aaa_test",
			Symbols:        []string{"test_sym"},
			FallbackToMain: true,
		}},
	}
	Register(fake)
	defer delete(Default, fake.name)

	deps := CollectRuntimeDeps([]string{"virtualize", "aaa_test_obf"})
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}
	if deps[0].ObfName != "aaa_test_obf" {
		t.Errorf("deps[0].ObfName = %q, want aaa_test_obf", deps[0].ObfName)
	}
	if deps[1].ObfName != "virtualize" {
		t.Errorf("deps[1].ObfName = %q, want virtualize", deps[1].ObfName)
	}
}

func TestCollectRuntimeDeps_IgnoresNonProviders(t *testing.T) {
	llvmOnly := fakeLLVMNoDeps{name: "zzz_no_deps"}
	Register(llvmOnly)
	defer delete(Default, llvmOnly.name)

	deps := CollectRuntimeDeps([]string{"zzz_no_deps"})
	if len(deps) != 0 {
		t.Fatalf("expected 0 deps, got %d", len(deps))
	}
}

type fakeLLVMNoDeps struct{ name string }

func (f fakeLLVMNoDeps) Name() string         { return f.name }
func (f fakeLLVMNoDeps) Kind() Kind           { return KindLLVM }
func (f fakeLLVMNoDeps) Apply(*Context) error { return nil }

func TestApplyStillWorksWithRuntimeDepProviders(t *testing.T) {
	requireCtx := &Context{Stage: StageLLVM, LLVM: llvm.Module{}}
	if err := Apply(requireCtx, []string{"virtualize"}); err != nil {
		// virtualize is hybrid and does nothing at LLVM stage; this guards that
		// the runtime-dep interface does not interfere with ordinary apply.
		t.Fatalf("Apply failed: %v", err)
	}
}
