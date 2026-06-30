package aiskillloader

import (
	"bytes"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// ImportSkillDBOptions controls optional fields when persisting skills into ai_forges.
type ImportSkillDBOptions struct {
	Author    string
	IsBuiltin bool
}

// ImportAISkillsToDB imports all skills from a loader into the ai_forges table
// as skillmd records. The returned count includes only rows that were created or updated.
func ImportAISkillsToDB(db *gorm.DB, loader SkillLoader, opts ...ImportSkillDBOptions) (int, error) {
	if db == nil {
		return 0, utils.Error("db is nil")
	}
	if loader == nil {
		return 0, utils.Error("skill loader is nil")
	}

	db.AutoMigrate(&schema.AIForge{})
	if err := yakit.EnsureAIForgeFTS5(db); err != nil {
		return 0, utils.Wrap(err, "ensure ai_forges FTS5 failed")
	}

	var opt ImportSkillDBOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	persisted := 0
	for _, meta := range loader.AllSkillMetas() {
		loaded, err := loader.LoadSkill(meta.Name)
		if err != nil {
			log.Warnf("failed to load skill %q: %v", meta.Name, err)
			continue
		}
		forge, err := LoadedSkillToAIForge(loaded)
		if err != nil {
			log.Warnf("failed to convert skill %q to forge: %v", meta.Name, err)
			continue
		}
		if opt.Author != "" {
			forge.Author = opt.Author
		}
		if opt.IsBuiltin {
			forge.IsBuiltin = true
		}
		existing, err := yakit.GetAIForgeByNameAndTypes(db, meta.Name, schema.FORGE_TYPE_SkillMD)
		if err == nil && sameSkillMDForge(existing, forge) {
			continue
		}
		if err != nil && !gorm.IsRecordNotFoundError(err) {
			log.Warnf("failed to query existing skillmd forge %q: %v", meta.Name, err)
			continue
		}
		if err := yakit.CreateOrUpdateAIForgeByName(db, meta.Name, forge); err != nil {
			log.Warnf("failed to persist skill %q as forge: %v", meta.Name, err)
			continue
		}
		persisted++
	}
	return persisted, nil
}

func sameSkillMDForge(existing *schema.AIForge, current *schema.AIForge) bool {
	if existing == nil || current == nil {
		return false
	}
	return existing.ForgeName == current.ForgeName &&
		existing.ForgeType == current.ForgeType &&
		existing.Description == current.Description &&
		existing.Tags == current.Tags &&
		existing.InitPrompt == current.InitPrompt &&
		existing.Author == current.Author &&
		existing.IsBuiltin == current.IsBuiltin &&
		bytes.Equal(existing.FSBytes, current.FSBytes)
}

// ImportAISkillsFromAllDirsToDB scans well-known skill directories and imports
// discovered SKILL.md entries into ai_forges for Yakit skill library listing.
func ImportAISkillsFromAllDirsToDB(db *gorm.DB, dirs []string) (int, error) {
	if db == nil {
		return 0, utils.Error("db is nil")
	}
	total := 0
	for _, dir := range dirs {
		if dir == "" || !utils.IsDir(dir) {
			continue
		}
		n, err := ImportAISkillsFromLocalDirToDB(db, dir)
		if err != nil {
			log.Warnf("import skills from %q failed: %v", dir, err)
			continue
		}
		total += n
	}
	return total, nil
}

// ImportAISkillsFromLocalDirToDB imports skills from a local directory into ai_forges.
func ImportAISkillsFromLocalDirToDB(db *gorm.DB, dirPath string, opts ...ImportSkillDBOptions) (int, error) {
	loader, err := NewAutoSkillLoader(WithAutoLoad_LocalDir(dirPath))
	if err != nil {
		return 0, err
	}
	return ImportAISkillsToDB(db, loader, opts...)
}

// ImportAISkillsFromArchiveFileToDB imports skills from an archive file into ai_forges.
// Supported archive formats are zip, tar, tar.gz and tgz.
func ImportAISkillsFromArchiveFileToDB(db *gorm.DB, archivePath string) (int, error) {
	loader, err := NewAutoSkillLoader(WithAutoLoad_ArchiveFile(archivePath))
	if err != nil {
		return 0, err
	}
	return ImportAISkillsToDB(db, loader)
}

// ImportAISkillsFromZipFileToDB imports skills from an archive file into ai_forges.
// Deprecated: use ImportAISkillsFromArchiveFileToDB instead.
func ImportAISkillsFromZipFileToDB(db *gorm.DB, zipPath string) (int, error) {
	return ImportAISkillsFromArchiveFileToDB(db, zipPath)
}

// ImportAISkillsFromFileSystemToDB imports skills from a filesystem into ai_forges.
func ImportAISkillsFromFileSystemToDB(db *gorm.DB, fsys fi.FileSystem, opts ...ImportSkillDBOptions) (int, error) {
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(fsys))
	if err != nil {
		return 0, err
	}
	return ImportAISkillsToDB(db, loader, opts...)
}
