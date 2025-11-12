package filesys

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type PreprocessedCFS struct {
	underlying  fi.FileSystem
	enabled     bool
	includeDirs []string
}

var _ fi.FileSystem = (*PreprocessedCFS)(nil)

var (
	globalTempDir     string
	globalIncludeDirs []string
	commonCLibraries  []string
)

func initTemp() error {
	tmpDir, err := os.MkdirTemp("", "c_headers_*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	globalTempDir = tmpDir
	return nil
}

// SetCommonCLibraries registers the common C header names that should be ensured in the preprocessed filesystem.
func SetCommonCLibraries(libs []string) {
	if libs == nil {
		commonCLibraries = nil
		return
	}
	commonCLibraries = append([]string(nil), libs...)
	globalIncludeDirs = nil
}

func ensureCommonIncludeDirs() error {
	if len(commonCLibraries) == 0 {
		return nil
	}
	if globalTempDir == "" {
		if err := initTemp(); err != nil {
			return err
		}
	}
	includeDir := filepath.Join(globalTempDir, "include")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		return err
	}
	for _, std := range commonCLibraries {
		targetPath := filepath.Join(includeDir, std)
		dirPath := filepath.Dir(targetPath)
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
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
	}
	if len(globalIncludeDirs) == 0 {
		globalIncludeDirs = []string{globalTempDir, includeDir}
	} else {
		if !containsDir(globalIncludeDirs, globalTempDir) {
			globalIncludeDirs = append(globalIncludeDirs, globalTempDir)
		}
		if !containsDir(globalIncludeDirs, includeDir) {
			globalIncludeDirs = append(globalIncludeDirs, includeDir)
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

func filterSystemIncludes(src string) string {
	var builder strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(src))
	firstLine := true
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#include") {
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
				walkDir(filePath)
			} else if underlying.Ext(entry.Name()) == ".h" || underlying.Ext(entry.Name()) == ".in" {
				if content, err := underlying.ReadFile(filePath); err == nil {
					filtered := filterSystemIncludes(string(content))
					relPath := strings.TrimPrefix(filePath, ".")
					relPath = strings.TrimPrefix(relPath, string(underlying.GetSeparators()))
					targetPath := filepath.Join(globalTempDir, relPath)
					targetDir := filepath.Dir(targetPath)

					os.MkdirAll(targetDir, 0755)
					os.WriteFile(targetPath, []byte(filtered), 0644)
					headerDirs[targetDir] = true
				}
			}
		}
		return nil
	}

	if err := walkDir("."); err != nil {
		return err
	}

	var copyIncludeDir func(string, string) error
	copyIncludeDir = func(srcDir, dstDir string) error {
		for _, std := range commonCLibraries {
			targetPath := filepath.Join(dstDir, std)
			dirPath := filepath.Dir(targetPath)

			if _, err := os.Stat(dirPath); err != nil {
				if os.IsNotExist(err) {
					if err := os.MkdirAll(dirPath, 0755); err != nil {
						return err
					}
				} else {
					return err
				}
			}

			if _, err := os.Stat(targetPath); err != nil {
				if os.IsNotExist(err) {
					if err := os.WriteFile(targetPath, []byte{}, 0644); err != nil {
						return err
					}
				} else {
					return err
				}
			}
		}

		entries, err := underlying.ReadDir(srcDir)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			srcPath := underlying.Join(srcDir, entry.Name())
			dstPath := filepath.Join(dstDir, entry.Name())

			if entry.IsDir() {
				os.MkdirAll(dstPath, 0755)
				headerDirs[dstPath] = true
				copyIncludeDir(srcPath, dstPath)
			} else {
				if content, err := underlying.ReadFile(srcPath); err == nil {
					if underlying.Ext(entry.Name()) == ".h" || underlying.Ext(entry.Name()) == ".in" {
						content = []byte(filterSystemIncludes(string(content)))
					}
					os.WriteFile(dstPath, content, 0644)
					headerDirs[filepath.Dir(dstPath)] = true
				}
			}
		}
		return nil
	}

	includeDir := filepath.Join(globalTempDir, "include")
	os.MkdirAll(includeDir, 0755)
	headerDirs[includeDir] = true
	copyIncludeDir("include", includeDir)

	// Build includeDirs list once, combining all header directories
	globalIncludeDirs = make([]string, 0, len(headerDirs)+1)
	globalIncludeDirs = append(globalIncludeDirs, globalTempDir)
	for dir := range headerDirs {
		if dir != globalTempDir {
			globalIncludeDirs = append(globalIncludeDirs, dir)
		}
	}

	return nil
}

// PreprocessCSource performs C macro preprocessing on source code
func PreprocessCSource(src string) (string, error) {
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

// NewPreprocessedCFs creates a new C preprocessor filesystem wrapper
func NewPreprocessedCFs(underlying fi.FileSystem) (*PreprocessedCFS, error) {
	fs := &PreprocessedCFS{
		underlying: underlying,
		enabled:    true,
	}

	// Copy all .h files
	if err := setupHeaderFiles(underlying); err != nil {
		return nil, fmt.Errorf("failed to setup header files: %w", err)
	}

	return fs, nil
}

func (f *PreprocessedCFS) ReadFile(name string) ([]byte, error) {
	data, err := f.underlying.ReadFile(name)
	if err != nil {
		return nil, err
	}

	if f.enabled && (strings.HasSuffix(strings.ToLower(name), ".c") || strings.HasSuffix(strings.ToLower(name), ".h")) {
		preprocessed, err := PreprocessCSource(string(data))
		if err != nil {
			log.Warnf("C macro preprocessing failed for %s: %v, using original source", name, err)
			return data, nil
		}
		return []byte(preprocessed), nil
	}

	return data, nil
}

func (f *PreprocessedCFS) SetEnabled(enabled bool) {
	f.enabled = enabled
}

func (f *PreprocessedCFS) Open(name string) (fs.File, error) {
	return f.underlying.Open(name)
}

func (f *PreprocessedCFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return f.underlying.OpenFile(name, flag, perm)
}

func (f *PreprocessedCFS) Stat(name string) (fs.FileInfo, error) {
	return f.underlying.Stat(name)
}

func (f *PreprocessedCFS) ReadDir(dirname string) ([]fs.DirEntry, error) {
	return f.underlying.ReadDir(dirname)
}

func (f *PreprocessedCFS) GetSeparators() rune {
	return f.underlying.GetSeparators()
}

func (f *PreprocessedCFS) Join(paths ...string) string {
	return f.underlying.Join(paths...)
}

func (f *PreprocessedCFS) IsAbs(name string) bool {
	return f.underlying.IsAbs(name)
}

func (f *PreprocessedCFS) Getwd() (string, error) {
	return f.underlying.Getwd()
}

func (f *PreprocessedCFS) Exists(path string) (bool, error) {
	return f.underlying.Exists(path)
}

func (f *PreprocessedCFS) Rename(old string, new string) error {
	return f.underlying.Rename(old, new)
}

func (f *PreprocessedCFS) Rel(base string, target string) (string, error) {
	return f.underlying.Rel(base, target)
}

func (f *PreprocessedCFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return f.underlying.WriteFile(name, data, perm)
}

func (f *PreprocessedCFS) Delete(name string) error {
	return f.underlying.Delete(name)
}

func (f *PreprocessedCFS) MkdirAll(name string, perm os.FileMode) error {
	return f.underlying.MkdirAll(name, perm)
}

func (f *PreprocessedCFS) String() string {
	underlyingStr := "FileSystem"
	if stringer, ok := f.underlying.(fmt.Stringer); ok {
		underlyingStr = stringer.String()
	}
	return fmt.Sprintf("PreprocessedCFS{underlying: %s}", underlyingStr)
}

func (f *PreprocessedCFS) Root() string {
	if rooter, ok := f.underlying.(interface{ Root() string }); ok {
		return rooter.Root()
	}
	return ""
}

func (f *PreprocessedCFS) ExtraInfo(path string) map[string]any {
	return f.underlying.ExtraInfo(path)
}

func (f *PreprocessedCFS) Base(p string) string {
	return f.underlying.Base(p)
}

func (f *PreprocessedCFS) PathSplit(s string) (string, string) {
	return f.underlying.PathSplit(s)
}

func (f *PreprocessedCFS) Ext(s string) string {
	return f.underlying.Ext(s)
}
