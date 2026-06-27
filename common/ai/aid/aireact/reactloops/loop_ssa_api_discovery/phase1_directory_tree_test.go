package loop_ssa_api_discovery

import (
	"path/filepath"
	"testing"
)

func TestFindCommonAncestor_multiModuleUnderProjectRoot(t *testing.T) {
	projectRoot := "/home/user/PublicCMS/publiccms-parent"
	paths := []string{
		filepath.Join(projectRoot, "publiccms-core/src"),
		filepath.Join(projectRoot, "publiccms-oauth/src"),
	}
	got := findCommonAncestor(paths, projectRoot)
	if got != projectRoot {
		t.Fatalf("findCommonAncestor() = %q, want %q", got, projectRoot)
	}
}

func TestFindCommonAncestor_neverEscapesProjectRoot(t *testing.T) {
	projectRoot := "/home/user/PublicCMS"
	paths := []string{
		"/home/user/PublicCMS/module-a/src",
		"/var/other/module-b/src",
	}
	got := findCommonAncestor(paths, projectRoot)
	if got != projectRoot {
		t.Fatalf("findCommonAncestor() = %q, want clamped %q", got, projectRoot)
	}
}

func TestClampJavaRootToProject(t *testing.T) {
	projectRoot := "/home/user/repo"
	if got := clampJavaRootToProject("/", projectRoot); got != projectRoot {
		t.Fatalf("clampJavaRootToProject(/) = %q, want %q", got, projectRoot)
	}
	inner := filepath.Join(projectRoot, "module/src/main/java")
	if got := clampJavaRootToProject(inner, projectRoot); got != inner {
		t.Fatalf("clampJavaRootToProject(inner) = %q, want %q", got, inner)
	}
}

func TestLongestCommonAncestorDir_absolutePaths(t *testing.T) {
	a := "/home/user/parent/module-a/src/main/java"
	b := "/home/user/parent/module-b/src/main/java"
	want := "/home/user/parent"
	got := longestCommonAncestorDir(a, b)
	if got != want {
		t.Fatalf("longestCommonAncestorDir() = %q, want %q", got, want)
	}
}
