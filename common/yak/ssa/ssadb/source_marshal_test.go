package ssadb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// TestMarshalAndUnmarshalConsistency 测试 MarshalFile 和 irSource2Editor 的一致性
func TestMarshalAndUnmarshalConsistency(t *testing.T) {
	tests := []struct {
		name        string
		programName string
		folderPath  string
		fileName    string
		sourceCode  string
	}{
		{
			name:        "正常路径",
			programName: "application",
			folderPath:  "path/to/folder",
			fileName:    "test.jsp",
			sourceCode:  "test code content",
		},
		{
			name:        "带前导斜杠的路径",
			programName: "application",
			folderPath:  "/path/to/folder",
			fileName:    "test.jsp",
			sourceCode:  "test code content",
		},
		{
			name:        "带尾部斜杠的路径",
			programName: "application",
			folderPath:  "path/to/folder/",
			fileName:    "test.jsp",
			sourceCode:  "test code content",
		},
		{
			name:        "包含 programName 的路径",
			programName: "application",
			folderPath:  "/application/path/to/folder/",
			fileName:    "test.jsp",
			sourceCode:  "test code content",
		},
		{
			name:        "空路径",
			programName: "application",
			folderPath:  "",
			fileName:    "test.jsp",
			sourceCode:  "test code content",
		},
		{
			name:        "根路径",
			programName: "application",
			folderPath:  "/",
			fileName:    "test.jsp",
			sourceCode:  "test code content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建原始 editor
			originalEditor := memedit.NewMemEditor(tt.sourceCode)
			originalEditor.SetProgramName(tt.programName)
			originalEditor.SetFolderPath(tt.folderPath)
			originalEditor.SetFileName(tt.fileName)
			originalHash := originalEditor.GetIrSourceHash()

			// 序列化
			irSource := MarshalFile(originalEditor)
			assert.Equal(t, tt.programName, irSource.ProgramName)
			assert.Equal(t, tt.fileName, irSource.FileName)
			assert.Equal(t, originalHash, irSource.SourceCodeHash)

			// 反序列化
			restoredEditor := irSource2Editor(irSource)
			assert.Equal(t, tt.programName, restoredEditor.GetProgramName())
			assert.Equal(t, tt.fileName, restoredEditor.GetFilename())
			assert.Equal(t, tt.sourceCode, restoredEditor.GetSourceCode())

			// 关键：验证 hash 一致性
			restoredHash := restoredEditor.GetIrSourceHash()
			assert.Equal(t, originalHash, restoredHash, "Hash should be consistent after marshal/unmarshal")

			// 验证规范化的 folderPath
			assert.Equal(t, originalEditor.GetFolderPath(), restoredEditor.GetFolderPath(), "FolderPath should be normalized consistently")
		})
	}
}

// TestHashStability 测试多次序列化反序列化的 hash 稳定性
func TestHashStability(t *testing.T) {
	sourceCode := "test code"
	programName := "application"
	folderPath := "/application/path/to/folder/"
	fileName := "test.jsp"

	// 第一次创建
	editor1 := memedit.NewMemEditor(sourceCode)
	editor1.SetProgramName(programName)
	editor1.SetFolderPath(folderPath)
	editor1.SetFileName(fileName)
	hash1 := editor1.GetIrSourceHash()

	// 第一次序列化
	irSource1 := MarshalFile(editor1)
	assert.Equal(t, hash1, irSource1.SourceCodeHash)

	// 第一次反序列化
	editor2 := irSource2Editor(irSource1)
	hash2 := editor2.GetIrSourceHash()
	assert.Equal(t, hash1, hash2, "Hash should remain stable after first round-trip")

	// 第二次序列化
	irSource2 := MarshalFile(editor2)
	assert.Equal(t, hash1, irSource2.SourceCodeHash, "Hash should remain stable after second marshal")

	// 第二次反序列化
	editor3 := irSource2Editor(irSource2)
	hash3 := editor3.GetIrSourceHash()
	assert.Equal(t, hash1, hash3, "Hash should remain stable after second round-trip")

	// 验证所有 editor 的规范化路径一致
	assert.Equal(t, editor1.GetFolderPath(), editor2.GetFolderPath())
	assert.Equal(t, editor2.GetFolderPath(), editor3.GetFolderPath())
}
