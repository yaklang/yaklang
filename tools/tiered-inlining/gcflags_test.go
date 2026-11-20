package main

import (
	"strings"
	"testing"
)

func TestHotPathsProtectedFromNoinline(t *testing.T) {
	// These hot-path package prefixes MUST remain at Go's default inlining level
	// to preserve YakVM, SSA, SyntaxFlow, ANTLR runtime, and HTTP/codec throughput.
	protectedHotPaths := []string{
		"all",
		"std",
		"runtime",
		"github.com/yaklang/antlr/v4",
		"github.com/yaklang/yaklang/common/yak/yakvm",
		"github.com/yaklang/yaklang/common/yak/ssa",
		"github.com/yaklang/yaklang/common/yak/yaklib",
		"github.com/yaklang/yaklang/common/syntaxflow/sfvm",
		"github.com/yaklang/yaklang/common/utils",
	}

	for name, pkgs := range Profiles {
		for _, pkg := range pkgs {
			clean := strings.TrimSuffix(pkg, "/...")
			for _, hot := range protectedHotPaths {
				if clean == hot || strings.HasPrefix(clean, hot+"/") {
					// Allow specific cold subpackages under common/yak/ssa only if never matching ssa itself.
					t.Errorf("profile %q disables inlining on protected hot-path package %q (matches %q)", name, pkg, hot)
				}
			}
		}
	}
}

func TestGCFlagsFormatting(t *testing.T) {
	for _, name := range []string{"tierA", "tierB"} {
		flags, err := GCFlags(name)
		if err != nil {
			t.Fatalf("GCFlags(%q) failed: %v", name, err)
		}
		if len(flags) == 0 {
			t.Fatalf("GCFlags(%q) returned empty slice", name)
		}
		for _, f := range flags {
			if !strings.HasPrefix(f, "-gcflags=") || !strings.HasSuffix(f, "=-l") {
				t.Errorf("unexpected flag format in %q: %q", name, f)
			}
		}
	}

	offFlags, err := GCFlags("off")
	if err != nil || len(offFlags) != 0 {
		t.Fatalf("GCFlags(off) = (%v, %v), want (nil, nil)", offFlags, err)
	}
}
