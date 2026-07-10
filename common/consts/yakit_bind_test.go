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
	t.Cleanup(func() {
		initYakitDatabaseOnce = oldOnce
		profileDatabase = oldProfileDB
		projectDataBase = oldProjectDB
		currentProfileDatabasePath = oldProfilePath
		currentProjectDatabasePath = oldProjectPath
		schema.SetGormProfileDatabase(oldProfileDB)
		schema.SetGormProjectDatabase(oldProjectDB)
	})

	initYakitDatabaseOnce = new(sync.Once)
	profileDatabase = nil
	projectDataBase = nil
	currentProfileDatabasePath = ""
	currentProjectDatabasePath = ""
}
