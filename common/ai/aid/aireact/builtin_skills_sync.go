package aireact

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func init() {
	yakit.RegisterPostInitDatabaseFunction(SyncBuiltinSkillsToDB, "sync-builtin-ai-skills")
}

// SyncBuiltinSkillsToDB makes embedded skills available to database search even
// before the first ReAct session is started.
func SyncBuiltinSkillsToDB() error {
	db := builtinSkillReleaseDB()
	if db == nil {
		return nil
	}
	loader, err := aiskillloader.NewAutoSkillLoader(aiskillloader.WithAutoLoad_FileSystem(GetBuiltinSkillsFS()))
	if err != nil {
		return err
	}
	_, err = aiskillloader.SyncBuiltinAISkillsToDB(db, loader)
	return err
}

func builtinSkillFilesKey(targetDir, name string) string {
	abs, _ := filepath.Abs(targetDir)
	return "ai.builtin_skills.files:" + filepath.ToSlash(filepath.Join(abs, "builtin", name))
}

func upgradeBuiltinSkillFiles(targetDir string) error {
	db := builtinSkillReleaseDB()
	if db == nil {
		return fmt.Errorf("builtin skill release database is unavailable")
	}
	loader, err := aiskillloader.NewAutoSkillLoader(aiskillloader.WithAutoLoad_FileSystem(GetBuiltinSkillsFS()))
	if err != nil {
		return err
	}
	for _, meta := range loader.AllSkillMetas() {
		if meta.Name == "" || meta.Name == "." || meta.Name == ".." || strings.ContainsAny(meta.Name, "/\\") {
			return fmt.Errorf("invalid builtin skill name %q", meta.Name)
		}
		loaded, err := loader.LoadSkill(meta.Name)
		if err != nil {
			return err
		}
		incoming, err := aiskillloader.SkillFileHashes(loaded.FileSystem)
		if err != nil {
			return err
		}
		key := builtinSkillFilesKey(targetDir, meta.Name)
		var previous map[string]string
		_, err = yakit.GetKeyModel(db, key)
		if err != nil && !gorm.IsRecordNotFoundError(err) {
			return err
		}
		if err == nil {
			// GetKey handles GeneralStorage's quoted-value representation.
			if err := json.Unmarshal([]byte(yakit.GetKey(db, key)), &previous); err != nil {
				return fmt.Errorf("read builtin skill baseline %q: %w", meta.Name, err)
			}
		}
		if reflect.DeepEqual(previous, incoming) {
			continue
		}
		skillDir := filepath.Join(targetDir, "builtin", meta.Name)
		relPath := meta.Name + "/SKILL.md"
		if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); os.IsNotExist(err) {
			if isBuiltinSkillSuppressed(relPath) {
				continue
			}
			if _, released := getBuiltinSkillReleaseTime(relPath); released {
				markBuiltinSkillUserRemoved(relPath)
				continue
			}
		} else if err != nil {
			return err
		}
		if info, err := os.Lstat(skillDir); err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("builtin skill path is not a regular directory: %s", skillDir)
			}
			localFS := filesys.NewRelLocalFs(skillDir)
			current, err := aiskillloader.SkillFileHashes(localFS)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(current, incoming) && !reflect.DeepEqual(current, previous) {
				legacyFS, err := builtinSkillLegacyFS(localFS, meta.Name+"-legacy")
				if err != nil {
					return err
				}
				if err := replaceBuiltinSkillDir(skillDir+"-legacy", legacyFS); err != nil {
					return fmt.Errorf("save builtin skill legacy %q: %w", meta.Name, err)
				}
				if err := persistBuiltinSkillLegacy(db, legacyFS); err != nil {
					return err
				}
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := replaceBuiltinSkillDir(skillDir, loaded.FileSystem); err != nil {
			return err
		}
		for name := range incoming {
			info, err := os.Stat(filepath.Join(skillDir, filepath.FromSlash(name)))
			if err != nil {
				return err
			}
			markBuiltinSkillReleased(meta.Name+"/"+name, info.ModTime())
		}
		raw, err := json.Marshal(incoming)
		if err != nil {
			return err
		}
		if err := yakit.SetKey(db, key, string(raw)); err != nil {
			return err
		}
	}
	return nil
}

func persistBuiltinSkillLegacy(db *gorm.DB, fsys fi.FileSystem) error {
	content, err := fsys.ReadFile("SKILL.md")
	if err != nil {
		return err
	}
	meta, err := aiskillloader.ParseSkillMeta(string(content))
	if err != nil {
		return err
	}
	forge, err := aiskillloader.LoadedSkillToAIForge(&aiskillloader.LoadedSkill{Meta: meta, FileSystem: fsys, SkillMDContent: string(content)})
	if err != nil {
		return err
	}
	return yakit.CreateOrUpdateAIForgeByName(db, meta.Name, forge)
}

func builtinSkillLegacyFS(localFS fi.FileSystem, name string) (fi.FileSystem, error) {
	raw, err := filesys.SerializeFileSystemToGzipBytes(localFS)
	if err != nil {
		return nil, err
	}
	legacy, err := filesys.NewGzipFSFromBytes(raw)
	if err != nil {
		return nil, err
	}
	content, err := legacy.ReadFile("SKILL.md")
	if err != nil {
		return nil, err
	}
	// VirtualFS.AddFile does not replace an existing entry.
	if err := legacy.Delete("SKILL.md"); err != nil {
		return nil, err
	}
	document, err := aiskillloader.ParseSkillDocument(string(content))
	if err != nil {
		// Keep malformed user documents byte-for-byte as a resource while making
		// the backup discoverable by the skill loader.
		legacy.AddFile("SKILL.original.md", string(content))
		legacy.AddFile("SKILL.md", fmt.Sprintf("---\nname: %s\ndescription: Preserved customized builtin skill\n---\n\n%s", name, content))
		return legacy, nil
	}
	renamed, err := document.Rename(name)
	if err != nil {
		return nil, err
	}
	legacy.AddFile("SKILL.md", renamed)
	return legacy, nil
}

// Stage the entire skill before replacing either the canonical or legacy copy.
// A failed stage leaves both usable copies intact; a failed rename rolls back.
func replaceBuiltinSkillDir(targetDir string, source fi.FileSystem) error {
	parent := filepath.Dir(targetDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".builtin-skill-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	hashes, err := aiskillloader.SkillFileHashes(source)
	if err != nil {
		return err
	}
	for name := range hashes {
		if !filepath.IsLocal(filepath.FromSlash(name)) {
			return fmt.Errorf("invalid builtin skill resource path %q", name)
		}
		content, err := source.ReadFile(name)
		if err != nil {
			return err
		}
		destination := filepath.Join(stage, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(destination, content, 0o644); err != nil {
			return err
		}
	}
	old := stage + ".old"
	if info, err := os.Lstat(targetDir); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("builtin skill path is not a regular directory: %s", targetDir)
		}
		if err := os.Rename(targetDir, old); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stage, targetDir); err != nil {
		if rollbackErr := os.Rename(old, targetDir); rollbackErr != nil && !os.IsNotExist(rollbackErr) {
			return fmt.Errorf("replace builtin skill: %v; restore %s: %w", err, old, rollbackErr)
		}
		return err
	}
	return os.RemoveAll(old)
}
