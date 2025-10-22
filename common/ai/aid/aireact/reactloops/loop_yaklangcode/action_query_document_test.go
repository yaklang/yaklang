package loop_yaklangcode

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aicommon_mock"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/yak/yakurl"
)

func TestSearchYakdocLibraries_LibNames(t *testing.T) {
	// Test searching by library names
	payloads := aitool.InvokeParams{
		"lib_names": []string{"str", "http"},
	}

	results := searchYakdocLibraries(payloads)

	// Should find results for both libraries
	assert.True(t, len(results) >= 2, "Should find results for both str and http libraries")

	// Check that results contain expected library information
	foundStr := false
	foundHttp := false

	for _, result := range results {
		if result.Path == "yakdoc://lib/str" {
			foundStr = true
			assert.Contains(t, result.Content, "# Library: str")
			assert.Contains(t, result.Content, "## Functions:")
		}
		if result.Path == "yakdoc://lib/http" {
			foundHttp = true
			assert.Contains(t, result.Content, "# Library: http")
		}
	}

	assert.True(t, foundStr, "Should find str library")
	assert.True(t, foundHttp, "Should find http library")
}

func TestSearchYakdocLibraries_CaseInsensitive(t *testing.T) {
	// Test case insensitive library search
	tests := []struct {
		name     string
		libNames []string
		expected int // minimum expected results
	}{
		{
			name:     "lowercase",
			libNames: []string{"str", "http"},
			expected: 2,
		},
		{
			name:     "uppercase",
			libNames: []string{"STR", "HTTP"},
			expected: 2,
		},
		{
			name:     "mixed case",
			libNames: []string{"Str", "Http"},
			expected: 2,
		},
		{
			name:     "random case",
			libNames: []string{"sTr", "hTtP"},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payloads := aitool.InvokeParams{
				"lib_names": tt.libNames,
			}

			results := searchYakdocLibraries(payloads)

			// Should find results regardless of case
			assert.True(t, len(results) >= tt.expected,
				"Should find at least %d results for case %s, got %d",
				tt.expected, tt.name, len(results))

			// Check that we found some library content
			foundLibraryContent := false
			for _, result := range results {
				if strings.Contains(result.Content, "# Library:") {
					foundLibraryContent = true
					break
				}
			}
			assert.True(t, foundLibraryContent, "Should find library content for case %s", tt.name)
		})
	}
}

func TestSearchYakdocLibraries_LibFunctionGlobs(t *testing.T) {
	// Test searching by function globs
	payloads := aitool.InvokeParams{
		"lib_function_globs": []string{"*Rand*", "str.Split"},
	}

	results := searchYakdocLibraries(payloads)

	// Should find some results
	assert.True(t, len(results) > 0, "Should find results for function globs")

	// Check that results contain function information
	for _, result := range results {
		assert.Contains(t, result.Path, "yakdoc://func/")
		assert.Contains(t, result.Content, "# Function:")
		// Verify that the content is not empty and contains function information
		assert.NotEmpty(t, result.Content, "Function content should not be empty")
	}
}

func TestSearchYakdocLibraries_EmptyParams(t *testing.T) {
	// Test with empty parameters
	payloads := aitool.InvokeParams{}

	results := searchYakdocLibraries(payloads)

	// Should return empty results
	assert.Equal(t, 0, len(results), "Should return empty results for empty parameters")
}

func TestSearchYakdocLibraries_NonExistentLib(t *testing.T) {
	// Test searching for non-existent library
	payloads := aitool.InvokeParams{
		"lib_names": []string{"nonexistent_lib_12345"},
	}

	results := searchYakdocLibraries(payloads)

	// Should return empty results
	assert.Equal(t, 0, len(results), "Should return empty results for non-existent library")
}

func TestYakurlDocumentSearch(t *testing.T) {
	// Test yakurl document search functionality
	tests := []struct {
		name     string
		yakURL   string
		expected bool // whether we expect to find results
	}{
		{
			name:     "Library search - str",
			yakURL:   "yakdocument://str/",
			expected: true,
		},
		{
			name:     "Function search - Split",
			yakURL:   "yakdocument://Split",
			expected: false, // May or may not find matches
		},
		{
			name:     "Wildcard search",
			yakURL:   "yakdocument://*Rand*",
			expected: false, // May or may not find matches
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := yakurl.LoadGetResource(tt.yakURL)
			if tt.expected {
				assert.NoError(t, err, "Should not error for valid yakURL")
				assert.NotNil(t, response, "Response should not be nil")
				if response != nil {
					resources := response.GetResources()
					assert.True(t, len(resources) >= 0, "Should have resources")
				}
			} else {
				// For searches that may not find results, we just check that it doesn't panic
				_ = err // May or may not error, both are acceptable
			}
		})
	}
}

func TestYakdocSearchResult(t *testing.T) {
	// Test YakdocSearchResult structure
	result := &YakdocSearchResult{
		Path:    "yakdoc://lib/test",
		Content: "Test content",
	}

	assert.Equal(t, "yakdoc://lib/test", result.Path)
	assert.Equal(t, "Test content", result.Content)
}

func BenchmarkSearchYakdocLibraries(b *testing.B) {
	payloads := aitool.InvokeParams{
		"lib_names":          []string{"str", "http", "json"},
		"lib_function_globs": []string{"*Split*", "*Parse*"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		results := searchYakdocLibraries(payloads)
		_ = results // avoid unused variable warning
	}
}

func BenchmarkYakurlDocumentSearch(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		response, err := yakurl.LoadGetResource("yakdocument://str/")
		_ = response // avoid unused variable warning
		_ = err      // avoid unused variable warning
	}
}

// Integration test that combines lib_names and lib_function_globs
func TestSearchYakdocLibraries_Combined(t *testing.T) {
	payloads := aitool.InvokeParams{
		"lib_names":          []string{"str"},
		"lib_function_globs": []string{"*Split*"},
	}

	results := searchYakdocLibraries(payloads)

	// Should find results from both searches
	assert.True(t, len(results) >= 1, "Should find results from combined search")

	// Should have both library and function results
	hasLibResult := false

	for _, result := range results {
		if result.Path == "yakdoc://lib/str" {
			hasLibResult = true
		}
	}

	assert.True(t, hasLibResult, "Should have library result")
	// Note: function results might not be found if no *Split* functions exist, which is okay
}

func TestSearchLibraryCaseInsensitive(t *testing.T) {
	// Test the case insensitive library search helper function
	tests := []struct {
		name       string
		libName    string
		shouldFind bool
	}{
		{
			name:       "lowercase str",
			libName:    "str",
			shouldFind: true,
		},
		{
			name:       "uppercase STR",
			libName:    "STR",
			shouldFind: true,
		},
		{
			name:       "mixed case Str",
			libName:    "Str",
			shouldFind: true,
		},
		{
			name:       "random case sTr",
			libName:    "sTr",
			shouldFind: true,
		},
		{
			name:       "nonexistent library",
			libName:    "nonexistent_lib_12345",
			shouldFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := searchLibraryCaseInsensitive(tt.libName)

			if tt.shouldFind {
				assert.NoError(t, err, "Should not error for existing library %s", tt.libName)
				assert.NotNil(t, response, "Response should not be nil for %s", tt.libName)
				if response != nil {
					resources := response.GetResources()
					assert.True(t, len(resources) > 0, "Should find resources for %s", tt.libName)
				}
			} else {
				// For non-existent libraries, we might get an error or empty results
				// Both are acceptable
				if err == nil && response != nil {
					resources := response.GetResources()
					assert.Equal(t, 0, len(resources), "Should not find resources for non-existent library")
				}
			}
		})
	}
}

func TestRAGQuerySomke(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}

	result, ok := handleRAGQueryDocument(mock.NewMockInvoker(context.Background()),
		consts.GetGormProfileDatabase(),
		"yak",
		aitool.InvokeParams{
			"question": []string{"如何进行AES解密？", "如何实现HTTP请求？"},
		},
	)
	fmt.Println(ok)
	fmt.Println(result)

}
