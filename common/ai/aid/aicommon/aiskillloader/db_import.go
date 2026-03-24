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

// ImportAISkillsToDB imports all skills from a loader into the ai_forges table
// as skillmd records. The returned count includes only rows that were created or updated.
func ImportAISkillsToDB(db *gorm.DB, loader SkillLoader) (int, error) {
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
		bytes.Equal(existing.FSBytes, current.FSBytes)
}

// ImportAISkillsFromLocalDirToDB imports skills from a local directory into ai_forges.
func ImportAISkillsFromLocalDirToDB(db *gorm.DB, dirPath string) (int, error) {
	loader, err := NewAutoSkillLoader(WithAutoLoad_LocalDir(dirPath))
	if err != nil {
		return 0, err
	}
	return ImportAISkillsToDB(db, loader)
}

// ImportAISkillsFromZipFileToDB imports skills from a zip file into ai_forges.
func ImportAISkillsFromZipFileToDB(db *gorm.DB, zipPath string) (int, error) {
	loader, err := NewAutoSkillLoader(WithAutoLoad_ZipFile(zipPath))
	if err != nil {
		return 0, err
	}
	return ImportAISkillsToDB(db, loader)
}

// ImportAISkillsFromFileSystemToDB imports skills from a filesystem into ai_forges.
func ImportAISkillsFromFileSystemToDB(db *gorm.DB, fsys fi.FileSystem) (int, error) {
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(fsys))
	if err != nil {
		return 0, err
	}
	return ImportAISkillsToDB(db, loader)
}
