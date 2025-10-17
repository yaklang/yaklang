package thirdparty_bin

import (
	"testing"
)

func TestFindMatchingPlatform(t *testing.T) {
	installer := &BaseInstaller{}

	// 创建测试用的下载信息映射
	downloadInfoMap := map[string]*DownloadInfo{
		"linux-amd64": {
			URL: "https://example.com/linux-amd64",
		},
		"darwin-*": {
			URL: "https://example.com/darwin",
		},
		"windows-*": {
			URL: "https://example.com/windows",
		},
		"*-arm64": {
			URL: "https://example.com/arm64",
		},
	}

	tests := []struct {
		name            string
		platformKey     string
		expectFound     bool
		expectedPattern string
		expectedURL     string
	}{
		{
			name:            "Exact match linux-amd64",
			platformKey:     "linux-amd64",
			expectFound:     true,
			expectedPattern: "linux-amd64",
			expectedURL:     "https://example.com/linux-amd64",
		},
		{
			name:            "Glob match darwin-amd64",
			platformKey:     "darwin-amd64",
			expectFound:     true,
			expectedPattern: "darwin-*",
			expectedURL:     "https://example.com/darwin",
		},
		{
			name:            "Glob match darwin-arm64",
			platformKey:     "darwin-arm64",
			expectFound:     true,
			expectedPattern: "darwin-*",
			expectedURL:     "https://example.com/darwin",
		},
		{
			name:            "Glob match windows-amd64",
			platformKey:     "windows-amd64",
			expectFound:     true,
			expectedPattern: "windows-*",
			expectedURL:     "https://example.com/windows",
		},
		{
			name:            "Glob match linux-arm64",
			platformKey:     "linux-arm64",
			expectFound:     true,
			expectedPattern: "*-arm64",
			expectedURL:     "https://example.com/arm64",
		},
		{
			name:        "No match for unsupported platform",
			platformKey: "freebsd-amd64",
			expectFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			downloadInfo, pattern, err := installer.findMatchingPlatform(downloadInfoMap, tt.platformKey)

			if tt.expectFound {
				if err != nil {
					t.Errorf("Expected to find match for %s, but got error: %v", tt.platformKey, err)
					return
				}

				if pattern != tt.expectedPattern {
					t.Errorf("Expected pattern '%s', got '%s'", tt.expectedPattern, pattern)
				}

				if downloadInfo.URL != tt.expectedURL {
					t.Errorf("Expected URL '%s', got '%s'", tt.expectedURL, downloadInfo.URL)
				}

				t.Logf("✓ Platform '%s' matched pattern '%s' -> URL: %s", tt.platformKey, pattern, downloadInfo.URL)
			} else {
				if err == nil {
					t.Errorf("Expected no match for %s, but found pattern '%s'", tt.platformKey, pattern)
				}
				t.Logf("✓ Platform '%s' correctly not matched", tt.platformKey)
			}
		})
	}
}

func TestPlatformMatchingPriority(t *testing.T) {
	installer := &BaseInstaller{}

	// 测试匹配优先级：精确匹配应该优于glob匹配
	downloadInfoMap := map[string]*DownloadInfo{
		"darwin-*": {
			URL: "https://example.com/darwin-generic",
		},
		"darwin-amd64": {
			URL: "https://example.com/darwin-amd64-specific",
		},
	}

	downloadInfo, pattern, err := installer.findMatchingPlatform(downloadInfoMap, "darwin-amd64")
	if err != nil {
		t.Fatalf("Expected to find match, got error: %v", err)
	}

	// 应该匹配精确的模式而不是glob模式
	if pattern != "darwin-amd64" {
		t.Errorf("Expected exact match 'darwin-amd64', got '%s'", pattern)
	}

	if downloadInfo.URL != "https://example.com/darwin-amd64-specific" {
		t.Errorf("Expected specific URL, got '%s'", downloadInfo.URL)
	}

	t.Logf("✓ Exact match 'darwin-amd64' correctly prioritized over glob pattern 'darwin-*'")
}

func TestGlobPatternValidation(t *testing.T) {
	installer := &BaseInstaller{}

	// 测试无效的glob模式
	downloadInfoMap := map[string]*DownloadInfo{
		"[invalid-pattern": {
			URL: "https://example.com/invalid",
		},
		"linux-amd64": {
			URL: "https://example.com/linux",
		},
	}

	// 应该跳过无效模式，匹配有效的
	downloadInfo, pattern, err := installer.findMatchingPlatform(downloadInfoMap, "linux-amd64")
	if err != nil {
		t.Fatalf("Expected to find match despite invalid pattern, got error: %v", err)
	}

	if pattern != "linux-amd64" {
		t.Errorf("Expected to match valid pattern, got '%s'", pattern)
	}

	if downloadInfo.URL != "https://example.com/linux" {
		t.Errorf("Expected correct URL, got '%s'", downloadInfo.URL)
	}

	t.Logf("✓ Invalid glob pattern correctly skipped, valid pattern matched")

	// 测试当没有有效匹配时的行为
	_, _, err = installer.findMatchingPlatform(downloadInfoMap, "freebsd-amd64")
	if err == nil {
		t.Error("Expected error when no valid patterns match")
	}
}
