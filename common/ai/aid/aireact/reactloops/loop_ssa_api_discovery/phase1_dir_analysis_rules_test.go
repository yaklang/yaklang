package loop_ssa_api_discovery

import "testing"

func TestIsKnownThirdPartyDir_localeAndPlugins(t *testing.T) {
	cases := []struct {
		name string
		rel  string
		want bool
	}{
		{"ach", "publiccms/src/main/webapp/resource/plugins/pdfjs/locale/ach", true},
		{"plugins", "publiccms/src/main/webapp/resource/plugins", true},
		{"pdfjs", "publiccms/src/main/webapp/resource/plugins/pdfjs", true},
		{"tinymce", "publiccms/src/main/webapp/resource/plugins/tinymce", true},
		{"emoticons", "publiccms/src/main/webapp/resource/plugins/tinymce/plugins/emoticons", true},
		{"admin", "publiccms-core/src/main/java/com/publiccms/controller/admin", false},
	}
	for _, tc := range cases {
		if got := isKnownThirdPartyDir(tc.name, tc.rel); got != tc.want {
			t.Fatalf("isKnownThirdPartyDir(%q, %q) = %v, want %v", tc.name, tc.rel, got, tc.want)
		}
	}
}

func TestIsStaticAssetOnlyDir(t *testing.T) {
	node := &DirectoryNode{
		RelPath:   "publiccms/src/main/webapp/resource/plugins/pdfjs/locale/zh-TW",
		FileNames: []string{"viewer.ftl"},
	}
	if !isStaticAssetOnlyDir(node) {
		t.Fatal("viewer.ftl-only dir should be static asset")
	}
	mixed := &DirectoryNode{
		RelPath:   "publiccms-core/src/main/java/com/publiccms/controller/admin",
		FileNames: []string{"LoginAdminController.java"},
	}
	if isStaticAssetOnlyDir(mixed) {
		t.Fatal("java dir should not be static asset only")
	}
}

func TestTryProgrammaticDirAnalysis_moduleRootContinues(t *testing.T) {
	ctx := &ProjectContextSummaryV1{
		Summary: "PublicCMS 多模块 CMS",
		FirstPartyBoundary: ProjectCodeBoundary{
			ModuleRoots: []string{"publiccms-core", "publiccms-oauth"},
		},
	}
	node := &DirectoryNode{
		RelPath:   "publiccms-core",
		FileNames: []string{"pom.xml", "build.gradle"},
	}
	analysis, ok := tryProgrammaticDirAnalysis(node, ctx, "")
	if !ok || analysis == nil {
		t.Fatal("expected programmatic analysis for module root")
	}
	if analysis.BfsControl != BfsControlContinue {
		t.Fatalf("BfsControl = %q, want %q", analysis.BfsControl, BfsControlContinue)
	}
	if !analysis.IsBusiness {
		t.Fatal("module root should be business container")
	}
}

func TestTryProgrammaticDirAnalysis_localeStop(t *testing.T) {
	ctx := &ProjectContextSummaryV1{
		ThirdPartyBoundary: ProjectCodeBoundary{
			PathPatterns: []string{"**/pdfjs/locale/**"},
		},
	}
	node := &DirectoryNode{
		RelPath:   "publiccms/src/main/webapp/resource/plugins/pdfjs/locale/sk",
		FileNames: []string{"viewer.ftl"},
	}
	analysis, ok := tryProgrammaticDirAnalysis(node, ctx, "")
	if !ok || analysis == nil {
		t.Fatal("expected programmatic skip for locale dir")
	}
	if analysis.BfsControl != BfsControlStop {
		t.Fatalf("BfsControl = %q, want %q", analysis.BfsControl, BfsControlStop)
	}
}

func TestProjectContextMatchesThirdPartyPath(t *testing.T) {
	ctx := &ProjectContextSummaryV1{
		ThirdPartyBoundary: ProjectCodeBoundary{
			PathPatterns: []string{"**/webapp/resource/plugins/**"},
		},
	}
	if !ctx.matchesThirdPartyPath("publiccms/src/main/webapp/resource/plugins/pdfjs") {
		t.Fatal("expected plugins path to match third party boundary")
	}
	if ctx.matchesThirdPartyPath("publiccms-core/src/main/java/com/publiccms") {
		t.Fatal("java source should not match third party boundary")
	}
}

func TestCollectNextLevelIDs_skipsStoppedParent(t *testing.T) {
	tree := &DirectoryTreeV1{
		Nodes: []DirectoryNode{
			{ID: "root", RelPath: "", Depth: 0, Analysis: &DirAnalysis{BfsControl: BfsControlContinue}},
			{ID: "pdfjs", ParentID: "root", RelPath: "plugins/pdfjs", Depth: 1},
			{ID: "locale", ParentID: "pdfjs", RelPath: "plugins/pdfjs/locale", Depth: 2},
			{ID: "ach", ParentID: "locale", RelPath: "plugins/pdfjs/locale/ach", Depth: 3, FileNames: []string{"viewer.ftl"}},
			{ID: "core", ParentID: "root", RelPath: "publiccms-core", Depth: 1},
			{ID: "ctrl", ParentID: "core", RelPath: "publiccms-core/controller", Depth: 2},
		},
	}

	next := CollectNextLevelIDs(tree, []string{"root"})
	if len(next) != 2 {
		t.Fatalf("expected 2 children from root, got %d: %v", len(next), next)
	}

	for i := range tree.Nodes {
		if tree.Nodes[i].ID == "pdfjs" {
			tree.Nodes[i].Analysis = &DirAnalysis{BfsControl: BfsControlStop}
			break
		}
	}

	next = CollectNextLevelIDs(tree, []string{"pdfjs"})
	if len(next) != 0 {
		t.Fatalf("expected 0 children under stopped pdfjs, got %d: %v", len(next), next)
	}

	if !ShouldSkipDirAnalysis(tree, "ach") {
		t.Fatal("ach should be skipped under stopped ancestor")
	}
	if ShouldSkipDirAnalysis(tree, "ctrl") {
		t.Fatal("ctrl should not be skipped")
	}
}
