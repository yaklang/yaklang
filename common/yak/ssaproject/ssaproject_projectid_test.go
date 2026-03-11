package ssaproject

import (
	"encoding/json"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestSSAProject_SaveToDB_PersistProjectIDInConfig(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.AutoMigrate(&schema.SSAProject{}).Error)

	project, err := NewSSAProject(
		ssaconfig.WithProjectName("projectid-test"),
		ssaconfig.WithProjectDescription("desc"),
		ssaconfig.WithProjectLanguage(ssaconfig.JAVA),
		ssaconfig.WithCodeSourceLocalFile("/tmp/projectid-test"),
	)
	require.NoError(t, err)

	require.NoError(t, project.SaveToDB(db))
	require.NotNil(t, project.SSAProject)
	require.NotZero(t, project.ID)
	originalID := uint64(project.ID)

	var saved schema.SSAProject
	require.NoError(t, db.First(&saved, uint(originalID)).Error)
	var stored ssaconfig.Config
	require.NoError(t, json.Unmarshal(saved.Config, &stored))
	require.NotNil(t, stored.BaseInfo)
	require.Equal(t, originalID, stored.BaseInfo.ProjectID)

	updateConfig := map[string]any{
		"BaseInfo": map[string]any{
			"project_id":   0,
			"project_name": "projectid-test-updated",
			"language":     "java",
		},
	}
	updateConfigRaw, err := json.Marshal(updateConfig)
	require.NoError(t, err)

	require.NoError(t, project.UpdateConfig(ssaconfig.WithJsonRawConfig(updateConfigRaw)))
	require.NotNil(t, project.SSAProject)
	require.Equal(t, uint(originalID), project.ID)

	require.NoError(t, project.SaveToDB(db))
	require.Equal(t, uint(originalID), project.ID)

	var count int64
	require.NoError(t, db.Model(&schema.SSAProject{}).Count(&count).Error)
	require.Equal(t, int64(1), count)

	var savedAfterUpdate schema.SSAProject
	require.NoError(t, db.First(&savedAfterUpdate, uint(originalID)).Error)
	var storedAfterUpdate ssaconfig.Config
	require.NoError(t, json.Unmarshal(savedAfterUpdate.Config, &storedAfterUpdate))
	require.NotNil(t, storedAfterUpdate.BaseInfo)
	require.Equal(t, originalID, storedAfterUpdate.BaseInfo.ProjectID)
	require.Equal(t, "projectid-test-updated", storedAfterUpdate.BaseInfo.ProjectName)
}
