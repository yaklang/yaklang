package aireact

import (
	"bytes"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

//go:embed skills
var builtinSkillsFS embed.FS

// GetBuiltinSkillsFS returns the embedded filesystem containing built-in skills.
// These skills ship with the binary and are always available unless explicitly
// disabled via WithDisableAutoSkills(true).
//
// The filesystem root contains skill directories (e.g. skills/code-review/),
// each with a SKILL.md defining the skill metadata and content.
func GetBuiltinSkillsFS() fi.FileSystem {
	return filesys.NewEmbedFS(builtinSkillsFS)
}

// ExtractBuiltinSkillsToDir extracts built-in skills from the embedded filesystem
// to a target directory on disk (typically ~/yakit-projects/ai-skills/).
// This enables users to view, modify, and extend skills directly on the filesystem.
//
// Only files that don't exist or have changed content are written, avoiding
// unnecessary disk I/O. The embedded FS layout is "skills/<skill-name>/SKILL.md";
// the "skills/" prefix is stripped so the output becomes "<targetDir>/<skill-name>/SKILL.md".
func ExtractBuiltinSkillsToDir(targetDir string) error {
	embedFS := filesys.NewEmbedFS(builtinSkillsFS)

	return filesys.SimpleRecursive(
		filesys.WithFileSystem(embedFS),
		filesys.WithFileStat(func(pathname string, info fs.FileInfo) error {
			// pathname is like "skills/code-review/SKILL.md"
			// Strip the "skills/" prefix to get the relative path under targetDir
			relPath := strings.TrimPrefix(pathname, "skills/")
			if relPath == pathname {
				// File not under skills/ directory, skip
				return nil
			}

			// Read content from embed
			content, err := embedFS.ReadFile(pathname)
			if err != nil {
				log.Warnf("failed to read embedded skill file %s: %v", pathname, err)
				return nil
			}

			// Target path on disk
			targetPath := filepath.Join(targetDir, relPath)

			// Check if file already exists with same content (skip if unchanged)
			existing, err := os.ReadFile(targetPath)
			if err == nil && bytes.Equal(existing, content) {
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

			log.Infof("extracted builtin skill file: %s", targetPath)
			return nil
		}),
	)
}
