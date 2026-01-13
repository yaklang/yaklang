package memedit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEditorFolderPathNormalization 测试 folderPath 规范化
// 注意：GetFolderPath 返回纯净的路径（不包含 programName，无前导/尾部斜杠）
// GetGlobalFolderPath 返回包含 programName 且带 trailing slash 的路径，用于数据库查询
func TestEditorFolderPathNormalization(t *testing.T) {
	tests := []struct {
		name               string
		programName        string
		inputPath          string
		expectedPath       string // GetFolderPath expected (pure path)
		expectedGlobalPath string // GetGlobalFolderPath expected (with programName and trailing /)
	}{
		{
			name:               "路径带前导斜杠",
			programName:        "application",
			inputPath:          "/path/to/folder",
			expectedPath:       "path/to/folder",
			expectedGlobalPath: "application/path/to/folder/",
		},
		{
			name:               "路径带尾部斜杠",
			programName:        "application",
			inputPath:          "path/to/folder/",
			expectedPath:       "path/to/folder",
			expectedGlobalPath: "application/path/to/folder/",
		},
		{
			name:               "路径带前导和尾部斜杠",
			programName:        "application",
			inputPath:          "/path/to/folder/",
			expectedPath:       "path/to/folder",
			expectedGlobalPath: "application/path/to/folder/",
		},
		{
			name:               "路径包含 programName 前缀",
			programName:        "application",
			inputPath:          "/application/path/to/folder/",
			expectedPath:       "path/to/folder",
			expectedGlobalPath: "application/path/to/folder/",
		},
		{
			name:               "路径包含 programName 但无前导斜杠",
			programName:        "application",
			inputPath:          "application/path/to/folder",
			expectedPath:       "path/to/folder",
			expectedGlobalPath: "application/path/to/folder/",
		},
		{
			name:               "空路径",
			programName:        "application",
			inputPath:          "",
			expectedPath:       "",
			expectedGlobalPath: "application/",
		},
		{
			name:               "只有斜杠",
			programName:        "application",
			inputPath:          "/",
			expectedPath:       "",
			expectedGlobalPath: "application/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor("test code")
			editor.SetProgramName(tt.programName)
			editor.SetFolderPath(tt.inputPath)
			editor.SetFileName("test.jsp")

			assert.Equal(t, tt.expectedPath, editor.GetFolderPath(), "FolderPath should be normalized (pure path)")
			assert.Equal(t, tt.expectedGlobalPath, editor.GetGlobalFolderPath(), "GlobalFolderPath should include programName and trailing slash")
		})
	}
}

// TestEditorHashConsistency 测试 hash 计算一致性
func TestEditorHashConsistency(t *testing.T) {
	sourceCode := "test code content"
	programName := "application"
	fileName := "test.jsp"

	tests := []struct {
		name        string
		folderPath1 string
		folderPath2 string
		shouldMatch bool
	}{
		{
			name:        "相同的规范化路径应该产生相同的 hash",
			folderPath1: "path/to/folder",
			folderPath2: "path/to/folder",
			shouldMatch: true,
		},
		{
			name:        "带前导斜杠的路径应该与不带的产生相同 hash",
			folderPath1: "/path/to/folder",
			folderPath2: "path/to/folder",
			shouldMatch: true,
		},
		{
			name:        "带尾部斜杠的路径应该与不带的产生相同 hash",
			folderPath1: "path/to/folder/",
			folderPath2: "path/to/folder",
			shouldMatch: true,
		},
		{
			name:        "包含 programName 的路径应该与不包含的产生相同 hash",
			folderPath1: "/application/path/to/folder/",
			folderPath2: "path/to/folder",
			shouldMatch: true,
		},
		{
			name:        "不同的路径应该产生不同的 hash",
			folderPath1: "path/to/folder1",
			folderPath2: "path/to/folder2",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor1 := NewMemEditor(sourceCode)
			editor1.SetProgramName(programName)
			editor1.SetFolderPath(tt.folderPath1)
			editor1.SetFileName(fileName)
			hash1 := editor1.GetIrSourceHash()

			editor2 := NewMemEditor(sourceCode)
			editor2.SetProgramName(programName)
			editor2.SetFolderPath(tt.folderPath2)
			editor2.SetFileName(fileName)
			hash2 := editor2.GetIrSourceHash()

			if tt.shouldMatch {
				assert.Equal(t, hash1, hash2, "Hashes should match")
			} else {
				assert.NotEqual(t, hash1, hash2, "Hashes should not match")
			}
		})
	}
}

// TestEditorHashWithEmptyPath 测试空路径的 hash 计算
func TestEditorHashWithEmptyPath(t *testing.T) {
	sourceCode := "test code"
	programName := "application"
	fileName := "test.jsp"

	tests := []struct {
		name       string
		folderPath string
	}{
		{name: "空字符串", folderPath: ""},
		{name: "单斜杠", folderPath: "/"},
		{name: "只有 programName", folderPath: "/application/"},
		{name: "只有 programName 无斜杠", folderPath: "application"},
	}

	var baseHash string
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor(sourceCode)
			editor.SetProgramName(programName)
			editor.SetFolderPath(tt.folderPath)
			editor.SetFileName(fileName)
			hash := editor.GetIrSourceHash()

			if i == 0 {
				baseHash = hash
			} else {
				assert.Equal(t, baseHash, hash, "All empty paths should produce the same hash")
			}
		})
	}
}

// TestGetGlobalFolderPath 测试 GetGlobalFolderPath 方法
func TestGetGlobalFolderPath(t *testing.T) {
	tests := []struct {
		name         string
		programName  string
		folderPath   string
		expectedPath string
	}{
		{
			name:         "有 programName 和 folderPath",
			programName:  "myapp",
			folderPath:   "src/main",
			expectedPath: "myapp/src/main/",
		},
		{
			name:         "只有 programName",
			programName:  "myapp",
			folderPath:   "",
			expectedPath: "myapp/",
		},
		{
			name:         "只有 folderPath",
			programName:  "",
			folderPath:   "src/main",
			expectedPath: "src/main/",
		},
		{
			name:         "都为空",
			programName:  "",
			folderPath:   "",
			expectedPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor("test code")
			editor.SetProgramName(tt.programName)
			editor.SetFolderPath(tt.folderPath)
			editor.SetFileName("test.go")

			assert.Equal(t, tt.expectedPath, editor.GetGlobalFolderPath())
		})
	}
}

// TestJoinGlobalPath 测试 JoinGlobalPath 方法
func TestJoinGlobalPath(t *testing.T) {
	tests := []struct {
		name         string
		programName  string
		folderPath   string
		subPath      string
		expectedPath string
	}{
		{
			name:         "正常连接",
			programName:  "myapp",
			folderPath:   "src/main",
			subPath:      "utils/helper.go",
			expectedPath: "myapp/src/main/utils/helper.go",
		},
		{
			name:         "subPath 为空",
			programName:  "myapp",
			folderPath:   "src/main",
			subPath:      "",
			expectedPath: "myapp/src/main",
		},
		{
			name:         "globalDir 为空",
			programName:  "",
			folderPath:   "",
			subPath:      "test.go",
			expectedPath: "test.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := NewMemEditor("test code")
			editor.SetProgramName(tt.programName)
			editor.SetFolderPath(tt.folderPath)
			editor.SetFileName("test.go")

			assert.Equal(t, tt.expectedPath, editor.JoinGlobalPath(tt.subPath))
		})
	}
}
