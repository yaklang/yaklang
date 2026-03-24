package aiskillloader

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// AutoSkillLoader discovers and loads skills by recursively traversing multiple
// filesystem-like sources, including skillmd forges materialized from the database.
type AutoSkillLoader struct {
	mu     sync.RWMutex
	skills map[string]*skillEntry

	sources []fi.FileSystem

	db           *gorm.DB
	dbSkillCount int

	cooldown    *utils.CoolDown
	scannedDirs map[string]struct{}
}

// AutoSkillLoaderOption configures AutoSkillLoader construction.
type AutoSkillLoaderOption func(*AutoSkillLoader) error

// WithAutoLoad_LocalDir adds a local directory as a skill source.
func WithAutoLoad_LocalDir(dirPath string) AutoSkillLoaderOption {
	return func(l *AutoSkillLoader) error {
		if !utils.IsDir(dirPath) {
			return utils.Errorf("skill directory does not exist: %s", dirPath)
		}
		localFS := filesys.NewRelLocalFs(dirPath)
		l.sources = append(l.sources, localFS)
		return nil
	}
}

// WithAutoLoad_ZipFile adds a zip file as a skill source.
func WithAutoLoad_ZipFile(zipPath string) AutoSkillLoaderOption {
	return func(l *AutoSkillLoader) error {
		zipFS, err := filesys.NewZipFSFromLocal(zipPath)
		if err != nil {
			return utils.Wrapf(err, "failed to open zip file: %s", zipPath)
		}
		l.sources = append(l.sources, zipFS)
		return nil
	}
}

// WithAutoLoad_FileSystem adds a generic FileSystem as a skill source.
func WithAutoLoad_FileSystem(fsys fi.FileSystem) AutoSkillLoaderOption {
	return func(l *AutoSkillLoader) error {
		if fsys == nil {
			return utils.Error("filesystem is nil")
		}
		l.sources = append(l.sources, fsys)
		return nil
	}
}

// WithAutoLoad_Database attaches a lazy database-backed skill source.
func WithAutoLoad_Database(db *gorm.DB) AutoSkillLoaderOption {
	return func(l *AutoSkillLoader) error {
		return l.attachDatabase(db)
	}
}

// NewAutoSkillLoader creates an AutoSkillLoader that recursively discovers
// SKILL.md files from multiple sources.
func NewAutoSkillLoader(opts ...AutoSkillLoaderOption) (*AutoSkillLoader, error) {
	l := &AutoSkillLoader{
		skills:      make(map[string]*skillEntry),
		cooldown:    utils.NewCoolDown(time.Minute),
		scannedDirs: make(map[string]struct{}),
	}

	for _, opt := range opts {
		if err := opt(l); err != nil {
			return nil, utils.Wrapf(err, "failed to configure AutoSkillLoader")
		}
	}

	// Discover skills from all sources
	for _, src := range l.sources {
		if err := l.discoverSkills(src); err != nil {
			log.Warnf("failed to discover skills from source: %v", err)
		}
	}

	return l, nil
}

// discoverSkills recursively walks a FileSystem to find all SKILL.md files.
func (l *AutoSkillLoader) discoverSkills(rootFS fi.FileSystem) error {
	return filesys.SimpleRecursive(
		filesys.WithFileSystem(rootFS),
		filesys.WithFileStat(func(pathname string, info fs.FileInfo) error {
			if info.Name() != skillMDFilename {
				return nil
			}

			parentDir := filepath.Dir(pathname)
			if parentDir == "." {
				parentDir = ""
			}

			content, err := rootFS.ReadFile(pathname)
			if err != nil {
				log.Warnf("failed to read %s: %v", pathname, err)
				return nil
			}

			meta, err := ParseSkillMeta(string(content))
			if err != nil {
				log.Warnf("failed to parse %s: %v", pathname, err)
				return nil
			}

			var skillFS fi.FileSystem
			if parentDir == "" {
				skillFS = rootFS
			} else {
				skillFS = &subDirFS{
					parent:  rootFS,
					subDir:  parentDir,
					dirName: filepath.Base(parentDir),
				}
			}

			l.mu.Lock()
			l.skills[meta.Name] = &skillEntry{
				fs:   skillFS,
				meta: meta,
			}
			l.mu.Unlock()

			log.Infof("auto-discovered skill: %s from %s", meta.Name, pathname)
			return nil
		}),
	)
}

// ComputeSkillHash computes a deterministic hash for a skill filesystem.
// It collects all files <=10KB (sorted by path), SHA256s each,
// concatenates the hex digests, and SHA256s the concatenation.
func ComputeSkillHash(skillFS fi.FileSystem) string {
	const maxFileSize = 10 * 1024 // 10KB

	var filePaths []string
	_ = filesys.SimpleRecursive(
		filesys.WithFileSystem(skillFS),
		filesys.WithFileStat(func(pathname string, info fs.FileInfo) error {
			if info.Size() <= maxFileSize {
				filePaths = append(filePaths, pathname)
			}
			return nil
		}),
	)

	sort.Strings(filePaths)

	var combined strings.Builder
	for _, fp := range filePaths {
		content, err := skillFS.ReadFile(fp)
		if err != nil {
			continue
		}
		h := sha256.Sum256(content)
		combined.WriteString(fmt.Sprintf("%x", h))
	}

	finalHash := sha256.Sum256([]byte(combined.String()))
	return fmt.Sprintf("%x", finalHash)
}

func getDatabaseSkillCount(db *gorm.DB) (int, error) {
	if db == nil {
		return 0, utils.Error("db is nil")
	}
	var count int
	if err := db.Model(&schema.AIForge{}).Where("forge_type = ?", schema.FORGE_TYPE_SkillMD).Count(&count).Error; err != nil {
		return 0, utils.Wrap(err, "count skillmd forges failed")
	}
	return count, nil
}

func (l *AutoSkillLoader) attachDatabase(db *gorm.DB) error {
	if db == nil {
		return utils.Error("db is nil")
	}
	count, err := getDatabaseSkillCount(db)
	if err != nil {
		return err
	}
	l.db = db
	l.dbSkillCount = count
	return nil
}

func (l *AutoSkillLoader) loadSkillFromDatabase(name string) (*LoadedSkill, error) {
	l.mu.RLock()
	db := l.db
	l.mu.RUnlock()
	if db == nil {
		return nil, utils.Errorf("skill %q not found", name)
	}

	forge, err := yakit.GetAIForgeByNameAndTypes(db, name, schema.FORGE_TYPE_SkillMD)
	if err != nil {
		return nil, utils.Wrapf(err, "load skill %q from skillmd forge failed", name)
	}
	loaded, err := AIForgeToLoadedSkill(forge)
	if err != nil {
		return nil, utils.Wrapf(err, "convert skillmd forge %q to loaded skill failed", name)
	}

	l.mu.Lock()
	l.skills[name] = &skillEntry{
		fs:   loaded.FileSystem,
		meta: loaded.Meta,
	}
	l.mu.Unlock()
	return loaded, nil
}

// --- SkillLoader interface implementation ---

// LoadSkill loads a specific skill by name.
func (l *AutoSkillLoader) LoadSkill(name string) (*LoadedSkill, error) {
	l.mu.RLock()
	entry, ok := l.skills[name]
	l.mu.RUnlock()
	if !ok {
		return l.loadSkillFromDatabase(name)
	}

	content, err := entry.fs.ReadFile(skillMDFilename)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read %s for skill %q", skillMDFilename, name)
	}

	return &LoadedSkill{
		Meta:           entry.meta,
		FileSystem:     entry.fs,
		SkillMDContent: string(content),
	}, nil
}

// GetFileSystem returns the read-only filesystem for a specific skill.
func (l *AutoSkillLoader) GetFileSystem(name string) (fi.FileSystem, error) {
	l.mu.RLock()
	entry, ok := l.skills[name]
	l.mu.RUnlock()
	if !ok {
		loaded, err := l.loadSkillFromDatabase(name)
		if err != nil {
			return nil, err
		}
		return loaded.FileSystem, nil
	}
	return entry.fs, nil
}

// HasSkills returns true if at least one skill is registered.
func (l *AutoSkillLoader) HasSkills() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.skills) > 0 || l.dbSkillCount > 0
}

// AllSkillMetas returns metadata for all available skills.
// For database-backed skills, metadata is resolved lazily and therefore not enumerated here.
func (l *AutoSkillLoader) AllSkillMetas() []*SkillMeta {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*SkillMeta, 0, len(l.skills))
	for _, entry := range l.skills {
		result = append(result, entry.meta)
	}
	return result
}

// GetSkillMeta returns skill metadata by name, using lazy DB lookup when needed.
func (l *AutoSkillLoader) GetSkillMeta(name string) (*SkillMeta, error) {
	l.mu.RLock()
	entry, ok := l.skills[name]
	db := l.db
	l.mu.RUnlock()
	if ok {
		return entry.meta, nil
	}
	if db == nil {
		return nil, utils.Errorf("skill %q not found", name)
	}
	forge, err := yakit.GetAIForgeByNameAndTypes(db, name, schema.FORGE_TYPE_SkillMD)
	if err != nil {
		return nil, err
	}
	meta := forgeToSkillMetaPreview(forge)
	if meta == nil {
		return nil, utils.Errorf("skill %q not found", name)
	}
	return meta, nil
}

// SearchSkillMetas searches local skills structurally and database-backed skillmd forges via BM25.
func (l *AutoSkillLoader) SearchSkillMetas(query string, limit int) ([]*SkillMeta, error) {
	local := SearchSkillMetasByStructure(query, l.AllSkillMetas(), limit)

	l.mu.RLock()
	db := l.db
	l.mu.RUnlock()
	if db == nil {
		return local, nil
	}

	dbResults, err := yakit.SearchAIForgeBM25(db, &yakit.AIForgeSearchFilter{
		ForgeTypes: []string{schema.FORGE_TYPE_SkillMD},
		Keywords:   strings.Fields(strings.TrimSpace(query)),
	}, limit, 0)
	if err != nil {
		if len(local) > 0 {
			return local, nil
		}
		return nil, err
	}

	seen := make(map[string]struct{}, len(local)+len(dbResults))
	merged := make([]*SkillMeta, 0, len(local)+len(dbResults))
	for _, meta := range local {
		if meta == nil || meta.Name == "" {
			continue
		}
		seen[meta.Name] = struct{}{}
		merged = append(merged, meta)
	}
	for _, forge := range dbResults {
		meta := forgeToSkillMetaPreview(forge)
		if meta == nil || meta.Name == "" {
			continue
		}
		if _, exists := seen[meta.Name]; exists {
			continue
		}
		seen[meta.Name] = struct{}{}
		merged = append(merged, meta)
	}
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, nil
}

// GetSkillSourceStats returns lightweight source statistics without forcing DB skill enumeration.
func (l *AutoSkillLoader) GetSkillSourceStats() SkillSourceStats {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return SkillSourceStats{
		LocalCount:    len(l.skills),
		DatabaseCount: l.dbSkillCount,
	}
}

// AddSource adds a new filesystem source and discovers skills from it.
// This allows adding skills incrementally after the loader is created.
// Returns the number of new skills discovered from this source.
func (l *AutoSkillLoader) AddSource(fsys fi.FileSystem) (int, error) {
	if fsys == nil {
		return 0, utils.Error("filesystem is nil")
	}

	// Count skills before adding (read lock only)
	l.mu.RLock()
	beforeCount := len(l.skills)
	l.mu.RUnlock()

	// Add source and discover skills
	// Note: discoverSkills acquires l.mu.Lock internally per-skill,
	// so we must NOT hold any lock here.
	l.mu.Lock()
	l.sources = append(l.sources, fsys)
	l.mu.Unlock()

	if err := l.discoverSkills(fsys); err != nil {
		return 0, err
	}

	// Count skills after adding
	l.mu.RLock()
	afterCount := len(l.skills)
	l.mu.RUnlock()

	return afterCount - beforeCount, nil
}

// AddLocalDir adds a local directory as a skill source.
// This is a convenience method that wraps AddSource.
// Directories already scanned are skipped to avoid duplicate discovery.
func (l *AutoSkillLoader) AddLocalDir(dirPath string) (int, error) {
	if !utils.IsDir(dirPath) {
		return 0, utils.Errorf("skill directory does not exist: %s", dirPath)
	}
	absPath, _ := filepath.Abs(dirPath)
	l.mu.RLock()
	_, alreadyScanned := l.scannedDirs[absPath]
	l.mu.RUnlock()
	if alreadyScanned {
		return 0, nil
	}

	localFS := filesys.NewRelLocalFs(dirPath)
	count, err := l.AddSource(localFS)
	if err == nil {
		l.mu.Lock()
		l.scannedDirs[absPath] = struct{}{}
		l.mu.Unlock()
	}
	return count, err
}

// RefreshFromDirs scans the given directories for skills, subject to a 60-second cooldown.
// Within the cooldown period, subsequent calls are silently skipped.
// Already-scanned directories (by absolute path) are never re-walked even when
// the cooldown expires, ensuring truly new dirs are picked up without redundant I/O.
func (l *AutoSkillLoader) RefreshFromDirs(dirs []string) {
	l.cooldown.Do(func() {
		for _, dir := range dirs {
			absDir, _ := filepath.Abs(dir)
			l.mu.RLock()
			_, alreadyScanned := l.scannedDirs[absDir]
			l.mu.RUnlock()
			if alreadyScanned {
				continue
			}
			if !utils.IsDir(dir) {
				continue
			}
			localFS := filesys.NewRelLocalFs(dir)
			if err := l.discoverSkills(localFS); err != nil {
				log.Warnf("failed to discover skills from %s: %v", dir, err)
				continue
			}
			l.mu.Lock()
			l.sources = append(l.sources, localFS)
			l.scannedDirs[absDir] = struct{}{}
			l.mu.Unlock()
			log.Debugf("refreshed skills from directory: %s", dir)
		}
	})
}

// RescanLocalDir re-discovers skills from a directory that may have been
// previously scanned. Unlike AddLocalDir, it always re-walks the directory,
// picking up any newly added or changed SKILL.md files.
// Used by LoadBuiltinSkillsFromDir after extracting embedded skills to disk.
func (l *AutoSkillLoader) RescanLocalDir(dirPath string) (int, error) {
	if !utils.IsDir(dirPath) {
		return 0, utils.Errorf("skill directory does not exist: %s", dirPath)
	}

	l.mu.RLock()
	beforeCount := len(l.skills)
	l.mu.RUnlock()

	localFS := filesys.NewRelLocalFs(dirPath)
	if err := l.discoverSkills(localFS); err != nil {
		return 0, err
	}

	absPath, _ := filepath.Abs(dirPath)
	l.mu.Lock()
	if _, exists := l.scannedDirs[absPath]; !exists {
		l.sources = append(l.sources, localFS)
	}
	l.scannedDirs[absPath] = struct{}{}
	l.mu.Unlock()

	l.mu.RLock()
	afterCount := len(l.skills)
	l.mu.RUnlock()

	return afterCount - beforeCount, nil
}

// AddZipFile adds a zip file as a skill source.
// This is a convenience method that wraps AddSource.
func (l *AutoSkillLoader) AddZipFile(zipPath string) (int, error) {
	zipFS, err := filesys.NewZipFSFromLocal(zipPath)
	if err != nil {
		return 0, utils.Wrapf(err, "failed to open zip file: %s", zipPath)
	}
	return l.AddSource(zipFS)
}

// AddDatabase attaches a lazy database-backed skill source.
func (l *AutoSkillLoader) AddDatabase(db *gorm.DB) (int, error) {
	before := l.dbSkillCount
	if err := l.attachDatabase(db); err != nil {
		return 0, err
	}
	if l.dbSkillCount <= before {
		return 0, nil
	}
	return l.dbSkillCount - before, nil
}

// Ensure AutoSkillLoader implements SkillLoader.
var _ SkillLoader = (*AutoSkillLoader)(nil)
