package aiskillloader

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// AutoSkillLoader discovers and loads skills by recursively traversing multiple
// file system sources. It intentionally focuses on filesystem/read concerns only.
type AutoSkillLoader struct {
	mu     sync.RWMutex
	skills map[string]*skillEntry

	sources []fi.FileSystem
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

// NewAutoSkillLoader creates an AutoSkillLoader that recursively discovers
// SKILL.md files from multiple sources.
func NewAutoSkillLoader(opts ...AutoSkillLoaderOption) (*AutoSkillLoader, error) {
	l := &AutoSkillLoader{
		skills: make(map[string]*skillEntry),
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

			parentDir := path.Dir(pathname)
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
					dirName: path.Base(parentDir),
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

// --- SkillLoader interface implementation ---

// LoadSkill loads a specific skill by name.
func (l *AutoSkillLoader) LoadSkill(name string) (*LoadedSkill, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	entry, ok := l.skills[name]
	if !ok {
		return nil, utils.Errorf("skill %q not found", name)
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
	defer l.mu.RUnlock()

	entry, ok := l.skills[name]
	if !ok {
		return nil, utils.Errorf("skill %q not found", name)
	}
	return entry.fs, nil
}

// HasSkills returns true if at least one skill is registered.
func (l *AutoSkillLoader) HasSkills() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.skills) > 0
}

// AllSkillMetas returns metadata for all available skills.
func (l *AutoSkillLoader) AllSkillMetas() []*SkillMeta {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*SkillMeta, 0, len(l.skills))
	for _, entry := range l.skills {
		result = append(result, entry.meta)
	}
	return result
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
func (l *AutoSkillLoader) AddLocalDir(dirPath string) (int, error) {
	if !utils.IsDir(dirPath) {
		return 0, utils.Errorf("skill directory does not exist: %s", dirPath)
	}
	localFS := filesys.NewRelLocalFs(dirPath)
	return l.AddSource(localFS)
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

// Ensure AutoSkillLoader implements SkillLoader.
var _ SkillLoader = (*AutoSkillLoader)(nil)
