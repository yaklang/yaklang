package c2ssa

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

var (
	globalTempDir     string
	globalIncludeDirs []string
	commonCLibraries  []string
	headerCache       = make(map[string]bool)

	// reAngleIncludeAnywhere matches #include <path> anywhere on a logical line (after comment strip).
	reAngleIncludeAnywhere = regexp.MustCompile(`#\s*include\s*<([^>]+)>`)
	// reAngleIncludeLinePrefix matches a line that begins with an angle-bracket include (same idea as legacy includeLineRegexp).
	reAngleIncludeLinePrefix = regexp.MustCompile(`^\s*#\s*include\s*<[^>]+>`)
	// reQuotedIncludeAnywhere matches #include "path" anywhere on a logical line.
	reQuotedIncludeAnywhere = regexp.MustCompile(`#\s*include\s*"([^"]+)"`)
)

func newCPreprocessFS(underlying fi.FileSystem) fi.FileSystem {
	if err := setupHeaderFiles(underlying); err != nil {
		log.Warnf("setupHeaderFiles failed: %v", err)
		return underlying
	}

	hookFS := filesys.NewHookFS(underlying)
	hookFS.AddReadHook(&filesys.ReadHook{
		Matcher: filesys.SuffixMatcher(".c", ".h"),
		AfterRead: func(ctx *filesys.ReadHookContext, data []byte) ([]byte, error) {
			src := string(data)
			headers := collectStubIncludePaths(src)
			if err := ensureHeaderFiles(headers); err != nil {
				log.Warnf("ensure headers for %s failed: %v", ctx.Name, err)
			}
			preprocessed, err := preprocessCSource(src)
			if err != nil {
				log.Warnf("C macro preprocessing failed for %s: %v, using original source", ctx.Name, err)
				return data, nil
			}
			return []byte(preprocessed), nil
		},
	})
	return hookFS
}

func initTemp() error {
	tmpDir, err := os.MkdirTemp("", "c_headers_*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	globalTempDir = tmpDir
	return nil
}

func ensureIncludeBase() (string, error) {
	if globalTempDir == "" {
		if err := initTemp(); err != nil {
			return "", err
		}
	}
	includeDir := filepath.Join(globalTempDir, "include")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		return "", err
	}
	addIncludeDir(globalTempDir)
	addIncludeDir(includeDir)
	return includeDir, nil
}

func addIncludeDir(dir string) {
	if dir == "" {
		return
	}
	if !containsDir(globalIncludeDirs, dir) {
		globalIncludeDirs = append(globalIncludeDirs, dir)
	}
}

func ensureHeaderFileExists(relPath string) error {
	if relPath == "" {
		return nil
	}
	includeDir, err := ensureIncludeBase()
	if err != nil {
		return err
	}
	if headerCache[relPath] {
		return nil
	}
	targetPath := filepath.Join(includeDir, relPath)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(targetPath); err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(targetPath, []byte{}, 0o644); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	headerCache[relPath] = true
	return nil
}

func ensureCommonIncludeDirs() error {
	if len(commonCLibraries) == 0 {
		return nil
	}
	for _, std := range commonCLibraries {
		if err := ensureHeaderFileExists(std); err != nil {
			return err
		}
	}
	return nil
}

func containsDir(dirs []string, dir string) bool {
	for _, existing := range dirs {
		if existing == dir {
			return true
		}
	}
	return false
}

// isAngleBracketIncludeLineForFilter reports whether a comment-stripped logical line should be
// removed when mirroring headers (angle-bracket system-style includes only).
func isAngleBracketIncludeLineForFilter(eff string) bool {
	return reAngleIncludeLinePrefix.MatchString(strings.TrimSpace(eff))
}

// stripCCommentsFromPhysicalLine removes /* */ segments on this line, handles an inBlock state
// carried across lines, and strips a trailing // line comment (naive: does not skip // inside strings).
// Returns the remaining text on this line for include scanning / filter matching.
func stripCCommentsFromPhysicalLine(line string, inBlock *bool) string {
	rest := line
	if *inBlock {
		j := strings.Index(rest, "*/")
		if j < 0 {
			return ""
		}
		*inBlock = false
		rest = rest[j+2:]
	}
	var eff strings.Builder
	for len(rest) > 0 {
		i := strings.Index(rest, "/*")
		if i < 0 {
			eff.WriteString(rest)
			break
		}
		eff.WriteString(rest[:i])
		rest = rest[i+2:]
		j := strings.Index(rest, "*/")
		if j < 0 {
			*inBlock = true
			break
		}
		rest = rest[j+2:]
	}
	out := eff.String()
	if k := strings.Index(out, "//"); k >= 0 {
		out = out[:k]
	}
	return out
}

// collectStubIncludePaths returns deduplicated include paths for empty header stubs before -nostdinc cpp.
// It scans line-by-line, skips // and /* */ (including multiline blocks). Macro-expanded includes are not visible.
// Paths inside string literals may be false positives (conservative tradeoff).
func collectStubIncludePaths(src string) []string {
	seen := make(map[string]bool)
	var headers []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		headers = append(headers, p)
	}

	inBlock := false
	scanner := bufio.NewScanner(strings.NewReader(src))
	for scanner.Scan() {
		line := scanner.Text()
		eff := stripCCommentsFromPhysicalLine(line, &inBlock)
		for _, m := range reAngleIncludeAnywhere.FindAllStringSubmatch(eff, -1) {
			if len(m) > 1 {
				add(m[1])
			}
		}
		for _, m := range reQuotedIncludeAnywhere.FindAllStringSubmatch(eff, -1) {
			if len(m) > 1 {
				add(m[1])
			}
		}
	}
	return headers
}

func ensureHeaderFiles(headers []string) error {
	if len(headers) == 0 {
		return nil
	}
	for _, header := range headers {
		if err := ensureHeaderFileExists(header); err != nil {
			return err
		}
	}
	return nil
}

func filterSystemIncludes(src string) string {
	var builder strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(src))
	firstLine := true
	inBlock := false
	for scanner.Scan() {
		line := scanner.Text()
		eff := stripCCommentsFromPhysicalLine(line, &inBlock)
		if isAngleBracketIncludeLineForFilter(eff) {
			continue
		}
		if !firstLine {
			builder.WriteString("\n")
		}
		builder.WriteString(line)
		firstLine = false
	}
	if err := scanner.Err(); err != nil {
		return src
	}
	if firstLine {
		return src
	}
	return builder.String()
}

// setupHeaderFiles sets up header files for C preprocessing
func setupHeaderFiles(underlying fi.FileSystem) error {
	headerDirs := make(map[string]bool)

	if globalTempDir == "" {
		if err := initTemp(); err != nil {
			return err
		}
	}

	var walkDir func(string) error
	walkDir = func(dir string) error {
		entries, err := underlying.ReadDir(dir)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			filePath := underlying.Join(dir, entry.Name())

			if entry.IsDir() {
				if err := walkDir(filePath); err != nil {
					return err
				}
				continue
			}

			if underlying.Ext(entry.Name()) == ".h" || underlying.Ext(entry.Name()) == ".in" {
				content, err := underlying.ReadFile(filePath)
				if err != nil {
					continue
				}
				filtered := filterSystemIncludes(string(content))
				relPath := strings.TrimPrefix(filePath, ".")
				relPath = strings.TrimPrefix(relPath, string(underlying.GetSeparators()))
				targetPath := filepath.Join(globalTempDir, relPath)
				targetDir := filepath.Dir(targetPath)

				if err := os.MkdirAll(targetDir, 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(targetPath, []byte(filtered), 0o644); err != nil {
					return err
				}
				headerDirs[targetDir] = true
			}
		}
		return nil
	}

	if err := walkDir("."); err != nil {
		return err
	}

	// Build includeDirs list once, combining all header directories
	globalIncludeDirs = nil
	addIncludeDir(globalTempDir)
	for dir := range headerDirs {
		addIncludeDir(dir)
	}

	return nil
}

// preprocessCSource performs C macro preprocessing on source code
func preprocessCSource(src string) (string, error) {
	var preprocessorCmd string
	if globalTempDir == "" {
		if err := initTemp(); err != nil {
			return "", err
		}
	}
	if len(globalIncludeDirs) == 0 {
		if err := ensureCommonIncludeDirs(); err != nil {
			return "", err
		}
	}

	candidates := []string{"gcc", "clang", "cc"}
	for _, cmd := range candidates {
		if _, err := exec.LookPath(cmd); err == nil {
			preprocessorCmd = cmd
			break
		}
	}

	if preprocessorCmd == "" {
		return "", fmt.Errorf("c preprocessor not found: please install gcc, clang, or compatible C compiler (platform: %s/%s)", runtime.GOOS, runtime.GOARCH)
	}

	tmpFile, err := os.CreateTemp(globalTempDir, "c_preprocess_*.c")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpFileName := tmpFile.Name()
	defer os.Remove(tmpFileName)

	if _, err := tmpFile.WriteString(src); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write source to temp file: %w", err)
	}
	tmpFile.Close()

	preprocessorArgs := []string{
		"-E",
		"-P",
		"-nostdinc",
		"-Wno-everything",
	}

	// Add all include directories
	for _, includeDir := range globalIncludeDirs {
		preprocessorArgs = append(preprocessorArgs, "-I", includeDir)
	}

	preprocessorArgs = append(preprocessorArgs, tmpFileName)

	cmd := exec.Command(preprocessorCmd, preprocessorArgs...)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		if len(outputStr) > 500 {
			outputStr = outputStr[:500] + "... (truncated)"
		}
		return src, fmt.Errorf("preprocessor failed: %w\nOutput: %s", err, outputStr)
	}

	return outputStr, nil
}

// PreprocessCSource 提供对外可直接调用的 C 预处理接口。
func PreprocessCSource(src string) (string, error) {
	return preprocessCSource(src)
}
