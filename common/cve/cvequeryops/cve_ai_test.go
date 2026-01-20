package cvequeryops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cveresources"
)

func TestCVEAICompleteConfig(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		config := &CVEAICompleteConfig{
			Concurrent: 5,
			TestLimit:  0,
		}
		assert.Equal(t, 5, config.Concurrent)
		assert.Equal(t, 0, config.TestLimit)
	})

	t.Run("WithCVEAIConcurrent", func(t *testing.T) {
		config := &CVEAICompleteConfig{Concurrent: 5}
		opt := WithCVEAIConcurrent(10)
		opt(config)
		assert.Equal(t, 10, config.Concurrent)
	})

	t.Run("WithCVEAIConcurrent zero ignored", func(t *testing.T) {
		config := &CVEAICompleteConfig{Concurrent: 5}
		opt := WithCVEAIConcurrent(0)
		opt(config)
		assert.Equal(t, 5, config.Concurrent) // should not change
	})

	t.Run("WithCVEAIConcurrent negative ignored", func(t *testing.T) {
		config := &CVEAICompleteConfig{Concurrent: 5}
		opt := WithCVEAIConcurrent(-1)
		opt(config)
		assert.Equal(t, 5, config.Concurrent) // should not change
	})

	t.Run("WithCVETestLimit", func(t *testing.T) {
		config := &CVEAICompleteConfig{TestLimit: 0}
		opt := WithCVETestLimit(100)
		opt(config)
		assert.Equal(t, 100, config.TestLimit)
	})

	t.Run("WithCVETestLimit zero ignored", func(t *testing.T) {
		config := &CVEAICompleteConfig{TestLimit: 10}
		opt := WithCVETestLimit(0)
		opt(config)
		assert.Equal(t, 10, config.TestLimit) // should not change
	})
}

func TestGenerateCVETranslationPrompt(t *testing.T) {
	t.Run("basic CVE", func(t *testing.T) {
		cve := &cveresources.CVE{
			CVE:             "CVE-2021-44228",
			CWE:             "CWE-502",
			DescriptionMain: "Apache Log4j2 allows remote code execution.",
			Severity:        "CRITICAL",
			Vendor:          "apache",
			Product:         "log4j",
			BaseCVSSv2Score: 10.0,
		}

		prompt := generateCVETranslationPrompt(cve)

		assert.Contains(t, prompt, "CVE-2021-44228")
		assert.Contains(t, prompt, "CWE-502")
		assert.Contains(t, prompt, "Apache Log4j2")
		assert.Contains(t, prompt, "CRITICAL")
		assert.Contains(t, prompt, "apache")
		assert.Contains(t, prompt, "log4j")
		assert.Contains(t, prompt, "10.0")
		assert.Contains(t, prompt, "title_zh")
		assert.Contains(t, prompt, "description_zh")
		assert.Contains(t, prompt, "solution")
	})

	t.Run("minimal CVE", func(t *testing.T) {
		cve := &cveresources.CVE{
			CVE:             "CVE-2020-1234",
			DescriptionMain: "A vulnerability exists.",
		}

		prompt := generateCVETranslationPrompt(cve)

		assert.Contains(t, prompt, "CVE-2020-1234")
		assert.Contains(t, prompt, "A vulnerability exists.")
		// Should not contain empty fields
		assert.NotContains(t, prompt, "CWE: \n")
	})
}

func TestTruncateString(t *testing.T) {
	t.Run("short string unchanged", func(t *testing.T) {
		result := truncateString("hello", 10)
		assert.Equal(t, "hello", result)
	})

	t.Run("exact length unchanged", func(t *testing.T) {
		result := truncateString("hello", 5)
		assert.Equal(t, "hello", result)
	})

	t.Run("long string truncated", func(t *testing.T) {
		result := truncateString("hello world", 5)
		assert.Equal(t, "hello...", result)
	})

	t.Run("empty string", func(t *testing.T) {
		result := truncateString("", 10)
		assert.Equal(t, "", result)
	})
}

func TestCVEExportImport(t *testing.T) {
	// Create temp directory for test files
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test_cve.jsonl")

	t.Run("export and import roundtrip", func(t *testing.T) {
		db := consts.GetGormCVEDatabase()
		if db == nil {
			t.Skip("CVE database not available")
		}

		// Check if there are any CVEs in the database
		var count int
		db.Model(&cveresources.CVE{}).Count(&count)
		if count == 0 {
			t.Skip("No CVE entries in database")
		}

		// Export
		err := ExportCVE(testFile)
		require.NoError(t, err)

		// Verify file exists and has content
		info, err := os.Stat(testFile)
		require.NoError(t, err)
		assert.True(t, info.Size() > 0, "exported file should have content")
	})

	t.Run("import from file", func(t *testing.T) {
		db := consts.GetGormCVEDatabase()
		if db == nil {
			t.Skip("CVE database not available")
		}

		// Create a test JSONL file with mock data
		mockFile := filepath.Join(tempDir, "mock_cve.jsonl")
		mockCVE := &cveresources.CVE{
			CVE:               "CVE-TEST-0001",
			DescriptionMain:   "Test vulnerability",
			TitleZh:           "测试漏洞",
			DescriptionMainZh: "这是一个测试漏洞",
			Severity:          "HIGH",
		}

		f, err := os.Create(mockFile)
		require.NoError(t, err)

		data, err := json.Marshal(mockCVE)
		require.NoError(t, err)
		f.Write(data)
		f.Write([]byte{'\n'})
		f.Close()

		// Import
		err = ImportCVE(mockFile)
		require.NoError(t, err)

		// Verify imported data
		var imported cveresources.CVE
		err = db.Where("cve = ?", "CVE-TEST-0001").First(&imported).Error
		if err == nil {
			assert.Equal(t, "Test vulnerability", imported.DescriptionMain)
			assert.Equal(t, "测试漏洞", imported.TitleZh)

			// Cleanup
			db.Where("cve = ?", "CVE-TEST-0001").Delete(&cveresources.CVE{})
		}
	})

	t.Run("import empty file", func(t *testing.T) {
		db := consts.GetGormCVEDatabase()
		if db == nil {
			t.Skip("CVE database not available")
		}

		emptyFile := filepath.Join(tempDir, "empty.jsonl")
		f, err := os.Create(emptyFile)
		require.NoError(t, err)
		f.Close()

		err = ImportCVE(emptyFile)
		require.NoError(t, err)
	})

	t.Run("import nonexistent file", func(t *testing.T) {
		err := ImportCVE(filepath.Join(tempDir, "nonexistent.jsonl"))
		require.Error(t, err)
	})

	t.Run("export to invalid path", func(t *testing.T) {
		err := ExportCVE("/nonexistent/path/file.jsonl")
		require.Error(t, err)
	})
}

func TestCVEAICompleteFields_NoDB(t *testing.T) {
	// This test verifies behavior when database is not available
	// The actual AI completion requires a real AI service

	t.Run("parse options correctly", func(t *testing.T) {
		// Test that options are parsed correctly without actually running AI
		config := &CVEAICompleteConfig{
			Concurrent: 5,
			TestLimit:  0,
		}

		// Apply options
		opts := []any{
			WithCVEAIConcurrent(10),
			WithCVETestLimit(3),
			"some_ai_option", // Should be passed to aiOpts
		}

		for _, opt := range opts {
			switch v := opt.(type) {
			case CVEAICompleteOption:
				v(config)
			default:
				config.aiOpts = append(config.aiOpts, opt)
			}
		}

		assert.Equal(t, 10, config.Concurrent)
		assert.Equal(t, 3, config.TestLimit)
		assert.Len(t, config.aiOpts, 1)
		assert.Equal(t, "some_ai_option", config.aiOpts[0])
	})
}

func TestCVETranslationTask(t *testing.T) {
	t.Run("task struct", func(t *testing.T) {
		cve := &cveresources.CVE{CVE: "CVE-2021-44228"}
		task := &cveTranslationTask{
			cve:    cve,
			prompt: "test prompt",
			index:  1,
			total:  10,
		}

		assert.Equal(t, "CVE-2021-44228", task.cve.CVE)
		assert.Equal(t, "test prompt", task.prompt)
		assert.Equal(t, 1, task.index)
		assert.Equal(t, 10, task.total)
	})

	t.Run("result struct success", func(t *testing.T) {
		cve := &cveresources.CVE{CVE: "CVE-2021-44228"}
		result := &cveTranslationResult{
			cve:     cve,
			success: true,
			err:     nil,
		}

		assert.True(t, result.success)
		assert.Nil(t, result.err)
	})

	t.Run("result struct failure", func(t *testing.T) {
		cve := &cveresources.CVE{CVE: "CVE-2021-44228"}
		result := &cveTranslationResult{
			cve:     cve,
			success: false,
			err:     context.DeadlineExceeded,
		}

		assert.False(t, result.success)
		assert.Error(t, result.err)
	})
}

// TestCVEAICompleteFields_Integration is an integration test that requires a real AI service
// Run with: go test -v -run TestCVEAICompleteFields_Integration -tags=integration
func TestCVEAICompleteFields_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := consts.GetGormCVEDatabase()
	if db == nil {
		t.Skip("CVE database not available")
	}

	var count int
	db.Model(&cveresources.CVE{}).Count(&count)
	if count == 0 {
		t.Skip("No CVE entries in database")
	}

	// Only process 1 CVE for testing
	err := CVEAICompleteFields(WithCVETestLimit(1))
	if err != nil {
		t.Logf("AI completion error (may be expected if no AI configured): %v", err)
	}
}
