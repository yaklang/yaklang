package yakgrpc

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestAIToolDefaultAsyncMetadataPersistence(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "tools.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.AIYakTool{}).Error)
	tool := &schema.AIYakTool{Name: "async_metadata_persistence"}
	// Exercise the same metadata fixup and DB update used by SaveAIToolV2 and
	// UpdateAITool. False must overwrite a previously persisted true value.
	for _, source := range []string{"__ASYNC__ = true", "__ASYNC__ = false", "__ASYNC__ = true", ""} {
		tool.Content = source + "\n__DESC__ = \"metadata persistence\"\nRESULT = \"done\""
		require.NoError(t, fixAIToolMetadata(tool))
		if tool.ID == 0 {
			_, err = yakit.CreateAIYakTool(db, tool)
		} else {
			_, err = yakit.UpdateAIYakToolByID(db, tool)
		}
		require.NoError(t, err)
		stored, err := yakit.GetAIYakTool(db, tool.Name)
		require.NoError(t, err)
		require.Equal(t, source == "__ASYNC__ = true", stored.DefaultAsync)
	}
}
