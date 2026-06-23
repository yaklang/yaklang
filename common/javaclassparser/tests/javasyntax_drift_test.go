package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/java/javasyntax"
)

// TestJavaSyntaxNoDriftFromFrontend asserts the decompiler's in-package validator
// (javasyntax.Validate) agrees with the SSA frontend (java2ssa.Frontend) on the SAME input.
// They must never disagree, otherwise the safety net would over- or under-stub relative to
// jdsc's notion of "valid". This is the canonical guard against the extracted leaf package
// drifting from the frontend it was factored out of.
func TestJavaSyntaxNoDriftFromFrontend(t *testing.T) {
	dir := os.Getenv("JDSC_DIR")
	if dir == "" {
		dir = "/tmp/jdsc-final"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no jdsc dir: %v", err)
	}
	var total, disagree int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".java") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		src := string(raw)
		total++
		validateErr := javasyntax.Validate(src)
		_, frontendErr := java2ssa.Frontend(src)
		if (validateErr == nil) != (frontendErr == nil) {
			disagree++
			t.Errorf("[DRIFT] %s: javasyntax.Validate ok=%v but Frontend ok=%v", name, validateErr == nil, frontendErr == nil)
		}
	}
	t.Logf("==== javasyntax vs Frontend agreement: total=%d disagree=%d ====", total, disagree)
}

// TestSuspectDeterminism checks whether the two flagged classes decompile deterministically.
// If decompilation is non-deterministic, the "false positive" in TestSafetyNetIsolated is a
// decompiler-output difference between runs, not a validator disagreement.
func TestSuspectDeterminism(t *testing.T) {
	dir := os.Getenv("JDSC_DIR")
	if dir == "" {
		dir = "/tmp/jdsc-final"
	}
	suspects := []string{
		"partial-decompile-5e9da855ea25409c87f1d598.class",
		"syntax-error--9029f2fb8110de78c25b0a00.class",
	}
	javaclassparser.EnableDecompileSyntaxValidation = false
	defer func() { javaclassparser.EnableDecompileSyntaxValidation = true }()
	for _, name := range suspects {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Logf("skip %s: %v", name, err)
			continue
		}
		validCount, invalidCount := 0, 0
		var lens []int
		for i := 0; i < 8; i++ {
			out, derr := javaclassparser.Decompile(raw)
			if derr != nil {
				continue
			}
			lens = append(lens, len(out))
			if _, ferr := java2ssa.Frontend(out); ferr == nil {
				validCount++
			} else {
				invalidCount++
			}
		}
		t.Logf("%s (net OFF): valid=%d invalid=%d lens=%v", name, validCount, invalidCount, lens)
	}
}
