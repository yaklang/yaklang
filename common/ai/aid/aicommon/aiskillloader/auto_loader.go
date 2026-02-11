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

// Ensure AutoSkillLoader implements SkillLoader.
var _ SkillLoader = (*AutoSkillLoader)(nil)

/*
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

// Ensure AutoSkillLoader implements SkillLoader.
var _ SkillLoader = (*AutoSkillLoader)(nil)
package aiskillloader

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// AutoSkillLoader discovers and loads skills by recursively traversing multiple
// file system sources. It implements SkillLoader and adds BM25 and AI-based search.
type AutoSkillLoader struct {
	mu     sync.RWMutex
	skills map[string]*skillEntry

	sources         []fi.FileSystem
	db              *gorm.DB
	searchAICallback SkillSearchAICallback
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
func WithAutoLoad_FileSystem(fs fi.FileSystem) AutoSkillLoaderOption {
	return func(l *AutoSkillLoader) error {
		if fs == nil {
			return utils.Error("filesystem is nil")
		}
		l.sources = append(l.sources, fs)
		return nil
	}
}

// WithAutoLoad_Database sets the persistence database for skill caching and FTS5 search.
func WithAutoLoad_Database(db *gorm.DB) AutoSkillLoaderOption {
	return func(l *AutoSkillLoader) error {
		if db == nil {
			return utils.Error("database is nil")
		}
		l.db = db
		return nil
	}
}

// WithAutoLoad_SearchAICallback sets the callback for AI-based skill search.
// The callback receives a prompt and JSON schema, invokes LiteForge,
// and returns parsed SkillSelection results.
func WithAutoLoad_SearchAICallback(cb SkillSearchAICallback) AutoSkillLoaderOption {
	return func(l *AutoSkillLoader) error {
		l.searchAICallback = cb
		return nil
	}
}

// NewAutoSkillLoader creates an AutoSkillLoader that recursively discovers
// SKILL.md files from multiple sources and optionally persists them to a database.
func NewAutoSkillLoader(opts ...AutoSkillLoaderOption) (*AutoSkillLoader, error) {
	l := &AutoSkillLoader{
		skills: make(map[string]*skillEntry),
	}

	for _, opt := range opts {
		if err := opt(l); err != nil {
			return nil, utils.Wrapf(err, "failed to configure AutoSkillLoader")
		}
	}

	// Ensure DB schema and FTS5 if database is provided
	if l.db != nil {
		l.db.AutoMigrate(&schema.AISkill{})
		if err := yakit.EnsureAISkillFTS5(l.db); err != nil {
			log.Warnf("failed to setup ai_skills FTS5 index: %v", err)
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

			// pathname is something like "deploy-app/SKILL.md" or "deep/nested/skill/SKILL.md"
			parentDir := path.Dir(pathname)
			if parentDir == "." {
				parentDir = ""
			}

			// Read SKILL.md content
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

			// Create sub-filesystem view for this skill
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

			// Compute content hash for change detection
			hash := computeSkillHash(skillFS)

			l.mu.Lock()
			// Register the skill (last-write-wins for duplicates across sources)
			l.skills[meta.Name] = &skillEntry{
				fs:   skillFS,
				meta: meta,
			}
			l.mu.Unlock()

			// Persist to database if available
			if l.db != nil {
				l.persistSkill(meta, hash, string(content), pathname)
			}

			log.Infof("auto-discovered skill: %s from %s", meta.Name, pathname)
			return nil
		}),
	)
}

// persistSkill saves or updates a skill in the database, using hash for deduplication.
func (l *AutoSkillLoader) persistSkill(meta *SkillMeta, hash, rawContent, sourcePath string) {
	// Check if skill with same name and hash already exists
	existing, err := yakit.GetAISkillByName(l.db, meta.Name)
	if err == nil && existing != nil && existing.Hash == hash {
		// No change detected, skip update
		return
	}

	// Build keywords from metadata
	var keywords []string
	for k, v := range meta.Metadata {
		keywords = append(keywords, k, v)
	}

	skill := &schema.AISkill{
		Name:                   meta.Name,
		Description:            meta.Description,
		License:                meta.License,
		Keywords:               strings.Join(keywords, ","),
		Body:                   meta.Body,
		Hash:                   hash,
		SourcePath:             sourcePath,
		DisableModelInvocation: meta.DisableModelInvocation,
	}

	if err := yakit.CreateOrUpdateAISkill(l.db, skill); err != nil {
		log.Warnf("failed to persist skill %q to database: %v", meta.Name, err)
	}
}

// computeSkillHash computes a deterministic hash for a skill's filesystem.
// It collects all files <=10KB (sorted by path), SHA256s each,
// concatenates the hex digests, and SHA256s the concatenation.
func computeSkillHash(skillFS fi.FileSystem) string {
	const maxFileSize = 10 * 1024 // 10KB

	// Collect all file paths recursively
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

	// Sort paths for deterministic ordering
	sort.Strings(filePaths)

	// SHA256 each file and concatenate hex digests
	var combined strings.Builder
	for _, fp := range filePaths {
		content, err := skillFS.ReadFile(fp)
		if err != nil {
			continue
		}
		h := sha256.Sum256(content)
		combined.WriteString(fmt.Sprintf("%x", h))
	}

	// Final SHA256 of concatenated digests
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

// ListSkills returns metadata for all available skills.
func (l *AutoSkillLoader) ListSkills() ([]*SkillMeta, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*SkillMeta, 0, len(l.skills))
	for _, entry := range l.skills {
		result = append(result, entry.meta)
	}
	return result, nil
}

// SearchSkills searches skills by keyword against name and description (simple substring).
func (l *AutoSkillLoader) SearchSkills(query string) ([]*SkillMeta, error) {
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

// Ensure AutoSkillLoader implements SkillLoader.
var _ SkillLoader = (*AutoSkillLoader)(nil)

// --- Extended search methods ---

// SearchKeywordBM25 uses SQLite FTS5 BM25 ranking to search skills by keyword.
// If a persistent DB is configured, it searches against the indexed ai_skills table.
// Otherwise, it creates an in-memory SQLite database, populates it with current skills,
// builds a temporary FTS5 index, and searches against that.
func (l *AutoSkillLoader) SearchKeywordBM25(query string, limit int) ([]*SkillMeta, error) {
	if query == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	l.mu.RLock()
	skillCount := len(l.skills)
	l.mu.RUnlock()

	if skillCount == 0 {
		return nil, nil
	}

	filter := &yakit.AISkillSearchFilter{
		Keywords: query,
	}

	var db *gorm.DB

	if l.db != nil {
		db = l.db
	} else {
		// Create a temporary in-memory SQLite database
		memDB, err := gorm.Open("sqlite3", ":memory:")
		if err != nil {
			return nil, utils.Wrapf(err, "failed to create in-memory SQLite for BM25 search")
		}
		defer memDB.Close()

		memDB.AutoMigrate(&schema.AISkill{})
		if err := yakit.EnsureAISkillFTS5(memDB); err != nil {
			log.Warnf("failed to setup FTS5 on in-memory DB: %v", err)
		}

		// Populate with current skills
		l.mu.RLock()
		for _, entry := range l.skills {
			skill := &schema.AISkill{
				Name:        entry.meta.Name,
				Description: entry.meta.Description,
				Keywords:    buildKeywordsString(entry.meta),
				Body:        entry.meta.Body,
			}
			memDB.Create(skill)
		}
		l.mu.RUnlock()

		// Rebuild FTS5 index after bulk insert
		_ = yakit.EnsureAISkillFTS5(memDB)

		db = memDB
	}

	results, err := yakit.SearchAISkillBM25(db, filter, limit, 0)
	if err != nil {
		return nil, utils.Wrapf(err, "BM25 search failed")
	}

	// Convert back to SkillMeta
	l.mu.RLock()
	defer l.mu.RUnlock()

	var metas []*SkillMeta
	for _, r := range results {
		if entry, ok := l.skills[r.Name]; ok {
			metas = append(metas, entry.meta)
		}
	}
	return metas, nil
}

// SearchByAI uses the configured callback to select the most relevant skills
// for a user's task. This is the slow search path (1-5s) that leverages AI reasoning.
func (l *AutoSkillLoader) SearchByAI(userNeed string) ([]*SkillMeta, error) {
	if l.searchAICallback == nil {
		return nil, utils.Error("search AI callback is not configured")
	}

	skills, err := l.ListSkills()
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, nil
	}

	return SearchByAI(skills, userNeed, l.searchAICallback)
}

// buildKeywordsString builds a comma-separated keywords string from SkillMeta.
func buildKeywordsString(meta *SkillMeta) string {
	var parts []string
	for k, v := range meta.Metadata {
		parts = append(parts, k, v)
	}
	return strings.Join(parts, ",")
}
*/
