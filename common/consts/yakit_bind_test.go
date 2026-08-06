package consts

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
)

func TestBindProfileDatabaseBeforeLazyInitTakesPrecedence(t *testing.T) {
	resetYakitDatabaseForBindTest(t)

	dir := t.TempDir()
	t.Setenv(CONST_YAK_DEFAULT_PROFILE_DATABASE_NAME, filepath.Join(dir, "default-profile.db"))
	t.Setenv(CONST_YAK_DEFAULT_PROJECT_DATABASE_NAME, filepath.Join(dir, "default-project.db"))

	boundPath := filepath.Join(dir, "bound-profile.db")
	boundDB, err := CreateProfileDatabase(boundPath)
	if err != nil {
		t.Fatalf("create bound profile db: %v", err)
	}
	BindProfileDatabase(boundDB, boundPath)

	got := GetGormProfileDatabase()
	if got != boundDB {
		t.Fatalf("GetGormProfileDatabase returned lazy default DB, want explicitly bound DB")
	}
	if currentProfileDatabasePath != boundPath {
		t.Fatalf("currentProfileDatabasePath = %q, want %q", currentProfileDatabasePath, boundPath)
	}
}

func TestLazyProjectInitDoesNotOverwriteBoundProfileDatabase(t *testing.T) {
	resetYakitDatabaseForBindTest(t)

	dir := t.TempDir()
	t.Setenv(CONST_YAK_DEFAULT_PROFILE_DATABASE_NAME, filepath.Join(dir, "default-profile.db"))
	t.Setenv(CONST_YAK_DEFAULT_PROJECT_DATABASE_NAME, filepath.Join(dir, "default-project.db"))

	boundPath := filepath.Join(dir, "bound-profile.db")
	boundDB, err := CreateProfileDatabase(boundPath)
	if err != nil {
		t.Fatalf("create bound profile db: %v", err)
	}
	BindProfileDatabase(boundDB, boundPath)

	if gotProject := GetGormProjectDatabase(); gotProject == nil {
		t.Fatal("project database should lazy initialize")
	}
	if gotProfile := GetGormProfileDatabase(); gotProfile != boundDB {
		t.Fatalf("lazy project init overwrote bound profile DB")
	}
	if currentProfileDatabasePath != boundPath {
		t.Fatalf("currentProfileDatabasePath = %q, want %q", currentProfileDatabasePath, boundPath)
	}
}

func TestLazyProfileInitDoesNotOverwriteBoundProjectDatabase(t *testing.T) {
	resetYakitDatabaseForBindTest(t)

	dir := t.TempDir()
	t.Setenv(CONST_YAK_DEFAULT_PROFILE_DATABASE_NAME, filepath.Join(dir, "default-profile.db"))
	t.Setenv(CONST_YAK_DEFAULT_PROJECT_DATABASE_NAME, filepath.Join(dir, "default-project.db"))

	boundPath := filepath.Join(dir, "bound-project.db")
	boundDB, err := CreateProjectDatabase(boundPath)
	if err != nil {
		t.Fatalf("create bound project db: %v", err)
	}
	BindProjectDatabase(boundDB, boundPath)

	if gotProfile := GetGormProfileDatabase(); gotProfile == nil {
		t.Fatal("profile database should lazy initialize")
	}
	if gotProject := GetGormProjectDatabase(); gotProject != boundDB {
		t.Fatalf("lazy profile init overwrote bound project DB")
	}
	if currentProjectDatabasePath != boundPath {
		t.Fatalf("currentProjectDatabasePath = %q, want %q", currentProjectDatabasePath, boundPath)
	}
}

func resetYakitDatabaseForBindTest(t *testing.T) {
	t.Helper()
	oldOnce := initYakitDatabaseOnce
	oldProfileDB := profileDatabase
	oldProjectDB := projectDataBase
	oldProfilePath := currentProfileDatabasePath
	oldProjectPath := currentProjectDatabasePath
	oldProjectBinding := projectDatabaseBinding.Load()
	t.Cleanup(func() {
		initYakitDatabaseOnce = oldOnce
		profileDatabase = oldProfileDB
		projectDataBase = oldProjectDB
		currentProfileDatabasePath = oldProfilePath
		currentProjectDatabasePath = oldProjectPath
		projectDatabaseBinding.Store(oldProjectBinding)
		schema.SetGormProfileDatabase(oldProfileDB)
		schema.SetGormProjectDatabase(oldProjectDB)
	})

	initYakitDatabaseOnce = new(sync.Once)
	profileDatabase = nil
	projectDataBase = nil
	currentProfileDatabasePath = ""
	currentProjectDatabasePath = ""
	projectDatabaseBinding.Store(nil)
}

func TestProjectDatabaseBindingGenerationChangesOnRebind(t *testing.T) {
	resetYakitDatabaseForBindTest(t)

	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.db")
	firstDB, err := CreateProjectDatabase(firstPath)
	if err != nil {
		t.Fatalf("create first project db: %v", err)
	}
	BindProjectDatabase(firstDB, firstPath)
	first := CaptureProjectDatabaseBinding()
	if first.Database != firstDB || first.Path != firstPath || first.Generation == 0 {
		t.Fatalf("unexpected first binding: %#v", first)
	}

	secondPath := filepath.Join(dir, "second.db")
	secondDB, err := CreateProjectDatabase(secondPath)
	if err != nil {
		t.Fatalf("create second project db: %v", err)
	}
	BindProjectDatabase(secondDB, secondPath)
	second := CaptureProjectDatabaseBinding()
	if second.Database != secondDB || second.Path != secondPath || second.Generation <= first.Generation {
		t.Fatalf("unexpected second binding: first=%#v second=%#v", first, second)
	}
}

func TestAdvanceProjectDatabaseGenerationKeepsHandlesAndRejectsStaleCaller(t *testing.T) {
	resetYakitDatabaseForBindTest(t)

	projectPath := filepath.Join(t.TempDir(), "project.db")
	projectDB, err := CreateProjectDatabase(projectPath)
	if err != nil {
		t.Fatalf("create project db: %v", err)
	}
	BindProjectDatabase(projectDB, projectPath)
	first := CaptureProjectDatabaseBinding()

	second, advanced := AdvanceProjectDatabaseGeneration(first.Generation)
	if !advanced {
		t.Fatal("expected current generation to advance")
	}
	if second.Database != first.Database || second.ReadDatabase != first.ReadDatabase || second.Path != first.Path {
		t.Fatalf("advancing generation changed database handles: first=%#v second=%#v", first, second)
	}
	if second.Generation <= first.Generation {
		t.Fatalf("generation did not advance: first=%d second=%d", first.Generation, second.Generation)
	}

	current, advanced := AdvanceProjectDatabaseGeneration(first.Generation)
	if advanced {
		t.Fatal("stale generation unexpectedly advanced the current project")
	}
	if current.Generation != second.Generation {
		t.Fatalf("stale caller changed generation: current=%d want=%d", current.Generation, second.Generation)
	}
}
