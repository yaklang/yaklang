package aiskillloader

import (
	"io/fs"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

const skillMDFilename = "SKILL.md"

// skillEntry holds a single skill's filesystem and parsed metadata.
type skillEntry struct {
	fs   fi.FileSystem
	meta *SkillMeta
}

// FSSkillLoader implements SkillLoader on top of filesys_interface.FileSystem.
// It expects the root filesystem to contain subdirectories, each representing a skill
// with a SKILL.md file inside.
//
// Example layout:
//
//	root/
//	  deploy-app/
//	    SKILL.md
//	    scripts/
//	  code-review/
//	    SKILL.md
type FSSkillLoader struct {
	mu     sync.RWMutex
	skills map[string]*skillEntry
}

// NewFSSkillLoader creates a SkillLoader from a root FileSystem.
// It scans the root directory for subdirectories containing SKILL.md.
func NewFSSkillLoader(rootFS fi.FileSystem) (*FSSkillLoader, error) {
	loader := &FSSkillLoader{
		skills: make(map[string]*skillEntry),
	}

	entries, err := rootFS.ReadDir(".")
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read skill root directory")
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		skillMDPath := rootFS.Join(dirName, skillMDFilename)

		exists, _ := rootFS.Exists(skillMDPath)
		if !exists {
			log.Debugf("skip directory %s: no %s found", dirName, skillMDFilename)
			continue
		}

		content, err := rootFS.ReadFile(skillMDPath)
		if err != nil {
			log.Warnf("failed to read %s in %s: %v", skillMDFilename, dirName, err)
			continue
		}

		meta, err := ParseSkillMeta(string(content))
		if err != nil {
			log.Warnf("failed to parse %s in %s: %v", skillMDFilename, dirName, err)
			continue
		}

		// Create a sub-filesystem view for this skill
		subFS := &subDirFS{
			parent:  rootFS,
			subDir:  dirName,
			dirName: dirName,
		}

		loader.skills[meta.Name] = &skillEntry{
			fs:   subFS,
			meta: meta,
		}
		log.Infof("loaded skill: %s from directory: %s", meta.Name, dirName)
	}

	return loader, nil
}

// LoadSkill loads a specific skill by name.
func (l *FSSkillLoader) LoadSkill(name string) (*LoadedSkill, error) {
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

// ListSkills returns metadata for all available skills.
func (l *FSSkillLoader) ListSkills() ([]*SkillMeta, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*SkillMeta, 0, len(l.skills))
	for _, entry := range l.skills {
		result = append(result, entry.meta)
	}
	return result, nil
}

// SearchSkills searches skills by keyword against name and description.
func (l *FSSkillLoader) SearchSkills(query string) ([]*SkillMeta, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	query = strings.ToLower(query)
	var result []*SkillMeta
	for _, entry := range l.skills {
		nameMatch := strings.Contains(strings.ToLower(entry.meta.Name), query)
		descMatch := strings.Contains(strings.ToLower(entry.meta.Description), query)
		if nameMatch || descMatch {
			result = append(result, entry.meta)
		}
	}
	return result, nil
}

// GetFileSystem returns the read-only filesystem for a specific skill.
func (l *FSSkillLoader) GetFileSystem(name string) (fi.FileSystem, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	entry, ok := l.skills[name]
	if !ok {
		return nil, utils.Errorf("skill %q not found", name)
	}
	return entry.fs, nil
}

// HasSkills returns true if at least one skill is registered.
func (l *FSSkillLoader) HasSkills() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.skills) > 0
}

// Ensure FSSkillLoader implements SkillLoader.
var _ SkillLoader = (*FSSkillLoader)(nil)

// subDirFS wraps a parent FileSystem to provide a view rooted at a subdirectory.
// It implements ReadOnlyFileSystem and PathFileSystem from filesys_interface.
type subDirFS struct {
	parent  fi.FileSystem
	subDir  string
	dirName string
}

func (s *subDirFS) Open(name string) (fs.File, error) {
	return s.parent.Open(s.parent.Join(s.subDir, name))
}

func (s *subDirFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return s.parent.ReadDir(s.parent.Join(s.subDir, name))
}

func (s *subDirFS) ReadFile(name string) ([]byte, error) {
	return s.parent.ReadFile(s.parent.Join(s.subDir, name))
}

func (s *subDirFS) OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error) {
	return s.parent.OpenFile(s.parent.Join(s.subDir, name), flag, perm)
}

func (s *subDirFS) Stat(name string) (fs.FileInfo, error) {
	return s.parent.Stat(s.parent.Join(s.subDir, name))
}

func (s *subDirFS) ExtraInfo(key string) map[string]any {
	return s.parent.ExtraInfo(key)
}

func (s *subDirFS) GetSeparators() rune             { return s.parent.GetSeparators() }
func (s *subDirFS) Join(elem ...string) string       { return s.parent.Join(elem...) }
func (s *subDirFS) Base(p string) string             { return s.parent.Base(p) }
func (s *subDirFS) PathSplit(p string) (string, string) { return s.parent.PathSplit(p) }
func (s *subDirFS) Ext(p string) string              { return s.parent.Ext(p) }
func (s *subDirFS) IsAbs(p string) bool              { return s.parent.IsAbs(p) }
func (s *subDirFS) Getwd() (string, error)           { return s.dirName, nil }
func (s *subDirFS) Exists(p string) (bool, error) {
	return s.parent.Exists(s.parent.Join(s.subDir, p))
}
func (s *subDirFS) Rel(basepath, targpath string) (string, error) {
	return s.parent.Rel(basepath, targpath)
}

// Write operations are unsupported (read-only).
func (s *subDirFS) Rename(string, string) error              { return utils.Error("read-only skill filesystem") }
func (s *subDirFS) WriteFile(string, []byte, fs.FileMode) error { return utils.Error("read-only skill filesystem") }
func (s *subDirFS) Delete(string) error                      { return utils.Error("read-only skill filesystem") }
func (s *subDirFS) MkdirAll(string, fs.FileMode) error       { return utils.Error("read-only skill filesystem") }

var _ fi.FileSystem = (*subDirFS)(nil)
