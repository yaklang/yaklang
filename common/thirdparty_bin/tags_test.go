package thirdparty_bin

import (
	"testing"
)

func TestTagsSupport(t *testing.T) {
	// 创建一个Manager进行测试
	manager, err := NewManager("", "/tmp/test_install")
	if err != nil {
		t.Fatalf("创建manager失败: %v", err)
	}

	// 创建测试用的BinaryDescriptor
	descriptor1 := &BinaryDescriptor{
		Name:        "tool1",
		Description: "测试工具1",
		Tags:        []string{"scanner", "security", "network"},
		Version:     "1.0.0",
		InstallType: "bin",
		DownloadInfoMap: map[string]*DownloadInfo{
			"linux-amd64": {
				URL: "https://example.com/tool1",
			},
		},
	}

	descriptor2 := &BinaryDescriptor{
		Name:        "tool2",
		Description: "测试工具2",
		Tags:        []string{"scanner", "web"},
		Version:     "2.0.0",
		InstallType: "bin",
		DownloadInfoMap: map[string]*DownloadInfo{
			"linux-amd64": {
				URL: "https://example.com/tool2",
			},
		},
	}

	descriptor3 := &BinaryDescriptor{
		Name:        "tool3",
		Description: "测试工具3",
		Tags:        []string{"security", "encryption"},
		Version:     "3.0.0",
		InstallType: "bin",
		DownloadInfoMap: map[string]*DownloadInfo{
			"linux-amd64": {
				URL: "https://example.com/tool3",
			},
		},
	}

	// 注册测试工具
	if err := manager.Register(descriptor1); err != nil {
		t.Fatalf("注册tool1失败: %v", err)
	}
	if err := manager.Register(descriptor2); err != nil {
		t.Fatalf("注册tool2失败: %v", err)
	}
	if err := manager.Register(descriptor3); err != nil {
		t.Fatalf("注册tool3失败: %v", err)
	}

	// 测试GetBinaryNamesByTags - 查找包含所有指定标签的工具
	t.Run("GetBinaryNamesByTags", func(t *testing.T) {
		// 查找同时包含"scanner"和"security"标签的工具
		result := manager.GetBinaryNamesByTags([]string{"scanner", "security"})
		expected := []string{"tool1"} // 只有tool1同时包含这两个标签

		if len(result) != len(expected) {
			t.Fatalf("期望返回%d个工具，实际返回%d个", len(expected), len(result))
		}

		for i, name := range result {
			if name != expected[i] {
				t.Errorf("期望第%d个工具是%s，实际是%s", i, expected[i], name)
			}
		}
	})

	// 测试GetBinaryNamesByAnyTag - 查找包含任意指定标签的工具
	t.Run("GetBinaryNamesByAnyTag", func(t *testing.T) {
		// 查找包含"web"或"encryption"标签的工具
		result := manager.GetBinaryNamesByAnyTag([]string{"web", "encryption"})
		expected := []string{"tool2", "tool3"} // tool2有web标签，tool3有encryption标签

		if len(result) != len(expected) {
			t.Fatalf("期望返回%d个工具，实际返回%d个", len(expected), len(result))
		}

		for i, name := range result {
			if name != expected[i] {
				t.Errorf("期望第%d个工具是%s，实际是%s", i, expected[i], name)
			}
		}
	})

	// 测试空标签列表
	t.Run("EmptyTags", func(t *testing.T) {
		result := manager.GetBinaryNamesByTags([]string{})
		if len(result) != 0 {
			t.Errorf("空标签列表应该返回空结果，实际返回%d个工具", len(result))
		}

		result = manager.GetBinaryNamesByAnyTag([]string{})
		if len(result) != 0 {
			t.Errorf("空标签列表应该返回空结果，实际返回%d个工具", len(result))
		}
	})

	// 测试不存在的标签
	t.Run("NonExistentTags", func(t *testing.T) {
		result := manager.GetBinaryNamesByTags([]string{"nonexistent"})
		if len(result) != 0 {
			t.Errorf("不存在的标签应该返回空结果，实际返回%d个工具", len(result))
		}

		result = manager.GetBinaryNamesByAnyTag([]string{"nonexistent"})
		if len(result) != 0 {
			t.Errorf("不存在的标签应该返回空结果，实际返回%d个工具", len(result))
		}
	})
}

func TestConfigParsingWithTags(t *testing.T) {
	// 测试配置解析是否正确处理tags字段
	yamlContent := `
version: "1.0"
description: "测试配置"
binaries:
  - name: "test_tool"
    description: "测试工具"
    tags: ["scanner", "security"]
    version: "1.0.0"
    install_type: "bin"
    download_info_map:
      linux-amd64:
        url: "https://example.com/test_tool"
`

	config, err := ParseConfig([]byte(yamlContent))
	if err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}

	if len(config.Binaries) != 1 {
		t.Fatalf("期望1个binary，实际有%d个", len(config.Binaries))
	}

	binary := config.Binaries[0]
	if len(binary.Tags) != 2 {
		t.Fatalf("期望2个tags，实际有%d个", len(binary.Tags))
	}

	expectedTags := []string{"scanner", "security"}
	for i, tag := range binary.Tags {
		if tag != expectedTags[i] {
			t.Errorf("期望第%d个tag是%s，实际是%s", i, expectedTags[i], tag)
		}
	}
}
