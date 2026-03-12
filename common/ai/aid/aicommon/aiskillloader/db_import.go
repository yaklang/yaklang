package aiskillloader

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// ImportAISkillsToDB imports all skills from a loader into the ai_skills table.
// It auto-migrates the schema, ensures the FTS index, and uses hash-based deduplication.
// The returned count includes only rows that were created or updated.
func ImportAISkillsToDB(db *gorm.DB, loader SkillLoader) (int, error) {
	if db == nil {
		return 0, utils.Error("db is nil")
	}
	if loader == nil {
		return 0, utils.Error("skill loader is nil")
	}

	db.AutoMigrate(&schema.AISkill{})
	if err := yakit.EnsureAISkillFTS5(db); err != nil {
		return 0, utils.Wrap(err, "ensure ai_skills FTS5 failed")
	}

	persisted := 0
	for _, meta := range loader.AllSkillMetas() {
		fsys, err := loader.GetFileSystem(meta.Name)
		if err != nil {
			log.Warnf("failed to get filesystem for skill %q: %v", meta.Name, err)
			continue
		}
		hash := ComputeSkillHash(fsys)
		existing, err := yakit.GetAISkillByName(db, meta.Name)
		if err == nil && existing != nil && existing.Hash == hash {
			continue
		}
		skill := &schema.AISkill{
			Name:                   meta.Name,
			Description:            meta.Description,
			License:                meta.License,
			Keywords:               buildKeywordsString(meta),
			Body:                   meta.Body,
			Hash:                   hash,
			DisableModelInvocation: meta.DisableModelInvocation,
		}
		if err := yakit.CreateOrUpdateAISkill(db, skill); err != nil {
			log.Warnf("failed to persist skill %q: %v", meta.Name, err)
			continue
		}
		persisted++
	}
	return persisted, nil
}

// ImportAISkillsFromLocalDirToDB imports skills from a local directory into ai_skills.
func ImportAISkillsFromLocalDirToDB(db *gorm.DB, dirPath string) (int, error) {
	loader, err := NewAutoSkillLoader(WithAutoLoad_LocalDir(dirPath))
	if err != nil {
		return 0, err
	}
	return ImportAISkillsToDB(db, loader)
}

// ImportAISkillsFromZipFileToDB imports skills from a zip file into ai_skills.
func ImportAISkillsFromZipFileToDB(db *gorm.DB, zipPath string) (int, error) {
	loader, err := NewAutoSkillLoader(WithAutoLoad_ZipFile(zipPath))
	if err != nil {
		return 0, err
	}
	return ImportAISkillsToDB(db, loader)
}

// ImportAISkillsFromFileSystemToDB imports skills from a filesystem into ai_skills.
func ImportAISkillsFromFileSystemToDB(db *gorm.DB, fsys fi.FileSystem) (int, error) {
	loader, err := NewAutoSkillLoader(WithAutoLoad_FileSystem(fsys))
	if err != nil {
		return 0, err
	}
	return ImportAISkillsToDB(db, loader)
}
