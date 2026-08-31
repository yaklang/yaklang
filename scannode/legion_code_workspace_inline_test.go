package scannode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegionInlineSourceWorkspaceMaterializesAndSanitizes(t *testing.T) {
	files := map[string]string{"src/main.yak": "println(\"private-source\")", "empty.yak": ""}
	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindInlineSources)
	spec.Locator, spec.InlineFiles = "", files
	spec.ExpectedSHA256 = legionInlineSourceDigest(files)
	spec.ExpectedRevision = spec.ExpectedSHA256
	raw, _ := json.Marshal(yakRuntimeOptions{SourceWorkspace: &spec})
	workspace, public, err := prepareLegionCodeWorkspace(context.Background(), raw, legionCodeWorkspaceMaterializeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspace.Cleanup() })
	if strings.Contains(string(public), "private-source") || strings.Contains(string(public), "inline_files") {
		t.Fatalf("private inline source leaked into public runtime options: %s", public)
	}
	if workspace.sha256 != spec.ExpectedSHA256 || workspace.lockedRevision != spec.ExpectedSHA256 || workspace.files != 2 {
		t.Fatalf("inline sources were not immutably locked: %#v", workspace)
	}
	read, err := workspace.read(map[string]any{"path": "src/main.yak"})
	if err != nil || read["content"] != files["src/main.yak"] {
		t.Fatalf("bound source read failed: %#v %v", read, err)
	}
	files["src/main.yak"] = "changed-by-caller"
	if workspace.inlineFiles["src/main.yak"] != "println(\"private-source\")" {
		t.Fatal("runtime retained mutable caller-owned source map")
	}
	info, err := os.Stat(filepath.Join(workspace.root, "src/main.yak"))
	if err != nil || info.Mode().Perm()&0222 != 0 {
		t.Fatalf("inline source file is writable: %v %v", info, err)
	}
	if err := validateLegionCodeWorkspaceContextPin(raw, public); err != nil {
		t.Fatalf("sanitized public context lost immutable source pin: %v", err)
	}
	if err := validateLegionCodeWorkspaceContextPin(raw, raw); err == nil {
		t.Fatal("private file bytes accepted from replay context")
	}
	root := workspace.root
	if err := workspace.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("private workspace not removed: %v", err)
	}
}

func TestLegionInlineSourceWorkspaceAllowsRequirementsOnly(t *testing.T) {
	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindInlineSources)
	spec.Locator = ""
	workspace, err := materializeLegionInlineWorkspace(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()
	if workspace.files != 0 || workspace.bytes != 0 || workspace.sha256 != legionRuleHash("") {
		t.Fatalf("requirements-only workspace invalid: %#v", workspace)
	}
	list, err := workspace.list(nil)
	if err != nil || list["count"] != 0 {
		t.Fatalf("empty workspace list: %#v %v", list, err)
	}
}

func TestLegionInlineSourceWorkspaceRejectsInvalidAndMixedSources(t *testing.T) {
	for _, path := range []string{"../a.yak", "/a.yak", "C:/a.yak", "a\\b.yak", "./a.yak", "a/../b.yak", " a.yak", ".git/config", "a\nb.yak", strings.Repeat("a", legionInlineSourceMaxPathBytes+1)} {
		t.Run(path, func(t *testing.T) {
			if err := validateLegionInlineFiles(map[string]string{path: "1"}, false); err == nil {
				t.Fatalf("unsafe inline path accepted: %q", path)
			}
		})
	}
	for _, files := range []map[string]string{
		{"a.yak": strings.Repeat("x", legionInlineSourceMaxFileBytes+1)},
		{"a.yak": "a\x00b"},
		{"a.yak": string([]byte{0xff})},
		{"dir": "1", "dir/a.yak": "2"},
		{"A.yak": "1", "a.yak": "2"},
	} {
		if err := validateLegionInlineFiles(files, false); err == nil {
			t.Fatal("invalid source content or colliding paths accepted")
		}
	}
	oversized := map[string]string{}
	for i := 0; i < 5; i++ {
		oversized[string(rune('a'+i))+".yak"] = strings.Repeat("x", legionInlineSourceMaxFileBytes)
	}
	if err := validateLegionInlineFiles(oversized, false); err == nil {
		t.Fatal("total source budget not enforced")
	}
	tooMany := map[string]string{}
	for i := 0; i <= legionInlineSourceMaxFiles; i++ {
		tooMany[string(rune('a'+i))+".yak"] = ""
	}
	if err := validateLegionInlineFiles(tooMany, false); err == nil {
		t.Fatal("file count budget not enforced")
	}
	for _, mutate := range []func(*legionCodeWorkspaceSpec){
		func(s *legionCodeWorkspaceSpec) { s.Locator = "https://source.invalid/repo" },
		func(s *legionCodeWorkspaceSpec) { s.PayloadID = "managed-payload" },
		func(s *legionCodeWorkspaceSpec) { s.Branch = "main" },
		func(s *legionCodeWorkspaceSpec) { s.Subpath = "src" },
		func(s *legionCodeWorkspaceSpec) { s.Auth = &legionCodeWorkspaceAuth{Token: "private"} },
		func(s *legionCodeWorkspaceSpec) { s.Proxy = &legionCodeWorkspaceProxy{URL: "http://proxy.invalid"} },
		func(s *legionCodeWorkspaceSpec) { s.ExpectedSHA256 = strings.Repeat("0", 64) },
		func(s *legionCodeWorkspaceSpec) { s.ExpectedRevision = "forged-revision" },
		func(s *legionCodeWorkspaceSpec) { s.ReadOnly = false },
	} {
		spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindInlineSources)
		spec.Locator = ""
		mutate(&spec)
		if err := normalizeLegionCodeWorkspaceSpec(&spec); err == nil {
			t.Fatalf("mixed/forged inline workspace accepted: %#v", spec)
		}
	}
}

func TestLegionInlineSamplesDoNotChangeGitWorkspaceAuthority(t *testing.T) {
	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
	spec.InlineFiles = map[string]string{"sample.yak": "println(\"private-user-sample\")"}
	if err := normalizeLegionCodeWorkspaceSpec(&spec); err != nil {
		t.Fatal(err)
	}
	public := publicLegionCodeWorkspaceSpec(spec)
	if public.Locator != spec.Locator || public.Kind != legionCodeWorkspaceKindGit || public.InlineFiles != nil {
		t.Fatalf("sample metadata altered Git authority or leaked: %#v", public)
	}
}
