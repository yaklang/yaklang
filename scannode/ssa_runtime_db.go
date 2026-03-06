package scannode

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
)

const ssaRuntimeDBDirName = "ssa-runtime-db"

func (s *ScanNode) needIsolateSSARuntimeDB() bool {
	if s == nil || s.invokeLimiter == nil {
		return false
	}
	// Only enable isolation when parallelism is explicitly increased.
	return s.invokeLimiter.totalN > 1
}

func buildSSARuntimeDBPaths(runtimeID string) (projectDB string, ssaDB string) {
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		runtimeID = fmt.Sprintf("runtime-%d", time.Now().UnixNano())
	}
	base := filepath.Join(consts.GetDefaultYakitBaseTempDir(), ssaRuntimeDBDirName)
	_ = os.MkdirAll(base, 0o755)
	// Use absolute paths to bypass YAKIT_HOME base dir joining logic.
	projectDB = filepath.Join(base, fmt.Sprintf("yakit-project-%s.db", runtimeID))
	ssaDB = filepath.Join(base, fmt.Sprintf("yakssa-%s.db", runtimeID))
	return projectDB, ssaDB
}

func cleanupSQLiteFiles(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	_ = os.Remove(path)
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
}

func buildSSARuntimeDBEnv(runtimeID string, ssaDatabaseRawOverride string) (env []string, cleanup func()) {
	projectDB, ssaDB := buildSSARuntimeDBPaths(runtimeID)
	ssaDatabaseRawOverride = strings.TrimSpace(ssaDatabaseRawOverride)
	ssaRaw := ssaDB
	cleanupSSA := true
	if ssaDatabaseRawOverride != "" {
		ssaRaw = ssaDatabaseRawOverride
		cleanupSSA = false
	}
	env = []string{
		fmt.Sprintf("%s=%s", consts.CONST_YAK_DEFAULT_PROJECT_DATABASE_NAME, projectDB),
		fmt.Sprintf("%s=%s", consts.ENV_SSA_DATABASE_RAW, ssaRaw),
	}
	cleanup = func() {
		cleanupSQLiteFiles(projectDB)
		if cleanupSSA {
			cleanupSQLiteFiles(ssaDB)
		}
	}
	return env, cleanup
}
