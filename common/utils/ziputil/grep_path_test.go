package ziputil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrepPathRegexp(t *testing.T) {
	files := map[string]string{
		"src/main.go":         "package main",
		"src/utils/helper.go": "package utils",
		"test/main_test.go":   "package main",
		"docs/README.md":      "# README",
		"config.json":         "{}",
	}
	zipData := createTestZip(t, files)

	t.Run("search go files", func(t *testing.T) {
		results, err := GrepPathRawRegexp(zipData, `\.go$`)
		require.NoError(t, err)
		assert.Len(t, results, 3)

		for _, r := range results {
			assert.Contains(t, r.FileName, ".go")
		}
	})

	t.Run("search test files", func(t *testing.T) {
		results, err := GrepPathRawRegexp(zipData, `_test\.go$`)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "test/main_test.go", results[0].FileName)
	})

	t.Run("search src directory", func(t *testing.T) {
		results, err := GrepPathRawRegexp(zipData, `^src/`)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("with limit", func(t *testing.T) {
		results, err := GrepPathRawRegexp(zipData, `\.go$`, WithGrepLimit(2))
		require.NoError(t, err)
		assert.LessOrEqual(t, len(results), 2)
	})
}

func TestGrepPathSubString(t *testing.T) {
	files := map[string]string{
		"src/main.go":         "package main",
		"src/TEST_file.go":    "package test",
		"test/helper_test.go": "package test",
		"docs/README.md":      "# README",
	}
	zipData := createTestZip(t, files)

	t.Run("case insensitive", func(t *testing.T) {
		results, err := GrepPathRawSubString(zipData, "test")
		require.NoError(t, err)
		assert.Len(t, results, 2) // TEST_file.go and helper_test.go
	})

	t.Run("case sensitive", func(t *testing.T) {
		results, err := GrepPathRawSubString(zipData, "TEST", WithGrepCaseSensitive())
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Contains(t, results[0].FileName, "TEST_file.go")
	})

	t.Run("search docs", func(t *testing.T) {
		results, err := GrepPathRawSubString(zipData, "docs")
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Contains(t, results[0].FileName, "docs")
	})
}

func TestPathFiltering(t *testing.T) {
	files := map[string]string{
		"src/main.go":         "content",
		"src/utils/helper.go": "content",
		"test/main_test.go":   "content",
		"vendor/lib.go":       "content",
		"docs/README.md":      "content",
	}
	zipData := createTestZip(t, files)

	t.Run("include path substring", func(t *testing.T) {
		results, err := GrepRawSubString(zipData, "content",
			WithIncludePathSubString("src/"))
		require.NoError(t, err)
		assert.Len(t, results, 2) // main.go and helper.go
		for _, r := range results {
			assert.Contains(t, r.FileName, "src/")
		}
	})

	t.Run("exclude path substring", func(t *testing.T) {
		results, err := GrepRawSubString(zipData, "content",
			WithExcludePathSubString("test", "vendor"))
		require.NoError(t, err)
		// Should exclude test/ and vendor/
		for _, r := range results {
			assert.NotContains(t, r.FileName, "test")
			assert.NotContains(t, r.FileName, "vendor")
		}
	})

	t.Run("include path regexp", func(t *testing.T) {
		results, err := GrepRawSubString(zipData, "content",
			WithIncludePathRegexp(`\.go$`))
		require.NoError(t, err)
		assert.Len(t, results, 4) // All .go files
		for _, r := range results {
			assert.Contains(t, r.FileName, ".go")
		}
	})

	t.Run("exclude path regexp", func(t *testing.T) {
		results, err := GrepRawSubString(zipData, "content",
			WithExcludePathRegexp(`_test\.go$`))
		require.NoError(t, err)
		// Should exclude test files
		for _, r := range results {
			assert.NotContains(t, r.FileName, "_test.go")
		}
	})

	t.Run("combined filters", func(t *testing.T) {
		results, err := GrepRawSubString(zipData, "content",
			WithIncludePathRegexp(`\.go$`),
			WithExcludePathSubString("test", "vendor"))
		require.NoError(t, err)
		// Should only include .go files from src/
		assert.Len(t, results, 2)
		for _, r := range results {
			assert.Contains(t, r.FileName, "src/")
			assert.Contains(t, r.FileName, ".go")
		}
	})
}

func TestPathFilteringWithGrepRegexp(t *testing.T) {
	files := map[string]string{
		"src/main.go":         "ERROR: test",
		"test/helper_test.go": "ERROR: test",
		"vendor/lib.go":       "ERROR: test",
		"docs/README.md":      "ERROR: test",
	}
	zipData := createTestZip(t, files)

	t.Run("grep with path filter", func(t *testing.T) {
		results, err := GrepRawRegexp(zipData, "ERROR",
			WithExcludePathSubString("vendor", "test"))
		require.NoError(t, err)
		assert.Len(t, results, 2) // src/main.go and docs/README.md
	})
}

func TestGrepPathWithFilters(t *testing.T) {
	files := map[string]string{
		"src/main.go":         "content",
		"src/utils/helper.go": "content",
		"test/main_test.go":   "content",
		"vendor/lib.go":       "content",
	}
	zipData := createTestZip(t, files)

	t.Run("grep path with include filter", func(t *testing.T) {
		results, err := GrepPathRawRegexp(zipData, `\.go$`,
			WithIncludePathSubString("src/"))
		require.NoError(t, err)
		assert.Len(t, results, 2)
		for _, r := range results {
			assert.Contains(t, r.FileName, "src/")
		}
	})

	t.Run("grep path with exclude filter", func(t *testing.T) {
		results, err := GrepPathRawRegexp(zipData, `\.go$`,
			WithExcludePathSubString("test", "vendor"))
		require.NoError(t, err)
		assert.Len(t, results, 2) // Only src/main.go and src/utils/helper.go
	})
}

func TestShouldIncludePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		config   *GrepConfig
		expected bool
	}{
		{
			name:     "no filters",
			path:     "src/main.go",
			config:   &GrepConfig{},
			expected: true,
		},
		{
			name: "exclude substring match",
			path: "vendor/lib.go",
			config: &GrepConfig{
				ExcludePathSubString: []string{"vendor"},
			},
			expected: false,
		},
		{
			name: "exclude substring no match",
			path: "src/main.go",
			config: &GrepConfig{
				ExcludePathSubString: []string{"vendor"},
			},
			expected: true,
		},
		{
			name: "include substring match",
			path: "src/main.go",
			config: &GrepConfig{
				IncludePathSubString: []string{"src/"},
			},
			expected: true,
		},
		{
			name: "include substring no match",
			path: "test/main.go",
			config: &GrepConfig{
				IncludePathSubString: []string{"src/"},
			},
			expected: false,
		},
		{
			name: "exclude regexp match",
			path: "main_test.go",
			config: &GrepConfig{
				ExcludePathRegexp: []string{`_test\.go$`},
			},
			expected: false,
		},
		{
			name: "include regexp match",
			path: "main.go",
			config: &GrepConfig{
				IncludePathRegexp: []string{`\.go$`},
			},
			expected: true,
		},
		{
			name: "combined filters - pass",
			path: "src/main.go",
			config: &GrepConfig{
				IncludePathSubString: []string{"src/"},
				ExcludePathSubString: []string{"test"},
			},
			expected: true,
		},
		{
			name: "combined filters - fail exclude",
			path: "src/test.go",
			config: &GrepConfig{
				IncludePathSubString: []string{"src/"},
				ExcludePathSubString: []string{"test"},
			},
			expected: false,
		},
		{
			name: "combined filters - fail include",
			path: "docs/README.md",
			config: &GrepConfig{
				IncludePathSubString: []string{"src/"},
				ExcludePathSubString: []string{"test"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldIncludePath(tt.path, tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGrepPathResult(t *testing.T) {
	files := map[string]string{
		"src/main.go":    "content",
		"test/helper.go": "content",
	}
	zipData := createTestZip(t, files)

	results, err := GrepPathRawSubString(zipData, ".go")
	require.NoError(t, err)
	require.NotEmpty(t, results)

	for _, r := range results {
		// 路径搜索的结果应该有特殊属性
		assert.Equal(t, 0, r.LineNumber)           // 路径搜索没有行号
		assert.Equal(t, r.FileName, r.Line)        // Line 应该等于 FileName
		assert.Contains(t, r.ScoreMethod, "path_") // ScoreMethod 应该包含 path_ 前缀
	}
}
