package ssadb

import (
	"strings"
	"testing"
	"unicode/utf8"

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
		// Invalid UTF-8 byte sequences — PostgreSQL rejects these at the
		// wire-protocol level just like NUL bytes. Observed in real scans:
		//   0xa0               (lone continuation byte, no leading byte)
		//   0xe8 0x07 0x10     (0xe8 starts a 3-byte seq, but 0x07/0x10 are
		//                       not valid continuation bytes 0x80-0xBF)
		// The sanitizer must produce a string PG will accept: valid UTF-8
		// and no NUL. strings.ToValidUTF8 strips the *invalid* bytes (the
		// 0xa0, or the 0xe8 leader) but keeps bytes that are valid UTF-8 on
		// their own (0x07/0x10 are valid ASCII control chars once 0xe8 is
		// gone). The invariant under test is "PG would accept the result",
		// not "all suspicious bytes vanished".
		{"lone 0xa0 stripped", "hello\xa0world", "helloworld"},
		{"leading 0xa0 stripped", "\xa0start", "start"},
		{"trailing 0xa0 stripped", "end\xa0", "end"},
		// 0xe8 is the invalid byte (3-byte leader without valid continuations);
		// 0x07 0x10 are valid ASCII once 0xe8 is removed.
		{"invalid 3-byte leader 0xe8 stripped, valid ascii kept", "ab\xe8\x07\x10cd", "ab\x07\x10cd"},
		{"only invalid 0xa0", "\xa0\xa0\xa0", ""},
		{"mixed NUL and 0xa0", "a\x00b\xa0c", "abc"},
		// Valid multi-byte UTF-8 MUST be preserved (regression guard — the
		// sanitizer must not nuke legitimate CJK / emoji content).
		{"valid CJK preserved", "你好世界", "你好世界"},
		{"valid emoji preserved", "hello \xf0\x9f\x98\x80 world", "hello \xf0\x9f\x98\x80 world"},
		{"valid 2-byte preserved", "caf\xc3\xa9", "caf\xc3\xa9"}, // café
		// Invalid byte in the middle of an otherwise-valid run
		{"invalid between valid CJK", "你\xa0好", "你好"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeStringForPG(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.False(t, strings.ContainsRune(result, 0), "result must not contain NUL byte")
			assert.True(t, utf8.ValidString(result), "result must be valid UTF-8")
		})
	}

	// Fast path: clean strings should be returned as-is (same pointer)
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

// TestSanitizeStructStringsForPG_InvalidUTF8_IrCode verifies that illegal
// UTF-8 byte sequences (the 0xa0 / 0xe8 0x07 0x10 patterns observed in real
// scans) are stripped from every string field on IrCode, while valid multi-byte
// UTF-8 (CJK, emoji) is preserved.
func TestSanitizeStructStringsForPG_InvalidUTF8_IrCode(t *testing.T) {
	ir := &IrCode{
		ProgramName:       "test\xa0program",
		Name:              "func\xe8\x07\x10name",
		VerboseName:       "verbose\xa0name",
		ShortVerboseName:  "short\xe8\x07\x10name",
		String:            "const\xa0value",
		OpcodeName:        "opcode",
		SourceCodeHash:    "hash\xa0abc",
		ProgramCompileHash: "compile\xe8\x07\x10hash",
		ConstType:         "normal",
		Variable:          StringSlice{"var\xa01", "var2", "var\xe8\x07\x103"},
	}

	sanitizeStructStringsForPG(ir)

	// 0xa0 is a lone continuation byte → stripped entirely.
	// 0xe8 0x07 0x10 → 0xe8 is the invalid 3-byte leader, stripped; 0x07 0x10
	// are valid ASCII control chars once 0xe8 is gone, so they survive.
	assert.True(t, utf8.ValidString(ir.ProgramName), "ProgramName must be valid UTF-8")
	assert.Equal(t, "testprogram", ir.ProgramName)
	assert.True(t, utf8.ValidString(ir.Name), "Name must be valid UTF-8")
	assert.Equal(t, "func\x07\x10name", ir.Name)
	assert.Equal(t, "verbosename", ir.VerboseName)
	assert.Equal(t, "short\x07\x10name", ir.ShortVerboseName)
	assert.Equal(t, "constvalue", ir.String)
	assert.Equal(t, "hashabc", ir.SourceCodeHash)
	assert.Equal(t, "compile\x07\x10hash", ir.ProgramCompileHash)
	// Non-invalid fields unchanged
	assert.Equal(t, "opcode", ir.OpcodeName)
	assert.Equal(t, "normal", ir.ConstType)
	// StringSlice elements
	assert.Equal(t, StringSlice{"var1", "var2", "var\x07\x103"}, ir.Variable)
}

// TestSanitizeStringForPG_PreservesValidUTF8 is a focused regression guard:
// the sanitizer must only strip *invalid* bytes, never legitimate multi-byte
// UTF-8 content that source code legitimately contains.
func TestSanitizeStringForPG_PreservesValidUTF8(t *testing.T) {
	cases := []string{
		"你好世界",                                     // 4 CJK chars
		"hello \xf0\x9f\x98\x80 world",                // emoji
		"caf\xc3\xa9",                                  // café (2-byte)
		"日本語テスト",                                  // Japanese
		"\xe4\xb8\xad\xe6\x96\x87",                     // 中文
	}
	for _, s := range cases {
		result := sanitizeStringForPG(s)
		assert.Equal(t, s, result, "valid UTF-8 must be preserved: %q", s)
		assert.True(t, utf8.ValidString(result))
	}
}

// TestBeforeSaveHookStripsInvalidUTF8_IrCode verifies the BeforeSave hook
// fires on the SQLite path and strips invalid UTF-8 bytes (SQLite tolerates
// them, so without the hook they would be stored as-is).
func TestBeforeSaveHookStripsInvalidUTF8_IrCode(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	require.NoError(t, db.AutoMigrate(&IrCode{}).Error)

	ir := &IrCode{
		ProgramName: "test\xa0program",
		Name:        "func\xe8\x07\x10name",
		String:      "hello\xa0world\xe8\x07\x10!",
	}
	require.NoError(t, db.Save(ir).Error)

	var result IrCode
	require.NoError(t, db.First(&result, ir.ID).Error)

	assert.True(t, utf8.ValidString(result.ProgramName), "ProgramName must be valid UTF-8")
	assert.True(t, utf8.ValidString(result.Name), "Name must be valid UTF-8")
	assert.True(t, utf8.ValidString(result.String), "String must be valid UTF-8")
	assert.Equal(t, "testprogram", result.ProgramName)
	assert.Equal(t, "func\x07\x10name", result.Name)
	assert.Equal(t, "helloworld\x07\x10!", result.String)
}
