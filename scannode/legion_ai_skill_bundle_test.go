package scannode

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiengine"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const skillBundleTestContent = "---\nname: example-skill\ndescription: Test imported SkillMD\n---\n# Instructions\nUse the supplied resource.\n"

func testSkillBundle(t *testing.T, entries map[string]string) *aiv1.ContextSkillBundle {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	raw := out.Bytes()
	digest := sha256.Sum256(raw)
	return &aiv1.ContextSkillBundle{
		ForgeId: "forge-1", OwnerUserId: "user-1", Name: "example-skill",
		ArchiveZip: raw, Sha256: hex.EncodeToString(digest[:]),
	}
}

func TestContextSkillBundleFSLoadsOriginalArchiveIntoNativeLoader(t *testing.T) {
	bundle := testSkillBundle(t, map[string]string{
		"SKILL.md":                skillBundleTestContent,
		"references/checklist.md": "original reference bytes\n",
	})
	vfs, err := contextSkillBundleFS(bundle, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true), aicommon.WithSkillsFS(vfs))
	loader := cfg.GetSkillLoader()
	if loader == nil || len(loader.AllSkillMetas()) != 1 {
		t.Fatalf("native Skill loader = %v", loader)
	}
	loaded, err := loader.LoadSkill("example-skill")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SkillMDContent != skillBundleTestContent {
		t.Fatal("SKILL.md bytes were changed")
	}
	resource, err := loaded.FileSystem.ReadFile("references/checklist.md")
	if err != nil || string(resource) != "original reference bytes\n" {
		t.Fatalf("resource = %q, %v", resource, err)
	}
}

func TestContextSkillBundleFSRejectsInvalidArchives(t *testing.T) {
	base := map[string]string{"SKILL.md": skillBundleTestContent}
	tooMany := map[string]string{"SKILL.md": skillBundleTestContent}
	for i := 0; i < maxContextSkillEntries; i++ {
		tooMany[fmt.Sprintf("references/%03d.md", i)] = "x"
	}
	var symlinkZIP bytes.Buffer
	symlinkWriter := zip.NewWriter(&symlinkZIP)
	skillEntry, err := symlinkWriter.Create("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = skillEntry.Write([]byte(skillBundleTestContent))
	symlinkHeader := &zip.FileHeader{Name: "references/link"}
	symlinkHeader.SetMode(os.ModeSymlink | 0777)
	symlinkEntry, err := symlinkWriter.CreateHeader(symlinkHeader)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = symlinkEntry.Write([]byte("/etc/passwd"))
	if err := symlinkWriter.Close(); err != nil {
		t.Fatal(err)
	}
	symlinkDigest := sha256.Sum256(symlinkZIP.Bytes())
	symlinkBundle := &aiv1.ContextSkillBundle{ForgeId: "forge-1", OwnerUserId: "user-1", Name: "example-skill", ArchiveZip: symlinkZIP.Bytes(), Sha256: hex.EncodeToString(symlinkDigest[:])}
	cases := []struct {
		name   string
		bundle *aiv1.ContextSkillBundle
		owner  string
		want   string
	}{
		{"owner mismatch", testSkillBundle(t, base), "user-2", "owner mismatch"},
		{"SHA mismatch", testSkillBundle(t, base), "user-1", "SHA-256 mismatch"},
		{"traversal", testSkillBundle(t, map[string]string{"SKILL.md": skillBundleTestContent, "../secret": "bad"}), "user-1", "unsafe path"},
		{"no skill", testSkillBundle(t, map[string]string{"README.md": "none"}), "user-1", "no SKILL.md"},
		{"two skills", testSkillBundle(t, map[string]string{"SKILL.md": skillBundleTestContent, "other/SKILL.md": skillBundleTestContent}), "user-1", "exactly one"},
		{"name mismatch", testSkillBundle(t, map[string]string{"SKILL.md": strings.Replace(skillBundleTestContent, "example-skill", "other-skill", 1)}), "user-1", "name does not match"},
		{"oversized entry", testSkillBundle(t, map[string]string{"SKILL.md": skillBundleTestContent, "large.bin": strings.Repeat("x", maxContextSkillEntryBytes+1)}), "user-1", "expanded size"},
		{"too many entries", testSkillBundle(t, tooMany), "user-1", "invalid entries"},
		{"symlink", symlinkBundle, "user-1", "nonregular entry"},
	}
	cases[1].bundle.Sha256 = strings.Repeat("0", 64)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := contextSkillBundleFS(tc.bundle, tc.owner)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want %q", err, tc.want)
			}
		})
	}
}

func TestStatelessSkillBundlePreservesOptionsAndRejectsForgeMix(t *testing.T) {
	bundle := testSkillBundle(t, map[string]string{"SKILL.md": skillBundleTestContent})
	handle := &statelessAIEngineRuntimeHandle{
		binding: aiSessionBinding{Ref: aiSessionCommandRef{OwnerUserID: "user-1"}},
		cachedOptions: []aiengine.AIEngineConfigOption{
			aiengine.WithExtOptions(aicommon.WithUserPresetPrompt("existing provider preset")),
		},
		newEngine: func(options ...aiengine.AIEngineConfigOption) (statelessTurnEngine, error) {
			engineConfig := aiengine.NewAIEngineConfig(options...)
			commonConfig := aicommon.NewConfig(context.Background(), engineConfig.ExtOptions...)
			if commonConfig.UserPresetPrompt != "existing provider preset" {
				t.Fatalf("prior ExtOptions dropped: %q", commonConfig.UserPresetPrompt)
			}
			if !commonConfig.IsAutoSkillsDisabled() {
				t.Fatal("node-local Skill discovery was not disabled")
			}
			loader := commonConfig.GetSkillLoader()
			if loader == nil || len(loader.AllSkillMetas()) != 1 || loader.AllSkillMetas()[0].Name != "example-skill" {
				t.Fatalf("turn Skill loader = %v", loader)
			}
			return nil, errFakeEngineFactory
		},
	}
	err := handle.SendInput(context.Background(), aiSessionInput{ContextPackage: &aiv1.ContextPackage{
		UserInput: "Use example skill", SkillBundles: []*aiv1.ContextSkillBundle{bundle},
	}})
	if err == nil || !strings.Contains(err.Error(), errFakeEngineFactory.Error()) {
		t.Fatalf("engine factory error = %v", err)
	}
	for _, pkg := range []*aiv1.ContextPackage{
		{UserInput: "test", SkillBundles: []*aiv1.ContextSkillBundle{bundle, bundle}},
		{UserInput: "test", SkillBundles: []*aiv1.ContextSkillBundle{bundle}, ForgeRelease: &aiv1.ContextForgeRelease{}},
		{UserInput: "test", SkillBundles: []*aiv1.ContextSkillBundle{bundle}, FocusRelease: &aiv1.ContextFocusRelease{}},
	} {
		err := handle.SendInput(context.Background(), aiSessionInput{ContextPackage: pkg})
		if err == nil {
			t.Fatal("conflicting Skill context was accepted")
		}
	}
}
