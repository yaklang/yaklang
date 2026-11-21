package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPatchDeadcodeAgainstHostGOROOT(t *testing.T) {
	goroot, err := goEnv("GOROOT")
	if err != nil {
		t.Fatalf("go env GOROOT: %v", err)
	}
	orig, err := os.ReadFile(filepath.Join(goroot, "src", "cmd", "link", "internal", "ld", "deadcode.go"))
	if err != nil {
		t.Fatalf("read deadcode.go: %v", err)
	}
	patched, err := PatchDeadcodeSource(orig)
	if err != nil {
		t.Fatalf("PatchDeadcodeSource failed: %v", err)
	}
	s := string(patched)
	if !strings.Contains(s, "yakReflectFilter.match(d.ldr, m.src)") {
		t.Fatalf("patched source missing yakReflectFilter.match call")
	}
	if !strings.Contains(s, "loadYakReflectPruneFilter()") {
		t.Fatalf("patched source missing loadYakReflectPruneFilter()")
	}
	// Idempotent check.
	patched2, err := PatchDeadcodeSource(patched)
	if err != nil {
		t.Fatalf("second PatchDeadcodeSource failed: %v", err)
	}
	if string(patched2) != s {
		t.Fatalf("PatchDeadcodeSource is not idempotent")
	}
}

func TestPatchDeadcodeFailsLoudlyOnDrift(t *testing.T) {
	_, err := PatchDeadcodeSource([]byte("package ld\nfunc deadcode() {}\n"))
	if err == nil {
		t.Fatalf("expected error on missing anchor, got nil")
	}
}

func TestDefaultProfilesProtectScriptReflectedPackages(t *testing.T) {
	protected := []string{
		"yaklib",
		"ssaapi",
		"schema",
		"ypb",
		"goja",
		"pcapx",
		"gopacket",
		"gvisor",
		"javajive",
	}
	for name, patterns := range DefaultProfiles {
		for _, pat := range patterns {
			if strings.HasPrefix(pat, "!") {
				continue
			}
			for _, guard := range protected {
				if strings.Contains(pat, guard) {
					t.Errorf("profile %q must not exclude script-reflected package pattern %q (matches %q)", name, pat, guard)
				}
			}
		}
	}
}

func TestEndToEndReflectPrune(t *testing.T) {
	tmp := t.TempDir()
	exeSuffix := ""
	if runtime.GOOS == "windows" {
		exeSuffix = ".exe"
	}

	// 1. Build this wrapper binary.
	wrapperBin := filepath.Join(tmp, "reflect-prune-link"+exeSuffix)
	buildWrapper := exec.Command("go", "build", "-o", wrapperBin, ".")
	if out, err := buildWrapper.CombinedOutput(); err != nil {
		t.Fatalf("build wrapper: %v\n%s", err, out)
	}

	// 2. Prepare patched linker.
	patchedLink := filepath.Join(tmp, "patched-link"+exeSuffix)
	prep := exec.Command(wrapperBin, "prepare", "-o", patchedLink)
	if out, err := prep.CombinedOutput(); err != nil {
		t.Fatalf("prepare patched linker: %v\n%s", err, out)
	}

	// 3. Create a tiny standalone module with:
	//    - prunedpkg.PrunedType: UnusedExported() (must be pruned) + IfaceMethod() (must survive)
	//    - keptpkg.KeptType: ReflectExported() (must survive and be callable via reflect)
	modDir := filepath.Join(tmp, "samplemod")
	if err := os.MkdirAll(filepath.Join(modDir, "prunedpkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(modDir, "keptpkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(modDir, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/samplemod\n\ngo 1.22\n")
	write("prunedpkg/p.go", `package prunedpkg

type PrunedType struct{ V int }

//go:noinline
func (p *PrunedType) UnusedExported() int { return p.V + 42 }

//go:noinline
func (p *PrunedType) IfaceMethod() int { return p.V + 7 }
`)
	write("keptpkg/k.go", `package keptpkg

type KeptType struct{ V int }

//go:noinline
func (k *KeptType) ReflectExported() int { return k.V + 99 }
`)
	write("main.go", `package main

import (
	"fmt"
	"os"
	"reflect"

	"example.com/samplemod/keptpkg"
	"example.com/samplemod/prunedpkg"
)

type runner interface {
	IfaceMethod() int
}

//go:noinline
func box(v any) any { return v }

func main() {
	r := box(&prunedpkg.PrunedType{V: 10}).(runner)
	if r.IfaceMethod() != 17 {
		os.Exit(11)
	}
	k := &keptpkg.KeptType{V: 1}
	name := "ReflectExported"
	if len(os.Args) > 99 {
		name = os.Args[99]
	}
	m := reflect.ValueOf(k).MethodByName(name)
	if !m.IsValid() {
		os.Exit(12)
	}
	res := m.Call(nil)
	if res[0].Int() != 100 {
		os.Exit(13)
	}
	fmt.Println("OK")
}
`)

	sampleBin := filepath.Join(tmp, "sample"+exeSuffix)
	toolexec := wrapperBin + " exec --link " + patchedLink + " --verbose --"
	buildSample := exec.Command("go", "build", "-toolexec="+toolexec, "-o", sampleBin, ".")
	buildSample.Dir = modDir
	buildSample.Env = append(os.Environ(), "YAK_REFLECT_PRUNE_EXCLUDE=example.com/samplemod/prunedpkg.")
	out, err := buildSample.CombinedOutput()
	if err != nil {
		t.Fatalf("build sample with patched linker: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "[reflect-prune] pruned 1 reflect-only exported methods") {
		t.Fatalf("expected verbose prune count = 1, got:\n%s", out)
	}

	// 4. Run sample binary: verifies interface dispatch on prunedpkg and reflect on keptpkg.
	runOut, err := exec.Command(sampleBin).CombinedOutput()
	if err != nil {
		t.Fatalf("run sample binary: %v\n%s", err, runOut)
	}
	if strings.TrimSpace(string(runOut)) != "OK" {
		t.Fatalf("unexpected sample output: %q", string(runOut))
	}

	// 5. Inspect symbols with `go tool nm`: UnusedExported must be gone, IfaceMethod & ReflectExported present.
	nmOut, err := exec.Command("go", "tool", "nm", sampleBin).CombinedOutput()
	if err != nil {
		t.Fatalf("go tool nm: %v\n%s", err, nmOut)
	}
	syms := string(nmOut)
	if strings.Contains(syms, "UnusedExported") {
		t.Fatalf("expected UnusedExported to be pruned, but found in nm output")
	}
	if !strings.Contains(syms, "IfaceMethod") {
		t.Fatalf("expected IfaceMethod to survive (interface method)")
	}
	if !strings.Contains(syms, "ReflectExported") {
		t.Fatalf("expected ReflectExported to survive (keptpkg)")
	}
}
