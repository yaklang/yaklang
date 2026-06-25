package loop_ssa_api_discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// dirTreeBenchSpec describes a synthetic multi-module Java repo for directory-tree benchmarks.
type dirTreeBenchSpec struct {
	Name         string
	ModuleCount  int
	FilesPerMod  int
	PackageDepth int // e.g. 4 -> com/example/app/service
}

var dirTreeBenchSpecs = []dirTreeBenchSpec{
	{Name: "tiny", ModuleCount: 1, FilesPerMod: 10, PackageDepth: 3},
	{Name: "small", ModuleCount: 1, FilesPerMod: 100, PackageDepth: 4},
	{Name: "medium", ModuleCount: 3, FilesPerMod: 50, PackageDepth: 4},
	{Name: "large", ModuleCount: 5, FilesPerMod: 200, PackageDepth: 5},
}

func packageSegments(depth int, modIdx, fileIdx int) string {
	segs := []string{"com", "example", fmt.Sprintf("mod%d", modIdx)}
	for i := 0; len(segs) < depth; i++ {
		segs = append(segs, fmt.Sprintf("pkg%d", i))
	}
	return strings.Join(segs, "/")
}

func writeSyntheticJavaFile(path, pkg, className string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf("package %s;\n\npublic class %s {}\n", pkg, className)
	return os.WriteFile(path, []byte(body), 0o644)
}

// setupSyntheticJavaRepo creates a Maven-like multi-module tree under a temp dir.
func setupSyntheticJavaRepo(tb testing.TB, spec dirTreeBenchSpec) string {
	tb.Helper()
	root := tb.TempDir()
	for m := 0; m < spec.ModuleCount; m++ {
		modName := fmt.Sprintf("module-%d", m)
		for f := 0; f < spec.FilesPerMod; f++ {
			pkgPath := packageSegments(spec.PackageDepth, m, f)
			pkg := strings.ReplaceAll(pkgPath, "/", ".")
			className := fmt.Sprintf("Type%d", f)
			javaPath := filepath.Join(root, modName, "src", "main", "java", pkgPath, className+".java")
			if err := writeSyntheticJavaFile(javaPath, pkg, className); err != nil {
				tb.Fatalf("write java file: %v", err)
			}
		}
		// noise dirs that must be skipped
		for _, noise := range []string{"target/classes", "build/out", "node_modules/x"} {
			p := filepath.Join(root, modName, noise, "Ignored.class")
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				tb.Fatalf("mkdir noise: %v", err)
			}
			if err := os.WriteFile(p, []byte("noise"), 0o644); err != nil {
				tb.Fatalf("write noise: %v", err)
			}
		}
	}
	return root
}

func TestBuildDirectoryTree_syntheticScales(t *testing.T) {
	maxDuration := map[string]time.Duration{
		"tiny":   500 * time.Millisecond,
		"small":  2 * time.Second,
		"medium": 5 * time.Second,
		"large":  15 * time.Second,
	}

	for _, spec := range dirTreeBenchSpecs {
		spec := spec
		t.Run(spec.Name, func(t *testing.T) {
			root := setupSyntheticJavaRepo(t, spec)
			wantFiles := spec.ModuleCount * spec.FilesPerMod

			start := time.Now()
			javaRoot := FindJavaRoot(root)
			findDur := time.Since(start)
			if javaRoot == "" {
				t.Fatalf("FindJavaRoot returned empty for %s", root)
			}
			if !isDirAncestorOrEqual(root, javaRoot) {
				t.Fatalf("FindJavaRoot escaped project root: root=%q javaRoot=%q", root, javaRoot)
			}
			t.Logf("FindJavaRoot: %v -> %s", findDur.Round(time.Millisecond), javaRoot)

			treeStart := time.Now()
			tree := BuildDirectoryTree(javaRoot)
			treeDur := time.Since(treeStart)
			if tree == nil {
				t.Fatal("BuildDirectoryTree returned nil")
			}
			if tree.TotalFiles != wantFiles {
				t.Fatalf("TotalFiles=%d want=%d (dirs=%d kb=%d)", tree.TotalFiles, wantFiles, tree.TotalDirs, tree.TotalSizeKB)
			}
			t.Logf("BuildDirectoryTree: %v dirs=%d files=%d kb=%d", treeDur.Round(time.Millisecond), tree.TotalDirs, tree.TotalFiles, tree.TotalSizeKB)

			limit, ok := maxDuration[spec.Name]
			if ok && treeDur > limit {
				t.Fatalf("BuildDirectoryTree too slow: %v > limit %v", treeDur, limit)
			}
		})
	}
}

func TestRunDirectoryAnalysisPipeline_syntheticMedium(t *testing.T) {
	spec := dirTreeBenchSpec{Name: "medium-pipeline", ModuleCount: 2, FilesPerMod: 30, PackageDepth: 4}
	root := setupSyntheticJavaRepo(t, spec)

	start := time.Now()
	javaRoot := FindJavaRoot(root)
	tree := BuildDirectoryTree(javaRoot)
	units, err := EstimateWorkUnits(tree)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("EstimateWorkUnits: %v", err)
	}
	t.Logf("pipeline(no react): %v javaRoot=%s dirs=%d files=%d workUnits=%d",
		elapsed.Round(time.Millisecond), javaRoot, tree.TotalDirs, tree.TotalFiles, len(units))
	if tree.TotalFiles != spec.ModuleCount*spec.FilesPerMod {
		t.Fatalf("unexpected file count: %d", tree.TotalFiles)
	}
}

func BenchmarkFindJavaRoot(b *testing.B) {
	for _, spec := range dirTreeBenchSpecs {
		spec := spec
		b.Run(spec.Name, func(b *testing.B) {
			root := setupSyntheticJavaRepo(b, spec)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if got := FindJavaRoot(root); got == "" {
					b.Fatal("empty java root")
				}
			}
		})
	}
}

func BenchmarkBuildDirectoryTree(b *testing.B) {
	for _, spec := range dirTreeBenchSpecs {
		spec := spec
		b.Run(spec.Name, func(b *testing.B) {
			root := setupSyntheticJavaRepo(b, spec)
			javaRoot := FindJavaRoot(root)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tree := BuildDirectoryTree(javaRoot)
				if tree == nil || tree.TotalFiles != spec.ModuleCount*spec.FilesPerMod {
					b.Fatalf("unexpected tree: %+v", tree)
				}
			}
		})
	}
}
