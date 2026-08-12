package tests

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// pruningCase describes one representative script and the yaklib module groups
// it must retain (.modtext sections present) and must NOT retain (absent).
// These reflect the production per-script module selection: a plain print
// script needs only the core runtime, the ssa script keeps the ssa module and
// its language frontends, and the poc script keeps the poc/cli/http groups.
type pruningCase struct {
	script     string
	outputSub  string // expected substring of the compiled binary's output
	wantRetain []string
	wantAbsent []string
}

var pruningCases = []pruningCase{
	{
		script:     "print_stdlib.yak",
		outputSub:  "hello world 123",
		wantRetain: nil, // base runtime: no dedicated module group
		wantAbsent: []string{"ssa", "ssafront", "poc", "cli"},
	},
	{
		script:     "ssa_go_parse.yak",
		outputSub:  "hello-from-go",
		wantRetain: []string{"ssa", "ssafront", "shared"},
		wantAbsent: []string{"poc", "cli"},
	},
	{
		script:     "poc_request.yak",
		outputSub:  "0",
		wantRetain: []string{"poc", "cli", "shared"},
		wantAbsent: []string{"ssa", "ssafront"},
	},
}

// modtextModules returns the sorted set of ".modtext.<m>" module names in the
// ELF, excluding the plain ".modtext" section name itself.
func modtextModules(f *elf.File) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range f.Sections {
		if !strings.HasPrefix(s.Name, ".modtext.") {
			continue
		}
		m := strings.TrimPrefix(s.Name, ".modtext.")
		if m != "" && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

func hasExecSection(f *elf.File, name string) bool {
	for _, s := range f.Sections {
		if s.Name == name && s.Flags&elf.SHF_EXECINSTR != 0 {
			return true
		}
	}
	return false
}

// TestArtifactPruning_DualEvidence compiles each representative script via the
// real ssa2llvm CLI, then verifies three layers of pruning evidence on the
// produced ELF:
//   A. behavior — the binary runs under env -i and prints the expected output;
//   B. size     — the exact byte size is recorded and printed;
//   C. content  — required yaklib module groups are present as real executable
//      .modtext sections, and unneeded groups are absent.
//
// This is a regression guard: if per-script pruning stops working (e.g. all
// modules are retained), the wantAbsent assertions fail.
func TestArtifactPruning_DualEvidence(t *testing.T) {
	repoRoot := RepoRoot(t)
	scriptDir := filepath.Join(repoRoot, "common", "yak", "ssa2llvm", "tests", "script")
	tmpDir := t.TempDir()

	sizes := make(map[string]int64)
	for _, tc := range pruningCases {
		tc := tc
		t.Run(tc.script, func(t *testing.T) {
			script := filepath.Join(scriptDir, tc.script)
			bin := filepath.Join(tmpDir, strings.TrimSuffix(tc.script, ".yak")+".bin")

			// Compile via the real CLI.
			res := runSSA2LLVMCLI(t, "compile", script, "-o", bin, "-f", "", "-a")
			if res.ExitCode != 0 {
				t.Fatalf("ssa2llvm compile %s failed (exit %d):\n%s", tc.script, res.ExitCode, res.Output)
			}

			// A. behavior: run under empty env, check expected output.
			run := runProcess(t, bin, nil)
			if run.ExitCode != 0 {
				t.Fatalf("compiled %s exited %d:\n%s", tc.script, run.ExitCode, run.Output)
			}
			if !strings.Contains(run.Output, tc.outputSub) {
				t.Fatalf("compiled %s output missing %q:\n%s", tc.script, tc.outputSub, run.Output)
			}

			// B. size: exact byte size.
			fi, err := os.Stat(bin)
			if err != nil {
				t.Fatalf("stat %s: %v", bin, err)
			}
			sizes[tc.script] = fi.Size()
			t.Logf("size %s = %d bytes", tc.script, fi.Size())

			// C. content: open ELF via debug/elf (CI-reliable, no host nm/readelf).
			f, err := elf.Open(bin)
			if err != nil {
				t.Fatalf("elf.Open %s: %v", bin, err)
			}
			defer f.Close()
			mods := modtextModules(f)
			t.Logf("modtext modules: %v", mods)

			present := map[string]bool{}
			for _, m := range mods {
				present[m] = true
			}
			for _, want := range tc.wantRetain {
				if !present[want] {
					t.Fatalf("%s: expected retained module %q, but .modtext.%s is absent (mods=%v)", tc.script, want, want, mods)
				}
				if !hasExecSection(f, ".modtext."+want) {
					t.Fatalf("%s: retained module %q section is not executable", tc.script, want)
				}
			}
			for _, forbid := range tc.wantAbsent {
				if present[forbid] {
					t.Fatalf("%s: module %q should be pruned, but .modtext.%s is present (mods=%v)", tc.script, forbid, forbid, mods)
				}
			}
		})
	}

	// Cross-script size ordering: a script with more module groups must be
	// larger than a script with fewer, reflecting per-script selection.
	if len(sizes) == 3 {
		printSZ := sizes["print_stdlib.yak"]
		pocSZ := sizes["poc_request.yak"]
		ssaSZ := sizes["ssa_go_parse.yak"]
		t.Logf("print=%d poc=%d ssa=%d", printSZ, pocSZ, ssaSZ)
		if !(printSZ < pocSZ && pocSZ < ssaSZ) {
			t.Errorf("size ordering not explainable by module selection: print=%d poc=%d ssa=%d", printSZ, pocSZ, ssaSZ)
		}
	} else {
		t.Logf("skipping size ordering: compiled %d/3 cases", len(sizes))
	}

	fmt.Printf("artifact-pruning sizes: %v\n", sizes)
}
