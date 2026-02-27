package aiskillloader

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/filesys"
)

func buildPersistenceTestVFS() *filesys.VirtualFS {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("recon/SKILL.md", buildTestSkillMD(
		"recon",
		"Reconnaissance and information gathering",
		"# Recon Skill\n\nGathering intelligence about targets.\n",
	))
	vfs.AddFile("recon/osint.md", "# OSINT Guide\n")
	vfs.AddFile("exploit/SKILL.md", buildTestSkillMD(
		"exploit",
		"Exploitation techniques",
		"# Exploit Skill\n\nVulnerability exploitation.\n",
	))
	vfs.AddFile("privesc/SKILL.md", buildTestSkillMD(
		"privesc",
		"Privilege escalation",
		"# Privesc Skill\n\nEscalating privileges.\n",
	))
	return vfs
}

func TestSkillsContextManager_LoadAndExtractNames(t *testing.T) {
	vfs := buildPersistenceTestVFS()
	loader, err := NewFSSkillLoader(vfs)
	if err != nil {
		t.Fatalf("failed to create loader: %v", err)
	}
	mgr := NewSkillsContextManager(loader)

	if err := mgr.LoadSkill("recon"); err != nil {
		t.Fatalf("failed to load recon: %v", err)
	}
	if err := mgr.LoadSkill("exploit"); err != nil {
		t.Fatalf("failed to load exploit: %v", err)
	}

	selected := mgr.GetCurrentSelectedSkills()
	if len(selected) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(selected))
	}

	names := make(map[string]bool)
	for _, s := range selected {
		names[s.Name] = true
	}
	if !names["recon"] || !names["exploit"] {
		t.Errorf("expected recon and exploit in loaded skills, got %v", names)
	}
}

func TestSkillsContextManager_RestoreFromNames(t *testing.T) {
	vfs := buildPersistenceTestVFS()

	// Simulate first session: load skills
	loader1, _ := NewFSSkillLoader(vfs)
	mgr1 := NewSkillsContextManager(loader1)
	mgr1.LoadSkill("recon")
	mgr1.LoadSkill("exploit")

	// Extract names (simulating what we persist)
	selected := mgr1.GetCurrentSelectedSkills()
	var savedNames []string
	for _, s := range selected {
		savedNames = append(savedNames, s.Name)
	}
	serialized := strings.Join(savedNames, ",")

	// Simulate second session: create new manager, restore from names
	loader2, _ := NewFSSkillLoader(vfs)
	mgr2 := NewSkillsContextManager(loader2)

	// Verify skills are NOT loaded initially
	if len(mgr2.GetCurrentSelectedSkills()) != 0 {
		t.Fatal("new manager should have no loaded skills")
	}

	// Restore
	restoredNames := strings.Split(serialized, ",")
	results := mgr2.LoadSkills(restoredNames)
	for name, err := range results {
		if err != nil {
			t.Errorf("failed to restore skill %q: %v", name, err)
		}
	}

	// Verify restoration
	restoredSelected := mgr2.GetCurrentSelectedSkills()
	if len(restoredSelected) != 2 {
		t.Fatalf("expected 2 restored skills, got %d", len(restoredSelected))
	}

	restoredMap := make(map[string]bool)
	for _, s := range restoredSelected {
		restoredMap[s.Name] = true
	}
	if !restoredMap["recon"] || !restoredMap["exploit"] {
		t.Errorf("expected recon and exploit to be restored, got %v", restoredMap)
	}

	// Verify skills are unfolded (active)
	if !mgr2.IsSkillLoadedAndUnfolded("recon") {
		t.Error("recon should be loaded and unfolded after restore")
	}
	if !mgr2.IsSkillLoadedAndUnfolded("exploit") {
		t.Error("exploit should be loaded and unfolded after restore")
	}
}

func TestSkillsContextManager_RestorePartialFailure(t *testing.T) {
	vfs := buildPersistenceTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	// Try restoring with one valid and one invalid skill name
	results := mgr.LoadSkills([]string{"recon", "nonexistent-skill"})

	if results["recon"] != nil {
		t.Errorf("recon should load successfully, got: %v", results["recon"])
	}
	if results["nonexistent-skill"] == nil {
		t.Error("nonexistent-skill should fail to load")
	}

	// recon should still be loaded
	if !mgr.IsSkillLoaded("recon") {
		t.Error("recon should be loaded despite nonexistent-skill failure")
	}
}

func TestSkillsContextManager_RestoreMultipleSkills_ContextFits(t *testing.T) {
	vfs := buildPersistenceTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	results := mgr.LoadSkills([]string{"recon", "exploit", "privesc"})
	for name, err := range results {
		if err != nil {
			t.Errorf("failed to load skill %q: %v", name, err)
		}
	}

	selected := mgr.GetCurrentSelectedSkills()
	if len(selected) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(selected))
	}
}

func TestSkillsContextManager_SerializeDeserializeNames(t *testing.T) {
	names := []string{"recon", "exploit", "privesc"}
	serialized := strings.Join(names, ",")

	deserialized := strings.Split(serialized, ",")
	if len(deserialized) != 3 {
		t.Fatalf("expected 3 names after deserialization, got %d", len(deserialized))
	}
	for i, n := range deserialized {
		if n != names[i] {
			t.Errorf("name mismatch at index %d: expected %q, got %q", i, names[i], n)
		}
	}
}

func TestSkillsContextManager_EmptyNamesRestore(t *testing.T) {
	vfs := buildPersistenceTestVFS()
	loader, _ := NewFSSkillLoader(vfs)
	mgr := NewSkillsContextManager(loader)

	results := mgr.LoadSkills([]string{})
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty names, got %d", len(results))
	}

	results = mgr.LoadSkills([]string{"", "  "})
	if len(results) != 0 {
		t.Errorf("expected 0 results for blank names, got %d", len(results))
	}
}
