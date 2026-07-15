package ssadb

import (
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeStringForPG(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"clean string", "hello world", "hello world"},
		{"single NUL", "hello\x00world", "helloworld"},
		{"leading NUL", "\x00start", "start"},
		{"trailing NUL", "end\x00", "end"},
		{"multiple NUL", "a\x00b\x00c\x00", "abc"},
		{"only NUL", "\x00\x00\x00", ""},
		{"empty", "", ""},
		{"tab is preserved", "hello\tworld", "hello\tworld"},
		{"newline is preserved", "hello\nworld", "hello\nworld"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeStringForPG(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.False(t, strings.ContainsRune(result, 0), "result must not contain NUL byte")
		})
	}

	// Fast path: strings without NUL should be returned as-is (same pointer)
	clean := "no nulls here"
	result := sanitizeStringForPG(clean)
	assert.Equal(t, clean, result)
}

func TestSanitizeStructStringsForPG_IrCode(t *testing.T) {
	ir := &IrCode{
		ProgramName:       "test\x00program",
		Name:              "func\x00name",
		VerboseName:       "verbose\x00name",
		ShortVerboseName:  "short\x00name",
		String:            "const\x00value",
		OpcodeName:        "opcode",
		SourceCodeHash:    "hash\x00abc",
		ProgramCompileHash: "compile\x00hash",
		ConstType:         "normal",
		Variable:          StringSlice{"var\x001", "var2", "var\x003"},
	}

	sanitizeStructStringsForPG(ir)

	assert.NotContains(t, ir.ProgramName, "\x00")
	assert.Equal(t, "testprogram", ir.ProgramName)
	assert.Equal(t, "funcname", ir.Name)
	assert.Equal(t, "verbosename", ir.VerboseName)
	assert.Equal(t, "shortname", ir.ShortVerboseName)
	assert.Equal(t, "constvalue", ir.String)
	assert.Equal(t, "hashabc", ir.SourceCodeHash)
	assert.Equal(t, "compilehash", ir.ProgramCompileHash)
	// Non-NUL fields unchanged
	assert.Equal(t, "opcode", ir.OpcodeName)
	assert.Equal(t, "normal", ir.ConstType)
	// StringSlice elements
	assert.Equal(t, StringSlice{"var1", "var2", "var3"}, ir.Variable)
}

func TestSanitizeStructStringsForPG_IrType(t *testing.T) {
	ir := &IrType{
		ProgramName: "test\x00prog",
		String:      "type\x00name",
	}
	sanitizeStructStringsForPG(ir)
	assert.Equal(t, "testprog", ir.ProgramName)
	assert.Equal(t, "typename", ir.String)
}

func TestSanitizeStructStringsForPG_IrNamePool(t *testing.T) {
	pool := &IrNamePool{
		ProgramName: "prog\x00name",
		Name:        "var\x00iable",
	}
	sanitizeStructStringsForPG(pool)
	assert.Equal(t, "progname", pool.ProgramName)
	assert.Equal(t, "variable", pool.Name)
}

func TestSanitizeStructStringsForPG_IrOffset(t *testing.T) {
	offset := &IrOffset{
		ProgramName:  "test\x00prog",
		VariableName: "var\x00name",
		FileHash:     "hash\x00123",
	}
	sanitizeStructStringsForPG(offset)
	assert.Equal(t, "testprog", offset.ProgramName)
	assert.Equal(t, "varname", offset.VariableName)
	assert.Equal(t, "hash123", offset.FileHash)
}

func TestBeforeSaveHookStripsNUL_IrCode(t *testing.T) {
	// Use SQLite to test the BeforeSave hook fires and strips NUL.
	// SQLite tolerates NUL bytes, so without the hook the value would be
	// stored as-is. With the hook, NUL bytes should be stripped before save.
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)

	ir := &IrCode{
		ProgramName: "test\x00program",
		Name:        "func\x00name",
		String:      "hello\x00world",
	}
	require.NoError(t, db.Save(ir).Error)

	// Read back
	var result IrCode
	require.NoError(t, db.First(&result, ir.ID).Error)

	assert.NotContains(t, result.ProgramName, "\x00", "ProgramName should have NUL stripped")
	assert.NotContains(t, result.Name, "\x00", "Name should have NUL stripped")
	assert.NotContains(t, result.String, "\x00", "String should have NUL stripped")
	assert.Equal(t, "testprogram", result.ProgramName)
	assert.Equal(t, "funcname", result.Name)
	assert.Equal(t, "helloworld", result.String)
}

func TestBeforeSaveHookStripsNUL_IrNamePool(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.AutoMigrate(&IrNamePool{}).Error)

	pool := &IrNamePool{
		ProgramName: "test\x00prog",
		Name:        "var\x00name",
	}
	require.NoError(t, db.Save(pool).Error)

	var result IrNamePool
	require.NoError(t, db.First(&result, pool.NameID).Error)
	assert.Equal(t, "testprog", result.ProgramName)
	assert.Equal(t, "varname", result.Name)
}

func TestBeforeSaveHookStripsNUL_UpsertIrCode(t *testing.T) {
	// UpsertIrCode uses db.Where().Assign().FirstOrCreate() which also
	// triggers BeforeSave. Verify the hook covers this path.
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)

	ir := &IrCode{
		ProgramName: "test\x00upsert",
		CodeID:      1,
		Name:        "func\x00x",
	}
	require.NoError(t, UpsertIrCode(db, ir))

	var result IrCode
	require.NoError(t, db.Where("program_name = ?", "testupsert").First(&result).Error)
	assert.Equal(t, "testupsert", result.ProgramName)
	assert.Equal(t, "funcx", result.Name)
}
