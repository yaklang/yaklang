package aiskillloader

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var builtinSkillSyncMu sync.Mutex

// SkillFileHashes fingerprints every file, including paths and resources larger
// than 10KB. ComputeSkillHash's size limit is unsuitable for release detection.
func SkillFileHashes(fsys fi.FileSystem) (map[string]string, error) {
	result := make(map[string]string)
	err := filesys.Recursive(".", filesys.WithFileSystem(fsys), filesys.WithFileStat(func(pathname string, info fs.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		content, err := fsys.ReadFile(pathname)
		if err != nil {
			return err
		}
		name := path.Clean(strings.ReplaceAll(pathname, "\\", "/"))
		result[name] = fmt.Sprintf("%x", sha256.Sum256(content))
		return nil
	}))
	return result, err
}

// SyncBuiltinAISkillsToDB publishes embedded revisions under their canonical
// names. User edits survive repeated starts of the same revision; on an upgrade
// they move to a single, replaceable <name>-legacy record in the same transaction.
func SyncBuiltinAISkillsToDB(db *gorm.DB, loader SkillLoader) (int, error) {
	if db == nil || loader == nil {
		return 0, utils.Error("builtin skill database and loader are required")
	}
	builtinSkillSyncMu.Lock()
	defer builtinSkillSyncMu.Unlock()
	if err := db.AutoMigrate(&schema.AIForge{}).Error; err != nil {
		return 0, err
	}
	if err := yakit.EnsureAIForgeFTS5(db); err != nil {
		return 0, err
	}
	count := 0
	for _, meta := range loader.AllSkillMetas() {
		loaded, err := loader.LoadSkill(meta.Name)
		if err != nil {
			return count, err
		}
		incoming, err := LoadedSkillToAIForge(loaded)
		if err != nil {
			return count, err
		}
		incoming.IsBuiltin = true
		incoming.Author = schema.AIResourceAuthorBuiltin
		hash, err := builtinSkillForgeHash(incoming)
		if err != nil {
			return count, err
		}
		incoming.BuiltinSkillHash = hash
		changed := false
		err = db.Transaction(func(tx *gorm.DB) error {
			existing, err := yakit.GetAIForgeByName(tx, incoming.ForgeName)
			if gorm.IsRecordNotFoundError(err) {
				changed = true
				return saveBuiltinSkillForge(tx, incoming)
			}
			if err != nil {
				return err
			}
			if existing.ForgeType != schema.FORGE_TYPE_SkillMD {
				return utils.Errorf("builtin skill %q conflicts with forge type %q", incoming.ForgeName, existing.ForgeType)
			}
			if existing.BuiltinSkillHash == hash {
				return nil
			}
			currentHash, err := builtinSkillForgeHash(existing)
			// Without a baseline, a differing record has unknown provenance. Back
			// it up once instead of trusting timestamps or an editable builtin flag.
			if err != nil || (currentHash != hash && currentHash != existing.BuiltinSkillHash) {
				legacy := *existing
				legacy.Model = gorm.Model{}
				legacy.ForgeName += "-legacy"
				legacy.ForgeVerboseName = legacy.ForgeName
				legacy.IsBuiltin = false
				legacy.BuiltinSkillHash = ""
				legacy.SkillPath = ""
				legacy.IsTemporary = false
				if err := saveBuiltinSkillForge(tx, &legacy); err != nil {
					return utils.Wrap(err, "save builtin skill legacy failed")
				}
			}
			changed = true
			return saveBuiltinSkillForge(tx, incoming)
		})
		if err != nil {
			return count, utils.Wrapf(err, "sync builtin skill %q failed", meta.Name)
		}
		if changed {
			count++
		}
	}
	return count, nil
}

func builtinSkillForgeHash(forge *schema.AIForge) (string, error) {
	fsys, err := filesys.NewGzipFSFromBytes(forge.FSBytes)
	if err != nil {
		return "", err
	}
	resources, err := SkillFileHashes(fsys)
	if err != nil {
		return "", err
	}
	// Compare actual resource contents, not tar order, permissions or timestamps.
	delete(resources, skillMDFilename)
	fields := forge.ToUpdateMap()
	delete(fields, "is_builtin")
	delete(fields, "skill_path")
	delete(fields, "is_temporary")
	fields["fs_bytes"] = resources
	raw, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

func saveBuiltinSkillForge(db *gorm.DB, forge *schema.AIForge) error {
	var existing schema.AIForge
	err := db.Unscoped().Where("forge_name = ?", forge.ForgeName).First(&existing).Error
	if gorm.IsRecordNotFoundError(err) {
		return db.Create(forge).Error
	}
	if err != nil {
		return err
	}
	fields := forge.ToUpdateMap()
	fields["author"] = forge.Author
	fields["builtin_skill_hash"] = forge.BuiltinSkillHash
	fields["deleted_at"] = nil
	return db.Unscoped().Model(&schema.AIForge{}).Where("id = ?", existing.ID).Updates(fields).Error
}
