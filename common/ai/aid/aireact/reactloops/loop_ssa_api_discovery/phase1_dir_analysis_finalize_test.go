package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPropagateStoppedSubtrees_clearsUnvisitedLeaves(t *testing.T) {
	tree := &DirectoryTreeV1{
		Nodes: []DirectoryNode{
			{ID: "webapp", RelPath: "publiccms/src/main/webapp", Depth: 3, Analysis: &DirAnalysis{
				BfsControl: BfsControlStop, IsBusiness: false, FunctionDesc: "web static",
				DepInfo: &DepInfo{Name: "Webapp"},
			}},
			{ID: "locale", ParentID: "webapp", RelPath: "publiccms/src/main/webapp/resource/plugins/pdfjs/locale", Depth: 7, Analysis: &DirAnalysis{BfsControl: BfsControlLeaf}},
			{ID: "ach", ParentID: "locale", RelPath: "publiccms/src/main/webapp/resource/plugins/pdfjs/locale/ach", Depth: 8, FileNames: []string{"viewer.ftl"}},
			{ID: "core", RelPath: "publiccms-core/src/main/java/com/publiccms/controller", Depth: 7, Analysis: &DirAnalysis{
				BfsControl: BfsControlContinue, IsBusiness: true, FunctionDesc: "controllers",
			}},
		},
	}
	propagateStoppedSubtrees(tree, nil, t.TempDir())
	ach := tree.GetNode("ach")
	if ach == nil || ach.Analysis == nil {
		t.Fatal("ach should have inherited analysis")
	}
	if ach.Analysis.BfsControl != BfsControlStop {
		t.Fatalf("ach BfsControl = %q, want stop", ach.Analysis.BfsControl)
	}
	if ach.Analysis.DepInfo == nil {
		t.Fatal("ach should inherit dependency_info")
	}
	core := tree.GetNode("core")
	if core == nil || core.Analysis == nil || core.Analysis.BfsControl != BfsControlContinue {
		t.Fatal("analyzed business node should be preserved")
	}
}

func TestSanitizeDirAnalysis_firstPartyJavaNeverStop(t *testing.T) {
	ctx := &ProjectContextSummaryV1{
		FirstPartyBoundary: ProjectCodeBoundary{
			ModuleRoots:  []string{"publiccms-common"},
			PackageRoots: []string{"com.publiccms"},
		},
	}
	node := &DirectoryNode{RelPath: "publiccms-common/src/main/java/com/publiccms/common/base"}
	analysis := &DirAnalysis{
		BfsControl:   BfsControlStop,
		IsBusiness:   false,
		FunctionDesc: "utility only",
		DepInfo:      &DepInfo{Name: "publiccms-common", Version: "unknown"},
	}
	out := sanitizeDirAnalysis(node, ctx, "", analysis)
	if out.BfsControl != BfsControlContinue {
		t.Fatalf("BfsControl = %q, want continue", out.BfsControl)
	}
	if !out.IsBusiness {
		t.Fatal("first-party java should remain business")
	}
	if out.DepInfo != nil {
		t.Fatal("first-party java should not have dependency_info")
	}
}

func TestSanitizeDirAnalysis_vendoredJavaCanStop(t *testing.T) {
	ctx := &ProjectContextSummaryV1{
		FirstPartyBoundary: ProjectCodeBoundary{PackageRoots: []string{"com.publiccms"}},
	}
	node := &DirectoryNode{RelPath: "publiccms-analyzer/src/main/java/com/google/typography/font/sfntly"}
	analysis := &DirAnalysis{
		BfsControl:   BfsControlStop,
		IsBusiness:   false,
		FunctionDesc: "Google sfntly",
		DepInfo:      &DepInfo{Name: "sfntly", Version: "unknown"},
	}
	out := sanitizeDirAnalysis(node, ctx, "", analysis)
	if out.BfsControl != BfsControlStop {
		t.Fatalf("BfsControl = %q, want stop", out.BfsControl)
	}
	if out.DepInfo == nil {
		t.Fatal("vendored java should keep dependency_info")
	}
	if out.DepInfo.Version != "" {
		t.Fatalf("unknown version should be cleared, got %q", out.DepInfo.Version)
	}
}

func TestInferDependencyInfo_packageJson(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"pdfjs-dist","version":"2.16.105"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	node := &DirectoryNode{RelPath: ".", FileNames: []string{"viewer.js"}}
	info := inferDependencyInfo(node, dir)
	if info == nil || info.Name != "pdfjs-dist" {
		t.Fatalf("name = %v, want pdfjs-dist", info)
	}
	if info.Version != "2.16.105" {
		t.Fatalf("version = %q, want 2.16.105", info.Version)
	}
}

func TestMarkVisitedLeaves_onlyAfterBFS(t *testing.T) {
	tree := &DirectoryTreeV1{
		Nodes: []DirectoryNode{
			{ID: "parent", RelPath: "mod/src/main/java/com/app", Analysis: &DirAnalysis{BfsControl: BfsControlContinue, IsBusiness: true, FunctionDesc: "pkg"}},
			{ID: "leaf", ParentID: "parent", RelPath: "mod/src/main/java/com/app/service", Analysis: &DirAnalysis{BfsControl: BfsControlContinue, IsBusiness: true, FunctionDesc: "svc"}},
			{ID: "unvisited", RelPath: "mod/webapp/static", Analysis: nil},
		},
	}
	markVisitedLeaves(tree)
	unvisited := tree.GetNode("unvisited")
	if unvisited != nil && unvisited.Analysis != nil {
		t.Fatal("unvisited node should stay without analysis")
	}
	parent := tree.GetNode("parent")
	if parent == nil || parent.Analysis == nil || parent.Analysis.BfsControl != BfsControlContinue {
		t.Fatal("parent with children should remain continue")
	}
	leaf := tree.GetNode("leaf")
	if leaf == nil || leaf.Analysis == nil || leaf.Analysis.BfsControl != BfsControlLeaf {
		t.Fatalf("visited leaf node should become bfs:leaf, got %+v", leaf.Analysis)
	}
}

func TestBuildDirectoryTree_noPrematureLeafMark(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "mod", "src", "main", "java", "com", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "Main.java"), []byte("package com.app;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tree := BuildDirectoryTree(root)
	if tree == nil {
		t.Fatal("nil tree")
	}
	for _, n := range tree.Nodes {
		if n.Analysis != nil {
			t.Fatalf("node %q should not have pre-BFS analysis", n.RelPath)
		}
	}
}
