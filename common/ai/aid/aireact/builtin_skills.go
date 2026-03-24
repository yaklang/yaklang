package aireact

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

//go:generate gzip-embed -cache --source ./skills --gz skills.tar.gz --root-path --no-embed

var builtinSkillsFS fi.FileSystem

const builtinSkillReleaseTimeKeyPrefix = "ai.builtin_skills.release_time"

var builtinSkillReleaseDB = func() *gorm.DB {
	return consts.GetGormProfileDatabase()
}

// GetBuiltinSkillsFS returns the embedded filesystem containing built-in skills.
// These skills ship with the binary and are always available unless explicitly
// disabled via WithDisableAutoSkills(true).
//
// The filesystem root contains skill directories (e.g. skills/code-review/),
// each with a SKILL.md defining the skill metadata and content.
func GetBuiltinSkillsFS() fi.FileSystem {
	return builtinSkillsFS
}

// ExtractBuiltinSkillsToDir extracts built-in skills from the embedded filesystem
// to a target directory on disk (typically ~/yakit-projects/ai-skills/builtin/).
// This enables users to view, modify, and extend skills directly on the filesystem.
// Built-in skills are placed under a "builtin/" subdirectory to separate them from
// user-created skills that live directly under the target directory.
//
// Only files that do not already exist are written. Existing local files are
// always preserved so users can freely customize built-in skills without their
// changes being overwritten by later runs. The embedded FS layout is
// "skills/<skill-name>/SKILL.md"; the "skills/" prefix is stripped and
// "builtin/" is prepended, so the output becomes
// "<targetDir>/builtin/<skill-name>/SKILL.md".
func ExtractBuiltinSkillsToDir(targetDir string) error {
	embedFS := GetBuiltinSkillsFS()

	return filesys.SimpleRecursive(
		filesys.WithFileSystem(embedFS),
		filesys.WithFileStat(func(pathname string, info fs.FileInfo) error {
			if info.IsDir() {
				return nil
			}

			// pathname is like "skills/code-review/SKILL.md"
			// Strip the "skills/" prefix to get the relative path under targetDir
			relPath := strings.TrimPrefix(pathname, "skills/")
			if relPath == pathname {
				// File not under skills/ directory, skip
				return nil
			}

			// Target path: <targetDir>/builtin/<skill-name>/SKILL.md
			targetPath := filepath.Join(targetDir, "builtin", relPath)

			// Preserve any existing local file and never overwrite it.
			if existingInfo, err := os.Stat(targetPath); err == nil {
				if releasedAt, ok := getBuiltinSkillReleaseTime(relPath); ok && existingInfo.ModTime().After(releasedAt) {
					log.Infof("preserving modified builtin skill file: %s", targetPath)
				} else {
					log.Infof("builtin skill file already exists, preserving local copy: %s", targetPath)
				}
				return nil
			} else if !os.IsNotExist(err) {
				log.Warnf("failed to inspect existing skill file %s: %v", targetPath, err)
				return nil
			}

			// Read content from embed only when a write is actually needed.
			content, err := embedFS.ReadFile(pathname)
			if err != nil {
				log.Warnf("failed to read embedded skill file %s: %v", pathname, err)
				return nil
			}

			// Ensure parent directory exists
			parentDir := filepath.Dir(targetPath)
			if err := os.MkdirAll(parentDir, 0o755); err != nil {
				log.Warnf("failed to create skill directory %s: %v", parentDir, err)
				return nil
			}

			// Write file
			if err := os.WriteFile(targetPath, content, 0o644); err != nil {
				log.Warnf("failed to write skill file %s: %v", targetPath, err)
				return nil
			}

			releasedAt := time.Now()
			if writtenInfo, err := os.Stat(targetPath); err == nil {
				releasedAt = writtenInfo.ModTime()
			}
			markBuiltinSkillReleased(relPath, releasedAt)

			log.Infof("extracted builtin skill file: %s", targetPath)
			return nil
		}),
	)
}

func builtinSkillReleaseKey(relPath string) string {
	normalizedPath := filepath.ToSlash(filepath.Join("builtin", relPath))
	return builtinSkillReleaseTimeKeyPrefix + ":" + normalizedPath
}

func getBuiltinSkillReleaseTime(relPath string) (time.Time, bool) {
	db := builtinSkillReleaseDB()
	if db == nil {
		return time.Time{}, false
	}

	raw := strings.TrimSpace(yakit.GetKey(db, builtinSkillReleaseKey(relPath)))
	if raw == "" {
		return time.Time{}, false
	}

	unixMillis, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		log.Warnf("failed to parse builtin skill release time for %s: %v", relPath, err)
		return time.Time{}, false
	}

	return time.UnixMilli(unixMillis), true
}

func markBuiltinSkillReleased(relPath string, releasedAt time.Time) {
	db := builtinSkillReleaseDB()
	if db == nil {
		return
	}

	if err := yakit.SetKey(db, builtinSkillReleaseKey(relPath), strconv.FormatInt(releasedAt.UnixMilli(), 10)); err != nil {
		log.Warnf("failed to persist builtin skill release time for %s: %v", relPath, err)
	}
}
