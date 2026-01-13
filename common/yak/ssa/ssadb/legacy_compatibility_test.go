package ssadb

import (
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// TestLegacyDataFormatCompatibility ensures that MarshalFile produces FolderPath compatible with legacy data
// (containing programName and trailing slash)
func TestLegacyDataFormatCompatibility(t *testing.T) {
	sourceCode := "test code"
	programName := "application"
	folderPath := "/path/to/folder/"
	// Legacy format: /programName/path/to/folder/
	expectedFolderPath := "/application/path/to/folder/"

	editor := memedit.NewMemEditor(sourceCode)
	editor.SetProgramName(programName)
	editor.SetFolderPath(folderPath)
	editor.SetFileName("test.jsp")

	irSource := MarshalFile(editor)

	// Simulate BeforeSave behavior which adds leading slash if missing
	// We call BeforeSave directly to verify the final stored path
	_ = irSource.BeforeSave(&gorm.DB{})

	assert.Equal(t, expectedFolderPath, irSource.FolderPath, "FolderPath should be compatible with legacy format")
}
