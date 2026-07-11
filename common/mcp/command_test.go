package mcp

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestSyncCommandLineMCPCurrentProjectDatabase(t *testing.T) {
	oldProfileDB := consts.GetGormProfileDatabase()
	oldProjectDB := consts.GetGormProjectDatabase()
	oldProfilePath := consts.GetCurrentProfileDatabasePath()
	oldProjectPath := consts.GetCurrentProjectDatabasePath()
	t.Cleanup(func() {
		consts.BindProfileDatabase(oldProfileDB, oldProfilePath)
		consts.BindProjectDatabase(oldProjectDB, oldProjectPath)
	})

	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.db")
	defaultProjectPath := filepath.Join(dir, "default-project.db")
	currentProjectPath := filepath.Join(dir, "current-project.db")

	profileDB, err := consts.CreateProfileDatabase(profilePath)
	require.NoError(t, err)
	defaultProjectDB, err := consts.CreateProjectDatabase(defaultProjectPath)
	require.NoError(t, err)
	currentProjectDB, err := consts.CreateProjectDatabase(currentProjectPath)
	require.NoError(t, err)
	require.NoError(t, currentProjectDB.Close())

	consts.BindProfileDatabase(profileDB, profilePath)
	consts.BindProjectDatabase(defaultProjectDB, defaultProjectPath)

	require.NoError(t, profileDB.Create(&schema.Project{
		ProjectName:      yakit.INIT_DATABASE_RECORD_NAME,
		DatabasePath:     defaultProjectPath,
		FolderID:         yakit.FolderID,
		ChildFolderID:    yakit.ChildFolderID,
		Type:             yakit.TypeProject,
		IsCurrentProject: false,
	}).Error)
	require.NoError(t, profileDB.Create(&schema.Project{
		ProjectName:      "selected",
		DatabasePath:     currentProjectPath,
		FolderID:         yakit.FolderID,
		ChildFolderID:    yakit.ChildFolderID,
		Type:             yakit.TypeProject,
		IsCurrentProject: true,
	}).Error)

	require.NoError(t, syncCommandLineMCPProjectDatabase())
	require.Equal(t, currentProjectPath, consts.GetCurrentProjectDatabasePath())
}
