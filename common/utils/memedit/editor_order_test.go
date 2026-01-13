package memedit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSetOrderIndependence 测试 SetProgramName 和 SetFolderPath 的调用顺序无关性
// 注意：GetFolderPath 返回纯净的路径（不包含 programName，无前导/尾部斜杠）
// GetGlobalFolderPath 返回包含 programName 且带 trailing slash 的路径
func TestSetOrderIndependence(t *testing.T) {
	programName := "application"
	folderPathWithPrefix := "/application/path/to/folder/"
	expectedPurePath := "path/to/folder"
	expectedGlobalPath := "application/path/to/folder/"
	sourceCode := "test code"

	t.Run("先 SetProgramName 后 SetFolderPath", func(t *testing.T) {
		editor := NewMemEditor(sourceCode)
		editor.SetProgramName(programName)
		editor.SetFolderPath(folderPathWithPrefix)

		assert.Equal(t, expectedPurePath, editor.GetFolderPath())
		assert.Equal(t, expectedGlobalPath, editor.GetGlobalFolderPath())
		assert.Equal(t, programName, editor.GetProgramName())
	})

	t.Run("先 SetFolderPath 后 SetProgramName", func(t *testing.T) {
		editor := NewMemEditor(sourceCode)
		editor.SetFolderPath(folderPathWithPrefix)
		// 此时 programName 为空，folderPath 应该是 "application/path/to/folder" (去除了首尾斜杠)
		assert.Equal(t, "application/path/to/folder", editor.GetFolderPath())

		editor.SetProgramName(programName)
		// 设置 programName 后应该自动重新规范化，GetFolderPath 返回纯净路径
		assert.Equal(t, expectedPurePath, editor.GetFolderPath())
		assert.Equal(t, expectedGlobalPath, editor.GetGlobalFolderPath())
		assert.Equal(t, programName, editor.GetProgramName())
	})

	t.Run("交替设置", func(t *testing.T) {
		editor := NewMemEditor(sourceCode)
		editor.SetFolderPath("/application/part1")
		editor.SetProgramName(programName)
		assert.Equal(t, "part1", editor.GetFolderPath())
		assert.Equal(t, "application/part1/", editor.GetGlobalFolderPath())

		editor.SetFolderPath("/application/part2/")
		assert.Equal(t, "part2", editor.GetFolderPath())
		assert.Equal(t, "application/part2/", editor.GetGlobalFolderPath())
	})
}
