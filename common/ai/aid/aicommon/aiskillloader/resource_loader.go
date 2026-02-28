package aiskillloader

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// SkillResourceLoadResult holds the result of loading a skill resource.
type SkillResourceLoadResult struct {
	SkillName    string
	FilePath     string
	ContentSize  int
	TotalLines   int
	IsTruncated  bool
	FuzzyMatched bool
	MatchedPath  string
}

// ParseSkillResourcePath parses a resource path in the format "@skill_name/path/to/file".
// Returns the skill name and the file path within the skill.
func ParseSkillResourcePath(resourcePath string) (skillName, filePath string, err error) {
	resourcePath = strings.TrimSpace(resourcePath)
	if !strings.HasPrefix(resourcePath, "@") {
		return "", "", utils.Errorf("resource path must start with '@', got: %q", resourcePath)
	}
	resourcePath = strings.TrimPrefix(resourcePath, "@")
	parts := strings.SplitN(resourcePath, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", "", utils.Error("resource path must contain a skill name after '@'")
	}
	skillName = parts[0]
	if len(parts) > 1 {
		filePath = parts[1]
	}
	return skillName, filePath, nil
}

// LoadSkillResource loads a file from a skill into the context as a ViewWindow.
// If the exact path is not found, it performs fuzzy matching:
// 1. Strip the extension from the filename
// 2. Recursively search the skill filesystem for files/dirs matching the base name
// 3. Load the first matching file, or all files in a matching directory
func (m *SkillsContextManager) LoadSkillResource(skillName, filePath string) (*SkillResourceLoadResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.loader == nil {
		return nil, utils.Error("skills context manager: no loader configured")
	}

	now := time.Now()

	state, ok := m.loadedSkills.Get(skillName)
	if !ok {
		loaded, err := m.loader.LoadSkill(skillName)
		if err != nil {
			return nil, utils.Wrapf(err, "skill %q is not loaded and cannot be loaded on demand", skillName)
		}
		transformedContent := TransformIncludesToResourceHints(loaded.SkillMDContent, skillName)
		nonce := GenerateNonce(skillName, skillMDFilename)
		skillMDWindow := NewViewWindow(skillName, skillMDFilename, transformedContent, nonce)
		state = &skillContextState{
			Skill:    loaded,
			IsFolded: false,
			ViewWindows: map[string]*ViewWindow{
				skillMDFilename: skillMDWindow,
			},
			LastAccessedAt: now,
		}
		m.loadedSkills.Set(skillName, state)
		m.contextSizeDirty = true
		log.Infof("auto-loaded skill %q for resource access", skillName)
	}

	state.LastAccessedAt = now

	if state.IsFolded {
		state.IsFolded = false
	}

	if filePath == "" {
		return nil, utils.Error("file_path is required for load_skill_resources")
	}

	resolvedPaths, fuzzyMatched, err := resolveFilePath(state.Skill.FileSystem, filePath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to resolve file %q in skill %q", filePath, skillName)
	}

	var result *SkillResourceLoadResult
	totalContentSize := 0

	for _, resolvedPath := range resolvedPaths {
		if _, exists := state.ViewWindows[resolvedPath]; exists {
			if result == nil {
				vw := state.ViewWindows[resolvedPath]
				result = &SkillResourceLoadResult{
					SkillName:    skillName,
					FilePath:     resolvedPath,
					ContentSize:  len(strings.Join(vw.Lines, "\n")),
					TotalLines:   vw.TotalLines(),
					IsTruncated:  vw.IsTruncated,
					FuzzyMatched: fuzzyMatched,
					MatchedPath:  resolvedPath,
				}
			}
			continue
		}

		content, err := state.Skill.FileSystem.ReadFile(resolvedPath)
		if err != nil {
			log.Warnf("failed to read file %q from skill %q: %v", resolvedPath, skillName, err)
			continue
		}

		nonce := GenerateNonce(skillName, resolvedPath)
		vw := NewViewWindow(skillName, resolvedPath, string(content), nonce)
		state.ViewWindows[resolvedPath] = vw
		totalContentSize += len(content)

		if result == nil {
			result = &SkillResourceLoadResult{
				SkillName:    skillName,
				FilePath:     resolvedPath,
				ContentSize:  len(content),
				TotalLines:   vw.TotalLines(),
				IsTruncated:  vw.IsTruncated,
				FuzzyMatched: fuzzyMatched,
				MatchedPath:  resolvedPath,
			}
		} else {
			result.ContentSize += len(content)
			result.TotalLines += vw.TotalLines()
		}
	}

	if result == nil {
		return nil, utils.Errorf("no files found for path %q in skill %q", filePath, skillName)
	}

	m.contextSizeDirty = true
	m.ensureContextFits()
	return result, nil
}

// resolveFilePath attempts to resolve a file path within a skill filesystem.
// Returns a list of resolved file paths, whether fuzzy matching was used, and any error.
func resolveFilePath(skillFS fi.FileSystem, requestedPath string) ([]string, bool, error) {
	exists, _ := skillFS.Exists(requestedPath)
	if exists {
		info, err := skillFS.Stat(requestedPath)
		if err == nil && info.IsDir() {
			files, err := collectDirFiles(skillFS, requestedPath)
			if err != nil || len(files) == 0 {
				return nil, false, utils.Errorf("directory %q exists but contains no readable files", requestedPath)
			}
			return files, false, nil
		}
		return []string{requestedPath}, false, nil
	}

	baseName := filepath.Base(requestedPath)
	nameWithoutExt := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	if nameWithoutExt == "" {
		return nil, false, utils.Errorf("cannot resolve empty filename from path %q", requestedPath)
	}

	var candidates []string
	_ = filesys.SimpleRecursive(
		filesys.WithFileSystem(skillFS),
		filesys.WithFileStat(func(pathname string, info fs.FileInfo) error {
			fileBase := filepath.Base(pathname)
			fileNameNoExt := strings.TrimSuffix(fileBase, filepath.Ext(fileBase))
			if strings.EqualFold(fileNameNoExt, nameWithoutExt) || strings.EqualFold(fileBase, baseName) {
				candidates = append(candidates, pathname)
			}
			return nil
		}),
		filesys.WithDirStat(func(pathname string, info fs.FileInfo) error {
			dirBase := filepath.Base(pathname)
			if strings.EqualFold(dirBase, nameWithoutExt) {
				files, err := collectDirFiles(skillFS, pathname)
				if err == nil {
					candidates = append(candidates, files...)
				}
			}
			return nil
		}),
	)

	if len(candidates) == 0 {
		return nil, false, utils.Errorf("file %q not found in skill (also tried fuzzy matching for %q)", requestedPath, nameWithoutExt)
	}

	seen := make(map[string]bool)
	var unique []string
	for _, c := range candidates {
		if !seen[c] {
			seen[c] = true
			unique = append(unique, c)
		}
	}

	return unique, true, nil
}

// collectDirFiles collects all files within a directory (non-recursive).
func collectDirFiles(skillFS fi.FileSystem, dirPath string) ([]string, error) {
	entries, err := skillFS.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, skillFS.Join(dirPath, entry.Name()))
		}
	}
	return files, nil
}

// FormatResourceLoadSummary returns a human-readable summary for stream logging.
func FormatResourceLoadSummary(result *SkillResourceLoadResult) string {
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("Loaded resource from skill '%s'", result.SkillName))
	if result.FuzzyMatched {
		buf.WriteString(fmt.Sprintf(" (fuzzy matched to '%s')", result.MatchedPath))
	}
	buf.WriteString(fmt.Sprintf(": %s", result.FilePath))
	buf.WriteString(fmt.Sprintf(" | %d lines, %.1fKB", result.TotalLines, float64(result.ContentSize)/1024))
	if result.IsTruncated {
		buf.WriteString(" (truncated)")
	}
	return buf.String()
}
