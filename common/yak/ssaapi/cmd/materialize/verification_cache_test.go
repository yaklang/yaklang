package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func TestVerificationSharesIdenticalBytesWithoutDroppingPaths(t *testing.T) {
	tree := filesys.NewVirtualFs()
	for i := 0; i < 24; i++ {
		tree.AddFile(fmt.Sprintf("lib%d/A.java", i), "class A { int f() { return 1; } }")
	}
	var calls atomic.Int32
	out := t.TempDir()
	got, err := verifyTreeWithVerifier(tree, 4, out, func(p string, b []byte, c *ssa.AntlrCache) sourceCheck { calls.Add(1); return verifySource(p, b, c) })
	if err != nil {
		t.Fatal(err)
	}
	if got.Files != 24 || got.Checked != 24 || got.SyntaxFailures != 0 {
		t.Fatalf("report: %+v", got)
	}
	if calls.Load() != 1 || got.UniqueChecks != 1 || got.ReusedChecks != 23 || got.FrontendCalls != 1 || got.FrontendFiles != 24 || got.JavaFiles != 24 {
		t.Fatalf("identical sources parsed %d times; want one proof for all 24 paths; report=%+v", calls.Load(), got)
	}
	raw, err := os.ReadFile(filepath.Join(out, "sources.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var c sourceCheck
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatal(err)
		}
		paths[c.Path] = true
	}
	if len(paths) != 24 {
		t.Fatalf("manifest lost paths: %d", len(paths))
	}
}

func TestVerificationDoesNotHideDuplicateFailuresOrDescriptorRole(t *testing.T) {
	tree := filesys.NewVirtualFs()
	tree.AddFile("one/A.java", "class A { int bad( {}")
	tree.AddFile("two/A.java", "class A { int bad( {}")
	tree.AddFile("p/package-info.java", "@Deprecated package p;")
	tree.AddFile("p/NotAClass.java", "@Deprecated package p;")
	tree.AddFile("p/not-package-info.java", "@Deprecated package p;")
	got, err := verifyTree(tree, 4, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got.Files != 5 || got.Checked != 5 || got.SyntaxFailures != 4 || got.FrontendFailures != 2 || got.EmptyUnits != 2 || got.FrontendCalls != 3 {
		t.Fatalf("invalid duplicate or non-descriptor was accepted: %+v", got)
	}
}
