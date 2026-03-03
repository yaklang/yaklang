package aiskillloader

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
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

	// ResourceType is "document" (content loaded into context) or "script" (path reference only).
	ResourceType string
	// AbsolutePath is the resolved absolute filesystem path for script resources.
	AbsolutePath string
	// MaterializedToArtifacts indicates the script was written to the artifacts directory
	// because no absolute path could be resolved from the skill filesystem.
	MaterializedToArtifacts bool
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

// LoadSkillResourceAsScript resolves a script file from a skill to an absolute filesystem path.
// Instead of loading the full content into the context, it creates a brief summary ViewWindow
// containing only the path reference and metadata.
//
// If the skill filesystem is local (RelLocalFs or subDirFS wrapping one), the absolute path
// is resolved directly. Otherwise, the script content is materialized to the artifacts
// directory via materializeFunc (typically invoker.EmitFileArtifactWithExt).
func (m *SkillsContextManager) LoadSkillResourceAsScript(
	skillName, filePath string,
	materializeFunc func(name, ext string, data any) string,
) (*SkillResourceLoadResult, error) {
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
		log.Infof("auto-loaded skill %q for script resource access", skillName)
	}

	state.LastAccessedAt = now
	if state.IsFolded {
		state.IsFolded = false
	}

	if filePath == "" {
		return nil, utils.Error("file_path is required for load_skill_resources (script mode)")
	}

	resolvedPaths, fuzzyMatched, err := resolveFilePath(state.Skill.FileSystem, filePath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to resolve file %q in skill %q", filePath, skillName)
	}

	if len(resolvedPaths) == 0 {
		return nil, utils.Errorf("no files found for path %q in skill %q", filePath, skillName)
	}

	resolvedPath := resolvedPaths[0]
	ext := filepath.Ext(resolvedPath)

	content, err := state.Skill.FileSystem.ReadFile(resolvedPath)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to read script file %q from skill %q", resolvedPath, skillName)
	}

	result := &SkillResourceLoadResult{
		SkillName:    skillName,
		FilePath:     resolvedPath,
		ContentSize:  len(content),
		ResourceType: "script",
		FuzzyMatched: fuzzyMatched,
		MatchedPath:  resolvedPath,
	}

	absPath, resolved := ResolveAbsoluteFilePath(state.Skill.FileSystem, resolvedPath)
	if resolved {
		result.AbsolutePath = absPath
	} else {
		if materializeFunc == nil {
			return nil, utils.Error("cannot materialize script: no artifact emitter available")
		}
		baseName := filepath.Base(resolvedPath)
		nameNoExt := strings.TrimSuffix(baseName, ext)
		artifactPath := materializeFunc(nameNoExt, ext, content)
		if artifactPath == "" {
			return nil, utils.Errorf("failed to materialize script %q to artifacts", resolvedPath)
		}
		result.AbsolutePath = artifactPath
		result.MaterializedToArtifacts = true
		log.Infof("materialized script %q from skill %q to artifacts: %s", resolvedPath, skillName, artifactPath)
	}

	typeLabel := ScriptTypeLabel(ext)
	viewKey := "script_ref:" + resolvedPath
	var summaryBuf strings.Builder
	summaryBuf.WriteString("[Script Resource Reference]\n")
	summaryBuf.WriteString(fmt.Sprintf("File: %s\n", filepath.Base(resolvedPath)))
	summaryBuf.WriteString(fmt.Sprintf("Type: %s\n", typeLabel))
	summaryBuf.WriteString(fmt.Sprintf("Absolute Path: %s\n", result.AbsolutePath))
	summaryBuf.WriteString(fmt.Sprintf("Size: %d bytes\n", len(content)))
	summaryBuf.WriteString(fmt.Sprintf("Source: skill %q / %s\n", skillName, resolvedPath))
	if result.MaterializedToArtifacts {
		summaryBuf.WriteString("Note: Script materialized from virtual filesystem to artifacts directory.\n")
	}
	summaryBuf.WriteString("Note: Use this absolute path directly in shell commands.\n")

	nonce := GenerateNonce(skillName, viewKey)
	vw := NewViewWindow(skillName, viewKey, summaryBuf.String(), nonce)
	state.ViewWindows[viewKey] = vw
	result.TotalLines = vw.TotalLines()

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

var scriptExtensions = map[string]bool{
	".sh": true, ".bash": true, ".zsh": true,
	".py": true, ".python": true,
	".yak": true,
	".js": true, ".ts": true,
	".go": true,
	".rb": true, ".pl": true, ".lua": true,
	".ps1": true, ".bat": true, ".cmd": true,
	".r": true, ".php": true, ".swift": true,
}

// IsScriptExtension returns true if the file extension indicates a script/executable file
// that should be loaded as a path reference rather than content.
func IsScriptExtension(ext string) bool {
	return scriptExtensions[strings.ToLower(ext)]
}

// ScriptTypeLabel returns a human-readable label for a script file extension.
func ScriptTypeLabel(ext string) string {
	labels := map[string]string{
		".sh": "shell script", ".bash": "bash script", ".zsh": "zsh script",
		".py": "python script", ".python": "python script",
		".yak": "yak script",
		".js": "javascript", ".ts": "typescript",
		".go": "go source",
		".rb": "ruby script", ".pl": "perl script", ".lua": "lua script",
		".ps1": "powershell script", ".bat": "batch script", ".cmd": "cmd script",
		".r": "R script", ".php": "php script", ".swift": "swift source",
	}
	if label, ok := labels[strings.ToLower(ext)]; ok {
		return label
	}
	return "script"
}

// ResolveAbsoluteFilePath attempts to resolve a file's absolute path from a skill filesystem.
// It handles RelLocalFs (via Root()) and subDirFS (by recursively unwrapping the parent).
// Returns the absolute path and true if resolution succeeds, or empty string and false otherwise.
func ResolveAbsoluteFilePath(skillFS fi.FileSystem, filePath string) (string, bool) {
	type rooter interface {
		Root() string
	}

	if r, ok := skillFS.(rooter); ok {
		absPath := filepath.Join(r.Root(), filePath)
		if _, err := os.Stat(absPath); err == nil {
			return absPath, true
		}
	}

	if sfs, ok := skillFS.(*subDirFS); ok {
		return ResolveAbsoluteFilePath(sfs.parent, filepath.Join(sfs.subDir, filePath))
	}

	return "", false
}

// SkillGrepMatch represents a single match found during a grep operation on skill files.
type SkillGrepMatch struct {
	SkillName string
	FilePath  string
	LineNo    int
	ColNo     int
	LineText  string
	Context   string // pre-formatted context from memedit.GetTextContextWithPrompt
}

// SkillGrepResult holds the aggregated result of a grep operation across skill files.
type SkillGrepResult struct {
	Pattern        string
	SkillName      string // empty if searched all skills
	TotalMatches   int
	IsTruncated    bool
	Matches        []SkillGrepMatch
	SearchedSkills []string
	SearchedFiles  int
}

const (
	grepMaxMatches    = 50
	grepContextLines  = 3
	grepMaxOutputSize = 30 * 1024
)

// GrepSkillResources searches for a regex/string pattern across files in one or all skills.
// If skillName is empty, all available skills (from loader.AllSkillMetas) are searched.
// Uses memedit.MemEditor for pattern matching and context extraction.
func (m *SkillsContextManager) GrepSkillResources(pattern, skillName string) (*SkillGrepResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.loader == nil {
		return nil, utils.Error("skills context manager: no loader configured")
	}

	if pattern == "" {
		return nil, utils.Error("grep pattern is required")
	}

	result := &SkillGrepResult{
		Pattern:   pattern,
		SkillName: skillName,
	}

	var skillsToSearch []string
	if skillName != "" {
		skillsToSearch = []string{skillName}
	} else {
		for _, meta := range m.loader.AllSkillMetas() {
			skillsToSearch = append(skillsToSearch, meta.Name)
		}
	}

	for _, sn := range skillsToSearch {
		if result.TotalMatches >= grepMaxMatches {
			result.IsTruncated = true
			break
		}

		skillFS, err := m.loader.GetFileSystem(sn)
		if err != nil {
			log.Warnf("grep: cannot access filesystem for skill %q: %v", sn, err)
			continue
		}
		result.SearchedSkills = append(result.SearchedSkills, sn)

		_ = filesys.SimpleRecursive(
			filesys.WithFileSystem(skillFS),
			filesys.WithFileStat(func(pathname string, info fs.FileInfo) error {
				if result.TotalMatches >= grepMaxMatches {
					result.IsTruncated = true
					return utils.Error("max matches reached")
				}

				if info.Size() > 512*1024 {
					return nil
				}

				content, readErr := skillFS.ReadFile(pathname)
				if readErr != nil {
					return nil
				}

				if isBinaryContent(content) {
					return nil
				}

				result.SearchedFiles++
				m.grepFileContent(sn, pathname, string(content), pattern, result)
				return nil
			}),
		)
	}

	if len(result.Matches) > 0 {
		viewContent := FormatGrepResultForView(result)
		viewKey := fmt.Sprintf("grep:%s", pattern)
		if skillName != "" {
			viewKey = fmt.Sprintf("grep:%s@%s", pattern, skillName)
		}
		nonce := GenerateNonce("grep", viewKey)
		vw := NewViewWindow("grep-results", viewKey, viewContent, nonce)
		state, ok := m.loadedSkills.Get(result.SearchedSkills[0])
		if ok {
			state.ViewWindows[viewKey] = vw
			m.contextSizeDirty = true
			m.ensureContextFits()
		}
	}

	return result, nil
}

// grepFileContent uses memedit to search a single file's content for the pattern.
func (m *SkillsContextManager) grepFileContent(
	skillName, filePath, content, pattern string,
	result *SkillGrepResult,
) {
	editor := memedit.NewMemEditor(content)

	matchErr := editor.FindRegexpRange(pattern, func(r *memedit.Range) error {
		if result.TotalMatches >= grepMaxMatches {
			result.IsTruncated = true
			return memedit.ErrorStop
		}

		startPos := r.GetStart()
		lineText, _ := editor.GetLine(startPos.GetLine())

		ctx := editor.GetTextContextWithPrompt(r, grepContextLines)

		result.Matches = append(result.Matches, SkillGrepMatch{
			SkillName: skillName,
			FilePath:  filePath,
			LineNo:    startPos.GetLine(),
			ColNo:     startPos.GetColumn(),
			LineText:  strings.TrimRight(lineText, "\n\r"),
			Context:   ctx,
		})
		result.TotalMatches++
		return nil
	})

	if matchErr != nil && matchErr != memedit.ErrorStop {
		editor.FindStringRange(pattern, func(r *memedit.Range) error {
			if result.TotalMatches >= grepMaxMatches {
				result.IsTruncated = true
				return memedit.ErrorStop
			}

			startPos := r.GetStart()
			lineText, _ := editor.GetLine(startPos.GetLine())

			ctx := editor.GetTextContextWithPrompt(r, grepContextLines)

			result.Matches = append(result.Matches, SkillGrepMatch{
				SkillName: skillName,
				FilePath:  filePath,
				LineNo:    startPos.GetLine(),
				ColNo:     startPos.GetColumn(),
				LineText:  strings.TrimRight(lineText, "\n\r"),
				Context:   ctx,
			})
			result.TotalMatches++
			return nil
		})
	}
}

// isBinaryContent checks whether content is likely binary by looking for NUL bytes.
func isBinaryContent(data []byte) bool {
	checkLen := 8192
	if len(data) < checkLen {
		checkLen = len(data)
	}
	for i := 0; i < checkLen; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// FormatGrepResultForView formats grep results for display in a ViewWindow.
func FormatGrepResultForView(result *SkillGrepResult) string {
	var buf strings.Builder

	scope := "all skills"
	if result.SkillName != "" {
		scope = fmt.Sprintf("skill '%s'", result.SkillName)
	}
	buf.WriteString(fmt.Sprintf("=== Grep Results for pattern %q in %s ===\n", result.Pattern, scope))
	buf.WriteString(fmt.Sprintf("Searched: %d skills, %d files | Found: %d matches",
		len(result.SearchedSkills), result.SearchedFiles, result.TotalMatches))
	if result.IsTruncated {
		buf.WriteString(fmt.Sprintf(" (showing first %d)", len(result.Matches)))
	}
	buf.WriteString("\n\n")

	currentFile := ""
	for _, match := range result.Matches {
		fileKey := match.SkillName + "/" + match.FilePath
		if fileKey != currentFile {
			if currentFile != "" {
				buf.WriteString("\n")
			}
			buf.WriteString(fmt.Sprintf("--- %s ---\n", fileKey))
			currentFile = fileKey
		}

		if match.Context != "" {
			buf.WriteString(match.Context)
			if !strings.HasSuffix(match.Context, "\n") {
				buf.WriteString("\n")
			}
		} else {
			buf.WriteString(fmt.Sprintf("  %d| %s\n", match.LineNo, match.LineText))
		}

		if buf.Len() > grepMaxOutputSize {
			buf.WriteString("\n... (output truncated due to size limit)\n")
			result.IsTruncated = true
			break
		}
	}

	return buf.String()
}

// FormatGrepSummary returns a brief one-line summary of a grep result for timeline/feedback.
func FormatGrepSummary(result *SkillGrepResult) string {
	scope := "all skills"
	if result.SkillName != "" {
		scope = fmt.Sprintf("skill '%s'", result.SkillName)
	}
	msg := fmt.Sprintf("Grep for %q in %s: %d matches across %d files in %d skills",
		result.Pattern, scope, result.TotalMatches, result.SearchedFiles, len(result.SearchedSkills))
	if result.IsTruncated {
		msg += fmt.Sprintf(" (truncated, showing %d)", len(result.Matches))
	}
	return msg
}

// FormatResourceLoadSummary returns a human-readable summary for stream logging.
func FormatResourceLoadSummary(result *SkillResourceLoadResult) string {
	var buf strings.Builder

	if result.ResourceType == "script" {
		buf.WriteString(fmt.Sprintf("Loaded script resource from skill '%s'", result.SkillName))
		if result.FuzzyMatched {
			buf.WriteString(fmt.Sprintf(" (fuzzy matched to '%s')", result.MatchedPath))
		}
		buf.WriteString(fmt.Sprintf(": %s", result.FilePath))
		buf.WriteString(fmt.Sprintf(" | %d bytes", result.ContentSize))
		buf.WriteString(fmt.Sprintf(" | path: %s", result.AbsolutePath))
		if result.MaterializedToArtifacts {
			buf.WriteString(" (materialized to artifacts)")
		}
		return buf.String()
	}

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
