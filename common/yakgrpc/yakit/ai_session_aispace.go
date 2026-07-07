package yakit

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"gorm.io/gorm"
)

func queryAISpaceWorkDirs(db *gorm.DB, sessionIDs []string) ([]string, error) {
	if db == nil {
		return nil, utils.Errorf("database is nil")
	}

	query := db.Model(&schema.AIAgentRuntime{}).
		Where("work_dir IS NOT NULL AND work_dir != ''")
	if len(sessionIDs) > 0 {
		query = query.Where("persistent_session IN (?)", sessionIDs)
	}

	var workDirs []string
	if err := query.Pluck("work_dir", &workDirs).Error; err != nil {
		if isMissingTableErr(err) {
			return nil, nil
		}
		return nil, err
	}
	return lo.Uniq(workDirs), nil
}

func isDeletableAISpaceWorkDir(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" {
		return false
	}

	base := filepath.Clean(consts.GetDefaultAISpaceDir())
	if path == base {
		return false
	}
	prefix := base + string(filepath.Separator)
	return strings.HasPrefix(path, prefix)
}

// RemoveAISpaceWorkDirs deletes session artifact directories under the default aispace root.
// Unsafe paths are skipped. Returns the number of directories removed.
func RemoveAISpaceWorkDirs(workDirs []string) int {
	removed := 0
	seen := make(map[string]struct{}, len(workDirs))
	for _, workDir := range workDirs {
		workDir = strings.TrimSpace(workDir)
		if workDir == "" {
			continue
		}
		if _, ok := seen[workDir]; ok {
			continue
		}
		seen[workDir] = struct{}{}

		if !isDeletableAISpaceWorkDir(workDir) {
			log.Warnf("skip deleting unsafe ai space work dir: %s", workDir)
			continue
		}
		if err := os.RemoveAll(workDir); err != nil {
			log.Warnf("delete ai space work dir failed: %s: %v", workDir, err)
			continue
		}
		removed++
		log.Infof("deleted ai space work dir: %s", workDir)
	}
	return removed
}

// CleanupAISpaceWorkDirsForSessions removes aispace directories referenced by the given sessions.
// Query work dirs before deleting runtime rows from the database.
func CleanupAISpaceWorkDirsForSessions(db *gorm.DB, sessionIDs []string) (int, error) {
	if len(sessionIDs) == 0 {
		return 0, nil
	}
	workDirs, err := queryAISpaceWorkDirs(db, sessionIDs)
	if err != nil {
		return 0, err
	}
	return RemoveAISpaceWorkDirs(workDirs), nil
}

// CleanupAISpaceWorkDirsForAllSessions removes all aispace directories referenced by runtimes.
func CleanupAISpaceWorkDirsForAllSessions(db *gorm.DB) (int, error) {
	workDirs, err := queryAISpaceWorkDirs(db, nil)
	if err != nil {
		return 0, err
	}
	return RemoveAISpaceWorkDirs(workDirs), nil
}

// CleanupOrphanAISpaceWorkDirs removes aispace directories that are not referenced by any runtime work_dir.
// Disk reconciliation only runs against the live project database to avoid touching real files in tests.
func CleanupOrphanAISpaceWorkDirs(db *gorm.DB) (int, error) {
	if db == nil || db != consts.GetGormProjectDatabase() {
		return 0, nil
	}
	referencedDirs, err := queryAISpaceWorkDirs(db, nil)
	if err != nil {
		return 0, err
	}
	referenced := make(map[string]struct{}, len(referencedDirs))
	for _, dir := range referencedDirs {
		dir = filepath.Clean(strings.TrimSpace(dir))
		if dir == "" {
			continue
		}
		referenced[dir] = struct{}{}
	}

	baseDir := filepath.Clean(consts.GetDefaultAISpaceDir())
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	orphanDirs := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(baseDir, entry.Name())
		if !isDeletableAISpaceWorkDir(dirPath) {
			continue
		}
		if _, ok := referenced[filepath.Clean(dirPath)]; ok {
			continue
		}
		orphanDirs = append(orphanDirs, dirPath)
	}
	return RemoveAISpaceWorkDirs(orphanDirs), nil
}
