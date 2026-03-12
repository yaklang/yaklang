package aiskillloader

import (
	"testing"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestLoadedSkillForgeRoundTrip(t *testing.T) {
	skillMD := `---
name: demo-skill
description: Demo skill
compatibility: python3
metadata:
  category: automation
---
Use the helper script when needed.
`
	meta, err := ParseSkillMeta(skillMD)
	if err != nil {
		t.Fatalf("ParseSkillMeta failed: %v", err)
	}
	vfs := filesys.NewVirtualFs()
	vfs.AddFile(skillMDFilename, skillMD)
	vfs.AddFile("scripts/run.py", "print('hello')")

	loaded := &LoadedSkill{
		Meta:           meta,
		FileSystem:     vfs,
		SkillMDContent: skillMD,
	}

	forge, err := LoadedSkillToAIForge(loaded)
	if err != nil {
		t.Fatalf("LoadedSkillToAIForge failed: %v", err)
	}
	if forge.ForgeType != "skillmd" {
		t.Fatalf("unexpected forge type: %s", forge.ForgeType)
	}
	if len(forge.FSBytes) == 0 {
		t.Fatal("expected FSBytes to be populated")
	}

	restored, err := AIForgeToLoadedSkill(forge)
	if err != nil {
		t.Fatalf("AIForgeToLoadedSkill failed: %v", err)
	}
	if restored.Meta.Name != "demo-skill" {
		t.Fatalf("unexpected skill name: %s", restored.Meta.Name)
	}
	content, err := restored.FileSystem.ReadFile("scripts/run.py")
	if err != nil {
		t.Fatalf("failed to read restored script: %v", err)
	}
	if string(content) != "print('hello')" {
		t.Fatalf("unexpected restored script content: %q", string(content))
	}
	if restored.Meta.Body != meta.Body {
		t.Fatalf("unexpected body: %q", restored.Meta.Body)
	}
}

func TestBuildSkillSourceFSFromForges(t *testing.T) {
	skillMD := `---
name: demo-skill
description: Demo skill
metadata:
  category: automation
---
Use the helper script when needed.
`
	meta, err := ParseSkillMeta(skillMD)
	if err != nil {
		t.Fatalf("ParseSkillMeta failed: %v", err)
	}
	vfs := filesys.NewVirtualFs()
	vfs.AddFile(skillMDFilename, skillMD)
	vfs.AddFile("scripts/run.py", "print('hello')")
	forge, err := LoadedSkillToAIForge(&LoadedSkill{Meta: meta, FileSystem: vfs, SkillMDContent: skillMD})
	if err != nil {
		t.Fatalf("LoadedSkillToAIForge failed: %v", err)
	}

	rootFS, count, err := BuildSkillSourceFSFromForges([]*schema.AIForge{forge})
	if err != nil {
		t.Fatalf("BuildSkillSourceFSFromForges failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("unexpected skill count: %d", count)
	}
	loader, err := NewFSSkillLoader(rootFS)
	if err != nil {
		t.Fatalf("NewFSSkillLoader failed: %v", err)
	}
	loaded, err := loader.LoadSkill("demo-skill")
	if err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	content, err := loaded.FileSystem.ReadFile("scripts/run.py")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "print('hello')" {
		t.Fatalf("unexpected script content: %q", string(content))
	}
}
