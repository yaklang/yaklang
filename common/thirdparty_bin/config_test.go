package thirdparty_bin

import (
	"testing"
)

func TestLoadConfigFromEmbedded(t *testing.T) {
	config, err := LoadConfigFromEmbedded()
	if err != nil {
		t.Fatalf("Failed to load embedded config: %v", err)
	}

	if config.Version == "" {
		t.Error("Config version should not be empty")
	}

	if config.Description == "" {
		t.Error("Config description should not be empty")
	}

	if len(config.Binaries) == 0 {
		t.Error("Config should contain at least one binary")
	}

	t.Logf("Loaded config version: %s", config.Version)
	t.Logf("Config description: %s", config.Description)
	t.Logf("Number of binaries: %d", len(config.Binaries))

	// 验证每个二进制工具的配置
	for _, binary := range config.Binaries {
		if binary.Name == "" {
			t.Error("Binary name should not be empty")
		}
		if binary.Version == "" {
			t.Error("Binary version should not be empty")
		}
		if binary.InstallType == "" {
			t.Error("Binary install type should not be empty")
		}
		if len(binary.DownloadInfoMap) == 0 {
			t.Errorf("Binary %s should have at least one download info", binary.Name)
		}

		// 验证下载信息
		for platform, downloadInfo := range binary.DownloadInfoMap {
			if downloadInfo.URL == "" {
				t.Errorf("Binary %s platform %s should have URL", binary.Name, platform)
			}
		}

		t.Logf("Binary: %s v%s (%s)", binary.Name, binary.Version, binary.InstallType)
	}
}

func TestGetBuiltinBinaryNames(t *testing.T) {
	names, err := GetBuiltinBinaryNames()
	if err != nil {
		t.Fatalf("Failed to get builtin binary names: %v", err)
	}

	if len(names) == 0 {
		t.Error("Should have at least one builtin binary")
	}

	t.Logf("Builtin binary names: %v", names)

	// 检查是否包含一些预期的工具
	expectedTools := []string{"vulinbox"}
	for _, expected := range expectedTools {
		found := false
		for _, name := range names {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected builtin tool %s not found", expected)
		}
	}
}

func TestGetBuiltinBinaryByName(t *testing.T) {
	// 测试获取已知的二进制工具
	binary, err := GetBuiltinBinaryByName("vulinbox")
	if err != nil {
		t.Fatalf("Failed to get vulinbox binary: %v", err)
	}

	if binary.Name != "vulinbox" {
		t.Errorf("Expected name 'vulinbox', got '%s'", binary.Name)
	}

	if binary.InstallType != "bin" {
		t.Errorf("Expected install type 'bin', got '%s'", binary.InstallType)
	}

	if len(binary.DownloadInfoMap) == 0 {
		t.Error("vulinbox should have download info")
	}

	// 测试获取不存在的二进制工具
	_, err = GetBuiltinBinaryByName("nonexistent")
	if err == nil {
		t.Error("Expected error when getting nonexistent binary")
	}
}

func TestParseConfig(t *testing.T) {
	// 测试解析有效的YAML配置
	validConfig := `
version: "1.0"
description: "Test config"
binaries:
  - name: "test-tool"
    description: "A test tool"
    version: "1.0.0"
    install_type: "archive"
    download_info_map:
      linux-amd64:
        url: "https://example.com/test-tool.tar.gz"
        checksums: "abc123"
        pick: "bin/test-tool"
        bin_dir: "test-tool"
        bin_path: "test-tool/test-tool"
    dependencies: []
`

	config, err := ParseConfig([]byte(validConfig))
	if err != nil {
		t.Fatalf("Failed to parse valid config: %v", err)
	}

	if config.Version != "1.0" {
		t.Errorf("Expected version '1.0', got '%s'", config.Version)
	}

	if len(config.Binaries) != 1 {
		t.Errorf("Expected 1 binary, got %d", len(config.Binaries))
	}

	binary := config.Binaries[0]
	if binary.Name != "test-tool" {
		t.Errorf("Expected name 'test-tool', got '%s'", binary.Name)
	}

	downloadInfo := binary.DownloadInfoMap["linux-amd64"]
	if downloadInfo == nil {
		t.Error("Expected linux-amd64 download info")
	} else {
		if downloadInfo.URL != "https://example.com/test-tool.tar.gz" {
			t.Errorf("Expected specific URL, got '%s'", downloadInfo.URL)
		}
		if downloadInfo.Pick != "bin/test-tool" {
			t.Errorf("Expected pick 'bin/test-tool', got '%s'", downloadInfo.Pick)
		}
	}

	// 测试解析无效的YAML配置
	invalidConfig := `
invalid yaml content
`

	_, err = ParseConfig([]byte(invalidConfig))
	if err == nil {
		t.Error("Expected error when parsing invalid config")
	}
}

func TestPickPatterns(t *testing.T) {
	config, err := LoadConfigFromEmbedded()
	if err != nil {
		t.Fatalf("Failed to load embedded config: %v", err)
	}

	// 验证不同的pick模式
	pickPatterns := map[string][]string{
		"specific_file":      {"docker/docker", "bin/helm", "terraform"},
		"directory_contents": {"node-v18.17.0-linux-x64/bin/*"},
		"directory":          {"node-v18.17.0-win-x64/"},
		"empty":              {""},
	}

	for patternType, patterns := range pickPatterns {
		t.Run(patternType, func(t *testing.T) {
			for _, binary := range config.Binaries {
				for platform, downloadInfo := range binary.DownloadInfoMap {
					for _, pattern := range patterns {
						if downloadInfo.Pick == pattern {
							t.Logf("Found %s pattern '%s' in %s:%s", patternType, pattern, binary.Name, platform)
						}
					}
				}
			}
		})
	}
}
