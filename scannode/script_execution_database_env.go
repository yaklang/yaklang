package scannode

import (
	"path/filepath"

	"github.com/yaklang/yaklang/common/consts"
)

// The immutable rule snapshot has a task-local Yakit home. Keep this environment
// override separate from the per-execution project and company SSA database.
func ruleSnapshotDatabaseEnv(taskHome string) []string {
	return []string{
		"YAKIT_HOME=" + taskHome,
		consts.CONST_YAK_DEFAULT_PROFILE_DATABASE_NAME + "=" + filepath.Join(taskHome, "yakit-profile-plugin.db"),
	}
}
