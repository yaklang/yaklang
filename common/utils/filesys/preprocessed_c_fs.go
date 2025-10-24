package filesys

import (
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
	tempDir     string
	includeDirs []string
	enabled     bool
}

var _ fi.FileSystem = (*PreprocessedCFS)(nil)

// NewPreprocessedCFs creates a new C preprocessor filesystem wrapper
func NewPreprocessedCFs(underlying fi.FileSystem) (*PreprocessedCFS, error) {
	fs := &PreprocessedCFS{
		underlying: underlying,
		enabled:    true,
	}

	tmpDir, err := os.MkdirTemp("", "c_headers_*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	fs.tempDir = tmpDir

	// Copy all .h files
	if err := fs.setupHeaderFiles(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to setup header files: %w", err)
	}

	return fs, nil
}

func (f *PreprocessedCFS) setupHeaderFiles() error {
	headerDirs := make(map[string]bool)

	var walkDir func(string) error
	walkDir = func(dir string) error {
		entries, err := f.underlying.ReadDir(dir)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			filePath := f.underlying.Join(dir, entry.Name())

			if entry.IsDir() {
				walkDir(filePath)
			} else if f.underlying.Ext(entry.Name()) == ".h" {
				if content, err := f.underlying.ReadFile(filePath); err == nil {
					relPath := strings.TrimPrefix(filePath, ".")
					relPath = strings.TrimPrefix(relPath, string(f.underlying.GetSeparators()))
					targetPath := filepath.Join(f.tempDir, relPath)
					targetDir := filepath.Dir(targetPath)

					os.MkdirAll(targetDir, 0755)
					os.WriteFile(targetPath, content, 0644)
					headerDirs[targetDir] = true
				}
			}
		}
		return nil
	}

	if err := walkDir("."); err != nil {
		return err
	}

	f.includeDirs = make([]string, 0, len(headerDirs)+1)
	f.includeDirs = append(f.includeDirs, f.tempDir)
	for dir := range headerDirs {
		if dir != f.tempDir {
			f.includeDirs = append(f.includeDirs, dir)
		}
	}

	var copyIncludeDir func(string, string) error
	copyIncludeDir = func(srcDir, dstDir string) error {
		entries, err := f.underlying.ReadDir(srcDir)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			srcPath := f.underlying.Join(srcDir, entry.Name())
			dstPath := filepath.Join(dstDir, entry.Name())

			if entry.IsDir() {
				os.MkdirAll(dstPath, 0755)
				headerDirs[dstPath] = true
				copyIncludeDir(srcPath, dstPath)
			} else {
				if content, err := f.underlying.ReadFile(srcPath); err == nil {
					os.WriteFile(dstPath, content, 0644)
					headerDirs[filepath.Dir(dstPath)] = true
				}
			}
		}
		return nil
	}

	if exists, _ := f.underlying.Exists("include"); exists {
		includeDir := filepath.Join(f.tempDir, "include")
		os.MkdirAll(includeDir, 0755)
		headerDirs[includeDir] = true
		copyIncludeDir("include", includeDir)

		f.includeDirs = make([]string, 0, len(headerDirs)+1)
		f.includeDirs = append(f.includeDirs, f.tempDir)
		for dir := range headerDirs {
			if dir != f.tempDir {
				f.includeDirs = append(f.includeDirs, dir)
			}
		}
	}

	return nil
}

// preprocessCSource performs C macro preprocessing on source code
func (f *PreprocessedCFS) preprocessCSource(src string) (string, error) {
	var preprocessorCmd string

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

	tmpFile, err := os.CreateTemp(f.tempDir, "c_preprocess_*.c")
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
	for _, includeDir := range f.includeDirs {
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

func (f *PreprocessedCFS) ReadFile(name string) ([]byte, error) {
	data, err := f.underlying.ReadFile(name)
	if err != nil {
		return nil, err
	}

	if f.enabled && strings.HasSuffix(strings.ToLower(name), ".c") {
		preprocessed, err := f.preprocessCSource(string(data))
		if err != nil {
			log.Warnf("C macro preprocessing failed for %s: %v, using original source", name, err)
			return data, nil
		}
		return []byte(preprocessed), nil
	}

	return data, nil
}

func (f *PreprocessedCFS) Cleanup() {
	if f.tempDir != "" {
		os.RemoveAll(f.tempDir)
		f.tempDir = ""
	}
}

func (f *PreprocessedCFS) SetEnabled(enabled bool) {
	f.enabled = enabled
}

func (f *PreprocessedCFS) GetTempDir() string {
	return f.tempDir
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
	return fmt.Sprintf("PreprocessedCFS{underlying: %s, tempDir: %s}", underlyingStr, f.tempDir)
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
