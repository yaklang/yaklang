package loop_yaklangcode

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBatchRegexReplaceOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    *BatchRegexReplaceOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil options",
			opts:    nil,
			wantErr: true,
			errMsg:  "options cannot be nil",
		},
		{
			name: "empty pattern",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "",
				Replacement: "test",
			},
			wantErr: true,
			errMsg:  "pattern cannot be empty",
		},
		{
			name: "invalid regex pattern",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "[invalid",
				Replacement: "test",
			},
			wantErr: true,
			errMsg:  "invalid regexp pattern",
		},
		{
			name: "negative group",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "test",
				Replacement: "replacement",
				Group:       -1,
			},
			wantErr: true,
			errMsg:  "group must be >= 0",
		},
		{
			name: "valid options",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "test",
				Replacement: "replacement",
				Group:       0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBatchRegexReplaceOptions(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBatchRegexReplace_BasicReplacements(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		opts     *BatchRegexReplaceOptions
		expected *BatchRegexReplaceResult
	}{
		{
			name: "simple string replacement",
			code: "hello world\nhello universe\ngoodbye world",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "hello",
				Replacement: "hi",
				Group:       0,
			},
			expected: &BatchRegexReplaceResult{
				ModifiedCode:     "hi world\nhi universe\ngoodbye world",
				ReplacementCount: 2,
				HasModifications: true,
			},
		},
		{
			name: "no matches",
			code: "hello world\ngoodbye world",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "xyz",
				Replacement: "abc",
				Group:       0,
			},
			expected: &BatchRegexReplaceResult{
				ModifiedCode:     "hello world\ngoodbye world",
				ReplacementCount: 0,
				HasModifications: false,
			},
		},
		{
			name: "empty code",
			code: "",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "test",
				Replacement: "replacement",
				Group:       0,
			},
			expected: &BatchRegexReplaceResult{
				ModifiedCode:     "",
				ReplacementCount: 0,
				HasModifications: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BatchRegexReplace(tt.code, tt.opts)
			require.NoError(t, err)

			assert.Equal(t, tt.expected.ModifiedCode, result.ModifiedCode)
			assert.Equal(t, tt.expected.ReplacementCount, result.ReplacementCount)
			assert.Equal(t, tt.expected.HasModifications, result.HasModifications)
			assert.Len(t, result.ModifiedLines, tt.expected.ReplacementCount)
		})
	}
}

func TestBatchRegexReplace_CaptureGroups(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		opts     *BatchRegexReplaceOptions
		expected string
	}{
		{
			name: "function rename with capture group",
			code: "func oldFunction() {\nfunc anotherFunction() {\nvar x = 5",
			opts: &BatchRegexReplaceOptions{
				Pattern:     `func\s+(\w+)\(`,
				Replacement: "new_$1",
				Group:       1,
			},
			expected: "func new_oldFunction() {\nfunc new_anotherFunction() {\nvar x = 5",
		},
		{
			name: "variable assignment with multiple groups",
			code: "var port = 3000\nvar host = \"localhost\"\nvar timeout = 5000",
			opts: &BatchRegexReplaceOptions{
				Pattern:     `var\s+(\w+)\s*=\s*(\d+)`,
				Replacement: "$1 := $2",
				Group:       0, // 替换整个匹配
			},
			expected: "port := 3000\nvar host = \"localhost\"\ntimeout := 5000",
		},
		{
			name: "capture group that doesn't exist",
			code: "func test() {\nfunc another() {",
			opts: &BatchRegexReplaceOptions{
				Pattern:     `func\s+(\w+)\(`,
				Replacement: "prefix_$1",
				Group:       2, // 不存在的捕获组
			},
			expected: "func test() {\nfunc another() {", // 应该保持不变
		},
		{
			name: "string literal replacement",
			code: `url := "http://example.com"\napi := "http://api.test.com"\nother := "https://secure.com"`,
			opts: &BatchRegexReplaceOptions{
				Pattern:     `"(http://[^"]+)"`,
				Replacement: "\"https://$1\"",
				Group:       0, // 替换整个匹配
			},
			expected: `url := "https://http://example.com"\napi := "https://http://api.test.com"\nother := "https://secure.com"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BatchRegexReplace(tt.code, tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.ModifiedCode)
		})
	}
}

func TestBatchRegexReplace_YaklangSpecificCases(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		opts     *BatchRegexReplaceOptions
		expected string
	}{
		{
			name: "yaklang function calls",
			code: "str.Split(data, \",\")\nstr.Join(items, \"|\")\nhttp.Get(url)",
			opts: &BatchRegexReplaceOptions{
				Pattern:     `(\w+)\.(\w+)\(`,
				Replacement: "$1.$2_v2(",
				Group:       0, // 替换整个匹配
			},
			expected: "str.Split_v2(data, \",\")\nstr.Join_v2(items, \"|\")\nhttp.Get_v2(url)",
		},
		{
			name: "yaklang error handling",
			code: "result, err := someFunc()\ndata, error := anotherFunc()\nif err != nil {",
			opts: &BatchRegexReplaceOptions{
				Pattern:     `(\w+),\s*err\s*:=`,
				Replacement: "$1, e :=",
				Group:       0,
			},
			expected: "result, e := someFunc()\ndata, error := anotherFunc()\nif err != nil {",
		},
		{
			name: "yaklang import statements",
			code: "import \"github.com/yaklang/yaklang/common/utils\"\nimport \"fmt\"\nimport \"github.com/yaklang/yaklang/common/log\"",
			opts: &BatchRegexReplaceOptions{
				Pattern:     `import\s+"(github\.com/yaklang/yaklang/[^"]+)"`,
				Replacement: "// $1",
				Group:       0,
			},
			expected: "// github.com/yaklang/yaklang/common/utils\nimport \"fmt\"\n// github.com/yaklang/yaklang/common/log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BatchRegexReplace(tt.code, tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.ModifiedCode)
		})
	}
}

func TestBatchRegexReplace_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		opts     *BatchRegexReplaceOptions
		expected string
	}{
		{
			name: "single line with multiple matches",
			code: "test test test",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "test",
				Replacement: "replaced",
				Group:       0,
			},
			expected: "replaced replaced replaced",
		},
		{
			name: "multiline string (should not match across lines)",
			code: "start line\nend line",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "start.*end",
				Replacement: "matched",
				Group:       0,
			},
			expected: "start line\nend line", // 不应该匹配跨行
		},
		{
			name: "special regex characters",
			code: "price: $100\ncost: $200\ntax: $50",
			opts: &BatchRegexReplaceOptions{
				Pattern:     `\$(\d+)`,
				Replacement: "USD_$1",
				Group:       0,
			},
			expected: "price: USD_100\ncost: USD_200\ntax: USD_50",
		},
		{
			name: "empty lines handling",
			code: "line1\n\nline3\n\nline5",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "line",
				Replacement: "row",
				Group:       0,
			},
			expected: "row1\n\nrow3\n\nrow5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BatchRegexReplace(tt.code, tt.opts)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.ModifiedCode)
		})
	}
}

func TestBatchRegexReplace_ModifiedLineInfo(t *testing.T) {
	code := "func oldFunc() {\n    return nil\n}\nfunc anotherFunc() {\n    return true\n}"
	opts := &BatchRegexReplaceOptions{
		Pattern:     `func\s+(\w+)\(`,
		Replacement: "new_$1",
		Group:       1,
	}

	result, err := BatchRegexReplace(code, opts)
	require.NoError(t, err)

	assert.Equal(t, 2, result.ReplacementCount)
	assert.Len(t, result.ModifiedLines, 2)

	// 检查第一个修改行
	assert.Equal(t, 1, result.ModifiedLines[0].LineNumber)
	assert.Equal(t, "func oldFunc() {", result.ModifiedLines[0].OriginalLine)
	assert.Equal(t, "func new_oldFunc() {", result.ModifiedLines[0].ModifiedLine)

	// 检查第二个修改行
	assert.Equal(t, 4, result.ModifiedLines[1].LineNumber)
	assert.Equal(t, "func anotherFunc() {", result.ModifiedLines[1].OriginalLine)
	assert.Equal(t, "func new_anotherFunc() {", result.ModifiedLines[1].ModifiedLine)
}

func TestBatchRegexReplaceMultiPattern(t *testing.T) {
	code := "func oldFunc() {\n    var port = 3000\n    return nil\n}"

	patterns := []BatchRegexReplaceOptions{
		{
			Pattern:     `func\s+(\w+)\(`,
			Replacement: "new_$1",
			Group:       1,
		},
		{
			Pattern:     `var\s+(\w+)\s*=\s*(\d+)`,
			Replacement: "$1 := $2",
			Group:       0,
		},
	}

	result, err := BatchRegexReplaceMultiPattern(code, patterns)
	require.NoError(t, err)

	expected := "func new_oldFunc() {\n    port := 3000\n    return nil\n}"
	assert.Equal(t, expected, result.ModifiedCode)
	assert.Equal(t, 2, result.ReplacementCount)
	assert.True(t, result.HasModifications)
}

func TestExpandReplacementReferences(t *testing.T) {
	tests := []struct {
		name        string
		replacement string
		matches     []string
		expected    string
	}{
		{
			name:        "simple reference",
			replacement: "new_$1",
			matches:     []string{"func oldName(", "oldName"},
			expected:    "new_oldName",
		},
		{
			name:        "multiple references",
			replacement: "$2_$1_suffix",
			matches:     []string{"var name = value", "name", "value"},
			expected:    "value_name_suffix",
		},
		{
			name:        "no references",
			replacement: "fixed_value",
			matches:     []string{"anything", "group1"},
			expected:    "fixed_value",
		},
		{
			name:        "$0 reference (whole match)",
			replacement: "prefix_$0_suffix",
			matches:     []string{"whole_match", "group1"},
			expected:    "prefix_whole_match_suffix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandReplacementReferences(tt.replacement, tt.matches)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBatchRegexReplace_ErrorCases(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		opts    *BatchRegexReplaceOptions
		wantErr bool
	}{
		{
			name: "invalid options",
			code: "test code",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "",
				Replacement: "test",
			},
			wantErr: true,
		},
		{
			name: "invalid regex at runtime", // 这种情况在验证阶段就会被捕获
			code: "test code",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "[invalid",
				Replacement: "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BatchRegexReplace(tt.code, tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBatchRegexReplace_DeleteLines(t *testing.T) {
	tests := []struct {
		name                 string
		code                 string
		opts                 *BatchRegexReplaceOptions
		expected             string
		expectedDeletedCount int
	}{
		{
			name: "delete single comment line",
			code: "line1\n// 保存ZIP文件\nline3",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "^// 保存ZIP文件$",
				Replacement: "",
				Group:       0,
			},
			expected:             "line1\nline3",
			expectedDeletedCount: 1,
		},
		{
			name: "delete multiple comment lines",
			code: "line1\n// 保存ZIP文件\nline3\n// 保存ZIP文件\nline5",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "^// 保存ZIP文件$",
				Replacement: "",
				Group:       0,
			},
			expected:             "line1\nline3\nline5",
			expectedDeletedCount: 2,
		},
		{
			name: "delete empty lines",
			code: "line1\n\nline3\n\nline5",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "^$",
				Replacement: "",
				Group:       0,
			},
			expected:             "line1\nline3\nline5",
			expectedDeletedCount: 2,
		},
		{
			name: "partial line replacement (not deletion)",
			code: "// TODO: 保存ZIP文件\n// FIXME: 保存ZIP文件",
			opts: &BatchRegexReplaceOptions{
				Pattern:     "保存ZIP文件",
				Replacement: "",
				Group:       0,
			},
			expected:             "// TODO: \n// FIXME: ",
			expectedDeletedCount: 0, // 不是删除整行，而是部分替换
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BatchRegexReplace(tt.code, tt.opts)
			require.NoError(t, err)

			assert.Equal(t, tt.expected, result.ModifiedCode)

			// 检查删除的行数
			deletedCount := 0
			for _, modLine := range result.ModifiedLines {
				if modLine.ModifiedLine == "[DELETED]" {
					deletedCount++
				}
			}
			assert.Equal(t, tt.expectedDeletedCount, deletedCount)

			// 验证行数变化
			originalLines := len(strings.Split(tt.code, "\n"))
			resultLines := len(strings.Split(result.ModifiedCode, "\n"))
			expectedLines := originalLines - tt.expectedDeletedCount
			assert.Equal(t, expectedLines, resultLines)
		})
	}
}

// 性能测试
func BenchmarkBatchRegexReplace(b *testing.B) {
	// 创建一个较大的代码示例
	var codeBuilder strings.Builder
	for i := 0; i < 1000; i++ {
		codeBuilder.WriteString("func testFunc")
		codeBuilder.WriteString(string(rune('A' + i%26)))
		codeBuilder.WriteString("() {\n    return nil\n}\n")
	}
	code := codeBuilder.String()

	opts := &BatchRegexReplaceOptions{
		Pattern:     `func\s+(\w+)\(`,
		Replacement: "new_$1",
		Group:       1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := BatchRegexReplace(code, opts)
		if err != nil {
			b.Fatal(err)
		}
	}
}
