package main

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRuntimeLookupPatchUpgradesTextOffOnly protects the upgrade from the
// historical patch, which optimized pc->offset but left offset->pc linear.
func TestRuntimeLookupPatchUpgradesTextOffOnly(t *testing.T) {
	script, err := filepath.Abs("../../scripts/patch_go_runtime.sh")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, "src", "runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}
	// The fixture has the older textOff-only marker, not the new virtual index.
	source := `package runtime
 type textsect struct { vaddr, end, baseaddr uintptr }
 type moduledata struct { text, etext uintptr; textsectmap []textsect }
 var firstmoduledata moduledata
 func (md *moduledata) textOff(pc uintptr) { // ssa2llvm: textsect binary search
 }
 func (md *moduledata) textAddr(off32 uint32) uintptr {
 off := uintptr(off32)
 res := md.text + off
 if len(md.textsectmap) > 1 {
		for i, sect := range md.textsectmap {
			// For the last section, include the end address (etext), as it is included in the functab.
			if off >= sect.vaddr && off < sect.end || (i == len(md.textsectmap)-1 && off == sect.end) {
				res = sect.baseaddr + off - sect.vaddr
				break
			}
		}
 }
 return res
 }
 `
	path := filepath.Join(runtimeDir, "symtab.go")
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	goShim := filepath.Join(dir, "go")
	if err := os.WriteFile(goShim, []byte("#!/bin/sh\nprintf '%s\\n' '"+dir+"'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	run := func() []byte {
		t.Helper()
		cmd := exec.Command("bash", script)
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("patch: %v\n%s", err, out)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	first := run()
	if _, err := parser.ParseFile(token.NewFileSet(), path, first, parser.AllErrors); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), "virtual textsect binary search") || !strings.Contains(string(first), "var ssa2llvmTextAddrMap") {
		t.Fatal("virtual lookup missing")
	}
	if second := run(); string(second) != string(first) {
		t.Fatal("patch is not idempotent")
	}
}
