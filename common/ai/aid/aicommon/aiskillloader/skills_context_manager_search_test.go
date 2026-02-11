package aiskillloader

import (
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func newSearchTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory DB: %v", err)
	}
	db.AutoMigrate(&schema.AISkill{})
	return db
}

func newSearchTestManager(t *testing.T) *SkillsContextManager {
	t.Helper()
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(buildNestedTestVFS()))
	if err != nil {
		t.Fatalf("failed to create autoloader: %v", err)
	}
	return NewSkillsContextManager(loader)
}

func TestManager_ListSkills(t *testing.T) {
	m := newSearchTestManager(t)
	skills, err := m.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills failed: %v", err)
	}
	if len(skills) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(skills))
	}
}

func TestManager_SearchSkills(t *testing.T) {
	m := newSearchTestManager(t)
	results, err := m.SearchSkills("deploy")
	if err != nil {
		t.Fatalf("SearchSkills failed: %v", err)
	}
	if len(results) != 1 || results[0].Name != "top-skill" {
		t.Fatalf("unexpected search results: %+v", results)
	}
}

func TestManager_SearchKeywordBM25_InMemory(t *testing.T) {
	m := newSearchTestManager(t)
	results, err := m.SearchKeywordBM25("security scanning", 5)
	if err != nil {
		t.Fatalf("SearchKeywordBM25 failed: %v", err)
	}
	found := false
	for _, r := range results {
		if r.Name == "hidden-skill" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected hidden-skill in BM25 results, got %+v", results)
	}
}

func TestManager_SearchKeywordBM25_WithDB(t *testing.T) {
	db := newSearchTestDB(t)
	defer db.Close()
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(buildNestedTestVFS()))
	if err != nil {
		t.Fatalf("failed to create autoloader: %v", err)
	}
	m := NewSkillsContextManager(loader, WithManagerDB(db))

	results, err := m.SearchKeywordBM25("code review linters", 5)
	if err != nil {
		t.Fatalf("SearchKeywordBM25 failed: %v", err)
	}
	found := false
	for _, r := range results {
		if r.Name == "code-review" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected code-review in BM25 results, got %+v", results)
	}
}

func TestManager_SearchByAI_Mock(t *testing.T) {
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(buildNestedTestVFS()))
	if err != nil {
		t.Fatalf("failed to create autoloader: %v", err)
	}
	called := false
	m := NewSkillsContextManager(loader, WithManagerSearchAICallback(func(prompt, schema string) ([]SkillSelection, error) {
		called = true
		return []SkillSelection{
			{SkillName: "top-skill", Reason: "deploy task"},
		}, nil
	}))
	results, err := m.SearchByAI("deploy app to prod")
	if err != nil {
		t.Fatalf("SearchByAI failed: %v", err)
	}
	if !called {
		t.Fatal("AI callback should be called")
	}
	if len(results) != 1 || results[0].Name != "top-skill" {
		t.Fatalf("unexpected AI search results: %+v", results)
	}
}

func TestManager_GetCurrentSelectedSkills(t *testing.T) {
	m := newSearchTestManager(t)
	if err := m.LoadSkill("top-skill"); err != nil {
		t.Fatalf("LoadSkill failed: %v", err)
	}
	selected := m.GetCurrentSelectedSkills()
	if len(selected) != 1 || selected[0].Name != "top-skill" {
		t.Fatalf("unexpected selected skills: %+v", selected)
	}
}

func TestManager_DBPersistenceOnInit(t *testing.T) {
	db := newSearchTestDB(t)
	defer db.Close()
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(buildNestedTestVFS()))
	if err != nil {
		t.Fatalf("failed to create autoloader: %v", err)
	}
	_ = NewSkillsContextManager(loader, WithManagerDB(db))

	skill, err := yakit.GetAISkillByName(db, "top-skill")
	if err != nil {
		t.Fatalf("expected top-skill persisted in DB: %v", err)
	}
	if skill.Hash == "" {
		t.Fatal("expected hash to be persisted")
	}
}

func TestManager_DBHashDedup(t *testing.T) {
	db := newSearchTestDB(t)
	defer db.Close()
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(buildNestedTestVFS()))
	if err != nil {
		t.Fatalf("failed to create autoloader: %v", err)
	}
	_ = NewSkillsContextManager(loader, WithManagerDB(db))
	skill1, err := yakit.GetAISkillByName(db, "top-skill")
	if err != nil {
		t.Fatalf("first lookup failed: %v", err)
	}
	updatedAt1 := skill1.UpdatedAt
	time.Sleep(10 * time.Millisecond)

	// Re-initialize with same loader content; hash dedup should avoid updates.
	_ = NewSkillsContextManager(loader, WithManagerDB(db))
	skill2, err := yakit.GetAISkillByName(db, "top-skill")
	if err != nil {
		t.Fatalf("second lookup failed: %v", err)
	}
	if !skill2.UpdatedAt.Equal(updatedAt1) {
		t.Fatalf("expected UpdatedAt unchanged by hash dedup; got %v -> %v", updatedAt1, skill2.UpdatedAt)
	}
}
