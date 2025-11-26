package ssaapi

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestDefaultProcess(t *testing.T) {
	config, err := DefaultConfig(
		WithFileSystem(filesys.NewLocalFs()),
		WithProcess(func(msg string, process float64) {
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, config)
	require.NotNil(t, config.process)
}

// TestUnifiedFsWithFileSystem 测试使用 WithFileSystem 选项时，fs 被正确转换为 UnifiedFileSys
// 这个测试覆盖所有文件系统类型，确保在不同平台（Windows/Linux/Mac）下都能正确工作
func TestUnifiedFsWithFileSystem(t *testing.T) {
	// 创建一个临时目录用于测试
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// 测试使用本地文件系统（LocalFs）
	// LocalFs 在 Windows 下使用 '\\'，在 Unix 下使用 '/'
	// 通过 UnifiedFS 包装后应该统一使用 '/'
	t.Run("LocalFs with WithFileSystem", func(t *testing.T) {
		localFs := filesys.NewLocalFs()

		// 记录原始分隔符（用于验证转换）
		originalSeparator := localFs.GetSeparators()
		t.Logf("Original LocalFs separator: %c (OS: %s)", originalSeparator, runtime.GOOS)

		config, err := DefaultConfig(
			WithFileSystem(localFs),
			WithEditor(nil), // 提供 editor 以避免错误
		)
		require.NoError(t, err)
		require.NotNil(t, config)
		require.NotNil(t, config.fs)

		// 验证 fs 是 UnifiedFS 类型
		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok, "fs should be converted to UnifiedFS")
		require.NotNil(t, unifiedFs)

		// 验证分隔符是 IrSource 的分隔符，即 '/'
		// 这是关键测试点：无论原始 fs 的分隔符是什么，都应该被转换为 '/'
		separator := unifiedFs.GetSeparators()
		require.Equal(t, ssadb.IrSourceFsSeparators, separator, "separator should be '/'")
		require.Equal(t, '/', separator, "separator should be '/'")

		// 在 Windows 下验证转换确实发生了
		if runtime.GOOS == "windows" {
			require.Equal(t, '\\', originalSeparator, "Windows LocalFs should use '\\\\'")
			require.NotEqual(t, originalSeparator, separator, "separator should be converted from '\\\\' to '/'")
		}
	})

	// 测试使用相对路径本地文件系统（RelLocalFs）
	// RelLocalFs 也是平台相关的，需要统一转换
	t.Run("RelLocalFs with WithFileSystem", func(t *testing.T) {
		relLocalFs := filesys.NewRelLocalFs(tempDir)

		originalSeparator := relLocalFs.GetSeparators()
		t.Logf("Original RelLocalFs separator: %c (OS: %s)", originalSeparator, runtime.GOOS)

		config, err := DefaultConfig(
			WithFileSystem(relLocalFs),
			WithEditor(nil),
		)
		require.NoError(t, err)
		require.NotNil(t, config)
		require.NotNil(t, config.fs)

		// 验证 fs 是 UnifiedFS 类型
		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok, "fs should be converted to UnifiedFS")
		require.NotNil(t, unifiedFs)

		// 验证分隔符是 '/'
		separator := unifiedFs.GetSeparators()
		require.Equal(t, ssadb.IrSourceFsSeparators, separator)
		require.Equal(t, '/', separator)

		// 在 Windows 下验证转换
		if runtime.GOOS == "windows" {
			require.Equal(t, '\\', originalSeparator, "Windows RelLocalFs should use '\\\\'")
		}
	})

	// 测试 Mock Windows FS - 在任何平台都能测试 Windows 路径转换
	// 这是关键测试：模拟 Windows 环境以验证分隔符转换
	t.Run("MockWindowsFS with WithFileSystem", func(t *testing.T) {
		// 创建一个虚拟文件系统作为底层
		baseFS := filesys.NewVirtualFs()
		baseFS.AddFile("project/src/main.go", "package main")
		baseFS.AddFile("project/src/utils/helper.go", "package utils")

		// 包装为 mock Windows FS
		mockWinFS := newMockWindowsFS(baseFS)

		// 验证 mock 确实使用 Windows 分隔符
		originalSeparator := mockWinFS.GetSeparators()
		require.Equal(t, '\\', originalSeparator, "MockWindowsFS should use '\\\\'")
		t.Logf("MockWindowsFS separator: %c (simulating Windows on %s)", originalSeparator, runtime.GOOS)

		// 测试 Windows 风格的路径操作
		winPath := mockWinFS.Join("project", "src", "main.go")
		require.Equal(t, "project\\src\\main.go", winPath, "Mock Windows Join should use '\\\\'")

		// 配置使用 mock Windows FS
		config, err := DefaultConfig(
			WithFileSystem(mockWinFS),
			WithEditor(nil),
		)
		require.NoError(t, err)
		require.NotNil(t, config.fs)

		// 验证转换为 UnifiedFS
		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok, "MockWindowsFS should be converted to UnifiedFS")

		// 关键验证：分隔符从 '\\' 转换为 '/'
		separator := unifiedFs.GetSeparators()
		require.Equal(t, '/', separator, "separator should be converted from '\\\\' to '/'")
		require.NotEqual(t, originalSeparator, separator, "separator must change from '\\\\' to '/'")

		// 验证转换后的路径操作使用 '/'
		unifiedPath := unifiedFs.Join("project", "src", "main.go")
		require.Equal(t, "project/src/main.go", unifiedPath, "UnifiedFS Join should use '/'")
		require.NotEqual(t, winPath, unifiedPath, "paths should be different after conversion")

		t.Logf("Successfully converted Windows path '%s' to unified path '%s'", winPath, unifiedPath)
	})

	// 测试 VirtualFS（已经使用 '/' 分隔符）
	t.Run("VirtualFS with WithFileSystem", func(t *testing.T) {
		virtualFs := filesys.NewVirtualFs()
		virtualFs.AddFile("dir/file.txt", "content")

		originalSeparator := virtualFs.GetSeparators()
		require.Equal(t, '/', originalSeparator, "VirtualFS should use '/'")

		config, err := DefaultConfig(
			WithFileSystem(virtualFs),
			WithEditor(nil),
		)
		require.NoError(t, err)
		require.NotNil(t, config.fs)

		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok)

		separator := unifiedFs.GetSeparators()
		require.Equal(t, '/', separator)
	})

	// 测试 ZipFS（已经使用 '/' 分隔符）
	t.Run("ZipFS with WithFileSystem", func(t *testing.T) {
		// 创建一个简单的 zip 文件
		buf := new(bytes.Buffer)
		zipWriter := zip.NewWriter(buf)
		fileWriter, err := zipWriter.Create("test/file.txt")
		require.NoError(t, err)
		_, err = fileWriter.Write([]byte("test content"))
		require.NoError(t, err)
		err = zipWriter.Close()
		require.NoError(t, err)

		zipFs, err := filesys.NewZipFSFromString(buf.String())
		require.NoError(t, err)

		originalSeparator := zipFs.GetSeparators()
		require.Equal(t, '/', originalSeparator, "ZipFS should use '/'")

		config, err := DefaultConfig(
			WithFileSystem(zipFs),
			WithEditor(nil),
		)
		require.NoError(t, err)
		require.NotNil(t, config.fs)

		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok)

		separator := unifiedFs.GetSeparators()
		require.Equal(t, '/', separator)
	})

	// 测试 Join 方法使用正确的分隔符
	t.Run("Join uses correct separator across platforms", func(t *testing.T) {
		localFs := filesys.NewLocalFs()
		config, err := DefaultConfig(
			WithFileSystem(localFs),
			WithEditor(nil),
		)
		require.NoError(t, err)

		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok)

		// 测试 Join 使用 '/' 作为分隔符
		// 无论在什么平台上，都应该使用 '/'
		joined := unifiedFs.Join("a", "b", "c")
		require.Equal(t, "a/b/c", joined, "Join should use '/' as separator on all platforms")

		// 测试更复杂的路径
		complexJoined := unifiedFs.Join("project", "src", "main", "java", "App.java")
		require.Equal(t, "project/src/main/java/App.java", complexJoined)
	})

	// 测试 PathSplit 方法使用正确的分隔符
	t.Run("PathSplit uses correct separator", func(t *testing.T) {
		localFs := filesys.NewLocalFs()
		config, err := DefaultConfig(
			WithFileSystem(localFs),
			WithEditor(nil),
		)
		require.NoError(t, err)

		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok)

		// 测试 PathSplit 使用 '/' 作为分隔符
		dir, file := unifiedFs.PathSplit("a/b/c.txt")
		require.Equal(t, "a/b", dir)
		require.Equal(t, "c.txt", file)
	})
}

// TestWindowsPathConversion 专门测试 Windows 路径到统一路径的转换
// 使用 mock Windows FS 在任何平台上都能测试
func TestWindowsPathConversion(t *testing.T) {
	// 创建虚拟文件系统
	baseFS := filesys.NewVirtualFs()
	baseFS.AddFile("C:/project/src/main.go", "package main\nfunc main() {}")
	baseFS.AddFile("C:/project/src/lib/utils.go", "package lib")
	baseFS.AddFile("C:/project/test/main_test.go", "package main")
	baseFS.AddDir("C:/project/build")

	// 创建 mock Windows FS
	mockWinFS := newMockWindowsFS(baseFS)

	t.Run("Windows separator verification", func(t *testing.T) {
		require.Equal(t, '\\', mockWinFS.GetSeparators())
	})

	t.Run("Windows path operations before conversion", func(t *testing.T) {
		// 测试 Windows 风格的路径操作
		joined := mockWinFS.Join("C:", "project", "src", "main.go")
		require.Equal(t, "C:\\project\\src\\main.go", joined)

		dir, file := mockWinFS.PathSplit("C:\\project\\src\\main.go")
		require.Equal(t, "C:\\project\\src", dir)
		require.Equal(t, "main.go", file)

		base := mockWinFS.Base("C:\\project\\src\\main.go")
		require.Equal(t, "main.go", base)
	})

	t.Run("Convert to UnifiedFS and verify Unix-style paths", func(t *testing.T) {
		config, err := DefaultConfig(
			WithFileSystem(mockWinFS),
			WithEditor(nil),
		)
		require.NoError(t, err)

		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok)

		// 验证转换后使用 Unix 风格分隔符
		require.Equal(t, '/', unifiedFs.GetSeparators())

		// 测试统一后的路径操作
		joined := unifiedFs.Join("C:", "project", "src", "main.go")
		require.Equal(t, "C:/project/src/main.go", joined, "should use '/' after conversion")

		dir, file := unifiedFs.PathSplit("C:/project/src/main.go")
		require.Equal(t, "C:/project/src", dir)
		require.Equal(t, "main.go", file)

		base := unifiedFs.Base("C:/project/src/main.go")
		require.Equal(t, "main.go", base)
	})

	t.Run("Complex Windows paths conversion", func(t *testing.T) {
		config, err := DefaultConfig(
			WithFileSystem(mockWinFS),
			WithEditor(nil),
		)
		require.NoError(t, err)

		unifiedFs := config.fs.(*filesys.UnifiedFS)

		testCases := []struct {
			name     string
			input    []string
			expected string
		}{
			{
				name:     "simple path",
				input:    []string{"a", "b", "c"},
				expected: "a/b/c",
			},
			{
				name:     "deep nested path",
				input:    []string{"project", "src", "main", "java", "com", "example", "App.java"},
				expected: "project/src/main/java/com/example/App.java",
			},
			{
				name:     "path with dots",
				input:    []string{"project", "..", "other", "file.txt"},
				expected: "other/file.txt",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := unifiedFs.Join(tc.input...)
				require.Equal(t, tc.expected, result)
				require.NotContains(t, result, "\\", "result should not contain Windows separator")
			})
		}
	})

	t.Run("IsAbs with unified paths", func(t *testing.T) {
		config, err := DefaultConfig(
			WithFileSystem(mockWinFS),
			WithEditor(nil),
		)
		require.NoError(t, err)

		unifiedFs := config.fs.(*filesys.UnifiedFS)

		// 在统一文件系统中，绝对路径以 '/' 开头
		require.True(t, unifiedFs.IsAbs("/absolute/path"))
		require.False(t, unifiedFs.IsAbs("relative/path"))

		// Windows 风格的绝对路径不应该被识别为绝对路径
		require.False(t, unifiedFs.IsAbs("C:\\Windows\\path"))
		require.False(t, unifiedFs.IsAbs("\\\\server\\share"))
	})
}

// TestUnifiedFsWithConfigInfo 测试使用 WithConfigInfo 选项时，fs 被正确转换为 UnifiedFileSys
// 覆盖所有通过 CodeSourceInfo 配置的文件系统类型
func TestUnifiedFsWithConfigInfo(t *testing.T) {
	// 创建一个临时目录和测试文件
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// 测试使用 local 类型的 CodeSourceInfo
	// 这会创建 RelLocalFs，它在不同平台有不同的分隔符
	t.Run("Local CodeSource with WithConfigInfo", func(t *testing.T) {
		config, err := DefaultConfig(
			WithConfigInfo(map[string]any{
				"kind":       "local",
				"local_file": tempDir,
			}),
			WithEditor(nil),
		)
		require.NoError(t, err)
		require.NotNil(t, config)
		require.NotNil(t, config.fs)

		// 验证 fs 是 UnifiedFS 类型
		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok, "fs should be converted to UnifiedFS")
		require.NotNil(t, unifiedFs)

		// 验证分隔符是 IrSource 的分隔符，即 '/'
		separator := unifiedFs.GetSeparators()
		require.Equal(t, ssadb.IrSourceFsSeparators, separator, "separator should be '/'")
		require.Equal(t, '/', separator, "separator should be '/'")

		t.Logf("Local CodeSource converted to UnifiedFS with separator: %c (OS: %s)", separator, runtime.GOOS)
	})

	// 测试 WithLocalFs 辅助函数
	t.Run("WithLocalFs helper", func(t *testing.T) {
		config, err := DefaultConfig(
			WithLocalFs(tempDir),
			WithEditor(nil),
		)
		require.NoError(t, err)
		require.NotNil(t, config)
		require.NotNil(t, config.fs)

		// 验证 fs 是 UnifiedFS 类型
		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok, "fs should be converted to UnifiedFS")

		// 验证分隔符是 '/'
		separator := unifiedFs.GetSeparators()
		require.Equal(t, ssadb.IrSourceFsSeparators, separator)
		require.Equal(t, '/', separator)
	})

	// 测试 compression 类型（ZipFS）
	t.Run("Compression CodeSource with WithConfigInfo", func(t *testing.T) {
		// 创建一个临时 zip 文件
		zipPath := filepath.Join(tempDir, "test.zip")
		zipFile, err := os.Create(zipPath)
		require.NoError(t, err)

		zipWriter := zip.NewWriter(zipFile)
		fileWriter, err := zipWriter.Create("test/file.txt")
		require.NoError(t, err)
		_, err = fileWriter.Write([]byte("test content"))
		require.NoError(t, err)
		err = zipWriter.Close()
		require.NoError(t, err)
		err = zipFile.Close()
		require.NoError(t, err)

		config, err := DefaultConfig(
			WithConfigInfo(map[string]any{
				"kind":       "compression",
				"local_file": zipPath,
			}),
			WithEditor(nil),
		)
		require.NoError(t, err)
		require.NotNil(t, config.fs)

		// 验证 fs 是 UnifiedFS 类型
		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok, "compression source should be wrapped in UnifiedFS")

		// 验证分隔符是 '/'
		separator := unifiedFs.GetSeparators()
		require.Equal(t, '/', separator, "ZipFS should use '/' separator")
	})

	// 测试 PathSplit 方法使用正确的分隔符
	t.Run("PathSplit uses correct separator", func(t *testing.T) {
		config, err := DefaultConfig(
			WithLocalFs(tempDir),
			WithEditor(nil),
		)
		require.NoError(t, err)

		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok)

		// 测试 PathSplit 使用 '/' 作为分隔符
		dir, file := unifiedFs.PathSplit("a/b/c.txt")
		require.Equal(t, "a/b", dir, "dir should use '/' as separator")
		require.Equal(t, "c.txt", file)
	})

	// 测试 Base 方法
	t.Run("Base uses correct separator", func(t *testing.T) {
		config, err := DefaultConfig(
			WithLocalFs(tempDir),
			WithEditor(nil),
		)
		require.NoError(t, err)

		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok)

		// 测试 Base 使用 '/' 作为分隔符
		base := unifiedFs.Base("a/b/c.txt")
		require.Equal(t, "c.txt", base)
	})

	// 测试 IsAbs 方法
	t.Run("IsAbs uses correct separator", func(t *testing.T) {
		config, err := DefaultConfig(
			WithLocalFs(tempDir),
			WithEditor(nil),
		)
		require.NoError(t, err)

		unifiedFs, ok := config.fs.(*filesys.UnifiedFS)
		require.True(t, ok)

		// 在统一的文件系统中，绝对路径以 '/' 开头
		require.True(t, unifiedFs.IsAbs("/absolute/path"))
		require.False(t, unifiedFs.IsAbs("relative/path"))

		// 在 Windows 下，原生的 '\\' 路径不应该被视为绝对路径（因为已经统一为 '/'）
		if runtime.GOOS == "windows" {
			require.False(t, unifiedFs.IsAbs("\\windows\\path"), "Windows-style path should not be absolute in unified fs")
		}
	})
}

// TestBothMethodsProduceSameResult 测试两种方式产生相同的结果
func TestBothMethodsProduceSameResult(t *testing.T) {
	tempDir := t.TempDir()

	// 使用 WithFileSystem
	config1, err := DefaultConfig(
		WithFileSystem(filesys.NewRelLocalFs(tempDir)),
		WithEditor(nil),
	)
	require.NoError(t, err)

	// 使用 WithConfigInfo
	config2, err := DefaultConfig(
		WithConfigInfo(map[string]any{
			"kind":       "local",
			"local_file": tempDir,
		}),
		WithEditor(nil),
	)
	require.NoError(t, err)

	// 两个配置都应该有 UnifiedFS
	unifiedFs1, ok1 := config1.fs.(*filesys.UnifiedFS)
	unifiedFs2, ok2 := config2.fs.(*filesys.UnifiedFS)
	require.True(t, ok1, "config1 fs should be UnifiedFS")
	require.True(t, ok2, "config2 fs should be UnifiedFS")

	// 两个配置的分隔符应该相同，都是 '/'
	sep1 := unifiedFs1.GetSeparators()
	sep2 := unifiedFs2.GetSeparators()
	require.Equal(t, sep1, sep2, "both configs should have same separator")
	require.Equal(t, ssadb.IrSourceFsSeparators, sep1)
	require.Equal(t, ssadb.IrSourceFsSeparators, sep2)
	require.Equal(t, '/', sep1)
	require.Equal(t, '/', sep2)

	// 测试 Join 方法产生相同的结果
	joined1 := unifiedFs1.Join("dir1", "dir2", "file.txt")
	joined2 := unifiedFs2.Join("dir1", "dir2", "file.txt")
	require.Equal(t, joined1, joined2, "Join should produce same result")
	require.Equal(t, "dir1/dir2/file.txt", joined1)
}

// mockWindowsFS 模拟 Windows 风格的文件系统，使用 '\\' 作为分隔符
// 这样可以在 Linux/macOS CI 环境中测试 Windows 路径转换
type mockWindowsFS struct {
	baseFS fi.FileSystem // 底层使用 Unix 风格的文件系统
}

func newMockWindowsFS(baseFS fi.FileSystem) *mockWindowsFS {
	return &mockWindowsFS{baseFS: baseFS}
}

// GetSeparators 返回 Windows 风格的分隔符
func (m *mockWindowsFS) GetSeparators() rune {
	return '\\' // Windows 分隔符
}

// Join 使用 Windows 风格的路径拼接
func (m *mockWindowsFS) Join(paths ...string) string {
	// 模拟 Windows 的 filepath.Join 行为
	return strings.Join(paths, "\\")
}

// 转换 Windows 路径为 Unix 路径以访问底层文件系统
func (m *mockWindowsFS) toUnixPath(windowsPath string) string {
	return strings.ReplaceAll(windowsPath, "\\", "/")
}

// 转换 Unix 路径为 Windows 路径
func (m *mockWindowsFS) toWindowsPath(unixPath string) string {
	return strings.ReplaceAll(unixPath, "/", "\\")
}

func (m *mockWindowsFS) ReadFile(name string) ([]byte, error) {
	return m.baseFS.ReadFile(m.toUnixPath(name))
}

func (m *mockWindowsFS) Open(name string) (fs.File, error) {
	return m.baseFS.Open(m.toUnixPath(name))
}

func (m *mockWindowsFS) OpenFile(name string, flag int, perm os.FileMode) (fs.File, error) {
	return m.baseFS.OpenFile(m.toUnixPath(name), flag, perm)
}

func (m *mockWindowsFS) Stat(name string) (fs.FileInfo, error) {
	return m.baseFS.Stat(m.toUnixPath(name))
}

func (m *mockWindowsFS) ReadDir(dirname string) ([]fs.DirEntry, error) {
	return m.baseFS.ReadDir(m.toUnixPath(dirname))
}

func (m *mockWindowsFS) PathSplit(path string) (string, string) {
	// 模拟 Windows 的 filepath.Split
	i := strings.LastIndex(path, "\\")
	if i < 0 {
		return "", path
	}
	return path[:i], path[i+1:]
}

func (m *mockWindowsFS) Base(path string) string {
	// 模拟 Windows 的 filepath.Base
	if path == "" {
		return "."
	}
	// 去掉尾部的分隔符
	for len(path) > 0 && path[len(path)-1] == '\\' {
		path = path[:len(path)-1]
	}
	// 找到最后一个分隔符
	i := strings.LastIndex(path, "\\")
	if i >= 0 {
		path = path[i+1:]
	}
	if path == "" {
		return "\\"
	}
	return path
}

func (m *mockWindowsFS) Ext(path string) string {
	return m.baseFS.Ext(path)
}

func (m *mockWindowsFS) IsAbs(path string) bool {
	// Windows 绝对路径: C:\\ 或 \\\\server\\share
	if len(path) >= 3 && path[1] == ':' && path[2] == '\\' {
		return true
	}
	if len(path) >= 2 && path[0] == '\\' && path[1] == '\\' {
		return true
	}
	return false
}

func (m *mockWindowsFS) Getwd() (string, error) {
	wd, err := m.baseFS.Getwd()
	if err != nil {
		return "", err
	}
	return m.toWindowsPath(wd), nil
}

func (m *mockWindowsFS) Exists(path string) (bool, error) {
	return m.baseFS.Exists(m.toUnixPath(path))
}

func (m *mockWindowsFS) Rename(old string, new string) error {
	return m.baseFS.Rename(m.toUnixPath(old), m.toUnixPath(new))
}

func (m *mockWindowsFS) Rel(base string, target string) (string, error) {
	rel, err := m.baseFS.Rel(m.toUnixPath(base), m.toUnixPath(target))
	if err != nil {
		return "", err
	}
	return m.toWindowsPath(rel), nil
}

func (m *mockWindowsFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return m.baseFS.WriteFile(m.toUnixPath(name), data, perm)
}

func (m *mockWindowsFS) Delete(name string) error {
	return m.baseFS.Delete(m.toUnixPath(name))
}

func (m *mockWindowsFS) MkdirAll(name string, perm os.FileMode) error {
	return m.baseFS.MkdirAll(m.toUnixPath(name), perm)
}

func (m *mockWindowsFS) ExtraInfo(path string) map[string]any {
	return m.baseFS.ExtraInfo(m.toUnixPath(path))
}

var _ fi.FileSystem = (*mockWindowsFS)(nil)
