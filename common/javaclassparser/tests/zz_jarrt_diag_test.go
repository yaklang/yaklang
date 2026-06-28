package tests

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// TestJarRoundTripDiag decompiles EVERY .class under RT_PKG in RT_JAR via the production JarFS path
// (enum fold + synthetic suppression), writes each as a flat `Outer$Inner.java` unit into RT_OUT
// preserving its package dir, then recompiles each unit in its own javac process with the whole tree
// on -sourcepath (so intra-jar $-references resolve to sibling decompiled SOURCES), -implicit:none,
// and RT_JAR(+RT_CP) on the classpath for external deps. This mirrors the cross-comparison harness's
// recompile axis, the truest measure of Yak's whole-program round-trip. It reports failing units with
// their javac error to surface real recompile bugs on a real library. Env-driven; skips when RT_JAR unset.
func TestJarRoundTripDiag(t *testing.T) {
	jarPath := os.Getenv("RT_JAR")
	if jarPath == "" {
		t.Skip("set RT_JAR=/path/to.jar RT_PKG=com/google/common/base/ RT_OUT=/tmp/rt to run")
	}
	pkg := os.Getenv("RT_PKG")
	outDir := os.Getenv("RT_OUT")
	if outDir == "" {
		outDir = t.TempDir()
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("no javac")
	}

	jf, err := javaclassparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatal(err)
	}

	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	var names []string
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".class") && strings.HasPrefix(f.Name, pkg) {
			names = append(names, f.Name)
		}
	}
	sort.Strings(names)
	t.Logf("found %d classes under %q", len(names), pkg)

	srcDir := filepath.Join(outDir, "src")
	_ = os.RemoveAll(srcDir)
	_ = os.MkdirAll(srcDir, 0o755)
	var written []string
	for _, n := range names {
		src, err := jf.ReadFile(n)
		if err != nil {
			t.Logf("decompile %s: %v", n, err)
			continue
		}
		jpath := filepath.Join(srcDir, filepath.FromSlash(strings.TrimSuffix(n, ".class")+".java"))
		_ = os.MkdirAll(filepath.Dir(jpath), 0o755)
		if err := os.WriteFile(jpath, src, 0o644); err != nil {
			t.Fatal(err)
		}
		written = append(written, jpath)
	}
	t.Logf("wrote %d java files to %s", len(written), srcDir)

	clsDir := filepath.Join(outDir, "_cls")
	_ = os.RemoveAll(clsDir)
	_ = os.MkdirAll(clsDir, 0o755)
	cp := jarPath
	if extra := os.Getenv("RT_CP"); extra != "" {
		cp = jarPath + ":" + extra
	}

	type res struct {
		path string
		out  string
		err  error
	}
	results := make([]res, len(written))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, jpath := range written {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, jpath string) {
			defer wg.Done()
			defer func() { <-sem }()
			out, err := exec.Command(javac, "-encoding", "UTF-8", "-sourcepath", srcDir,
				"-implicit:none", "-cp", cp, "-d", clsDir, jpath).CombinedOutput()
			results[i] = res{jpath, string(out), err}
		}(i, jpath)
	}
	wg.Wait()

	fails := 0
	shown := 0
	cat := map[string]int{}
	showSrc := os.Getenv("RT_SHOW") != ""
	for _, r := range results {
		if r.err == nil {
			continue
		}
		fails++
		for _, ln := range strings.Split(r.out, "\n") {
			i := strings.Index(ln, "错误: ")
			tag := "错误: "
			if i < 0 {
				i = strings.Index(ln, "error: ")
				tag = "error: "
			}
			if i < 0 {
				continue
			}
			msg := ln[i+len(tag):]
			msg = normCat(msg)
			cat[msg]++
			break
		}
		if showSrc && shown < 60 {
			shown++
			rel := strings.TrimPrefix(r.path, srcDir+"/")
			t.Logf("FAIL %s\n%s", rel, firstNLinesDiag(r.out, 3))
		}
	}
	type kv struct {
		k string
		v int
	}
	var sorted []kv
	for k, v := range cat {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })
	for _, e := range sorted {
		t.Logf("CAT %3d  %s", e.v, e.k)
	}
	t.Logf("RESULT recompile: %d/%d units FAILED under %q", fails, len(written), pkg)
}

// normCat collapses numeric/identifier specifics so similar errors group together.
func normCat(s string) string {
	s = strings.TrimSpace(s)
	for _, d := range []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"} {
		s = strings.ReplaceAll(s, d, "N")
	}
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func firstNLinesDiag(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
