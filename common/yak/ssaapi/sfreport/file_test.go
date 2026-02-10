package sfreport

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

func TestFile2EditorRoundTrip(t *testing.T) {
	tests := []struct {
		name            string
		sourceCode      string
		folderPath      string
		fileName        string
		fullCode        bool
		expectHashMatch bool
	}{
		{
			name:            "nested path",
			sourceCode:      "public class Test { }",
			folderPath:      "/src/main/java",
			fileName:        "Test.java",
			fullCode:        true,
			expectHashMatch: true,
		},
		{
			name:            "root level file",
			sourceCode:      "",
			folderPath:      "/",
			fileName:        "empty.txt",
			fullCode:        true,
			expectHashMatch: true,
		},
		{
			name:            "special characters",
			sourceCode:      "print('你好世界')",
			folderPath:      "/app",
			fileName:        "main.py",
			fullCode:        true,
			expectHashMatch: true,
		},
		{
			name:            "truncated code",
			sourceCode:      "public class VeryLongClass {\n    public static void main(String[] args) {\n        System.out.println(\"Line 1\");\n    }\n}",
			folderPath:      "/src",
			fileName:        "VeryLongClass.java",
			fullCode:        false,
			expectHashMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			programName := uuid.NewString()

			var fileUrl string
			if tt.folderPath == "/" {
				fileUrl = "/" + programName + "/" + tt.fileName
			} else {
				fileUrl = "/" + programName + tt.folderPath + "/" + tt.fileName
			}

			originalEditor := memedit.NewMemEditor(tt.sourceCode)
			originalEditor.SetProgramName(programName)
			originalEditor.SetFolderPath(tt.folderPath)
			originalEditor.SetFileName(tt.fileName)
			originalEditor.SetUrl(fileUrl)

			originalHash := originalEditor.GetIrSourceHash()

			file := editor2File(originalEditor, tt.fullCode)

			convertedEditor, err := file2Editor(file, programName)
			require.NoError(t, err)

			assert.Equal(t, programName, convertedEditor.GetProgramName())
			assert.Equal(t, tt.folderPath, convertedEditor.GetFolderPath())
			assert.Equal(t, tt.fileName, convertedEditor.GetFilename())

			if tt.expectHashMatch {
				assert.Equal(t, originalHash, convertedEditor.GetIrSourceHash())
				assert.Equal(t, fileUrl, convertedEditor.GetUrl())
				assert.Equal(t, tt.sourceCode, convertedEditor.GetSourceCode())
			} else {
				assert.NotEqual(t, originalHash, convertedEditor.GetIrSourceHash())
			}
		})
	}
}
