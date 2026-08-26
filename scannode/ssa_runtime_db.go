package scannode

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

const ssaRuntimeDBDirName = "ssa-runtime-db"

func (s *ScanNode) needIsolateSSARuntimeDB() bool {
	if s == nil || s.invokeLimiter == nil {
		return false
	}
	// Unlimited and explicitly parallel Nodes must never let child processes
	// share the implicit local project/SSA SQLite files.
	return s.invokeLimiter.capacity() == 0 || s.invokeLimiter.capacity() > 1
}

func buildSSARuntimeDBPaths(runtimeID string) (projectDB string, ssaDB string) {
	runtimeID = strings.TrimSpace(runtimeID)
	if runtimeID == "" {
		runtimeID = fmt.Sprintf("runtime-%d", time.Now().UnixNano())
	} else {
		runtimeID = sanitizeLogName(runtimeID)
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

func copySQLiteIRIntoDebugDir(debugDir, livePath string) {
	debugDir = strings.TrimSpace(debugDir)
	livePath = strings.TrimSpace(livePath)
	if debugDir == "" || livePath == "" {
		return
	}
	absLive, err := filepath.Abs(livePath)
	if err != nil {
		absLive = livePath
	}
	absDebug, err := filepath.Abs(debugDir)
	if err != nil {
		absDebug = debugDir
	}
	sep := string(os.PathSeparator)
	if absLive == filepath.Join(absDebug, "ssadb.db") ||
		strings.HasPrefix(absLive, absDebug+sep) {
		return
	}
	if err := os.MkdirAll(debugDir, 0o755); err != nil {
		log.Warnf("[ssadb] mkdir debug dir for sqlite copy failed: %v", err)
		return
	}
	dest := filepath.Join(debugDir, "ssadb.db")
	if err := copyFileIfExists(absLive, dest); err != nil {
		log.Warnf("[ssadb] copy sqlite IR into debug dir failed: %v", err)
		return
	}
	_ = copyFileIfExists(absLive+"-wal", dest+"-wal")
	_ = copyFileIfExists(absLive+"-shm", dest+"-shm")
	log.Infof("[ssadb] copied sqlite IR into debug dir: %s", dest)
}

func copyFileIfExists(src, dst string) error {
	if src == "" || dst == "" || src == dst {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
