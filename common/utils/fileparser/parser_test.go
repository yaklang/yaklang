package fileparser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/log"
)

func TestParseFile(t *testing.T) {
	// 测试文件路径
	testFile := "/Users/z3/Downloads/doc1.docx"

	// 检查文件是否存在
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("跳过测试：测试文件不存在，请提供一个真实的Word文档进行测试")
	}

	// 创建输出目录
	outputDir := "/tmp/yaklang_test_output"
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		t.Fatalf("创建输出目录失败: %v", err)
	}
	// defer os.RemoveAll(outputDir) // 测试完成后清理

	// 解析文件
	result, err := ParseFile(testFile)
	if err != nil {
		t.Fatalf("解析文件失败: %v", err)
	}

	// 验证结果
	if result.FileType != FileTypeWord {
		t.Errorf("文件类型不匹配，期望: %s，实际: %s", FileTypeWord, result.FileType)
	}

	// 将解析结果写入文件
	for fileType, files := range result.Files {
		// 为每种类型创建子目录
		typeDir := filepath.Join(outputDir, string(fileType))
		err := os.MkdirAll(typeDir, 0755)
		if err != nil {
			t.Errorf("创建目录失败 %s: %v", typeDir, err)
			continue
		}

		// 写入文件
		for _, file := range files {
			filePath := filepath.Join(typeDir, file.Name)
			err := os.WriteFile(filePath, file.Data, 0644)
			if err != nil {
				t.Errorf("写入文件失败 %s: %v", filePath, err)
				continue
			}
			log.Infof("已写入文件: %s，大小: %s", filePath, file.Metadata["size"])
		}
	}

	// 输出统计信息
	t.Run("文件类型统计", func(t *testing.T) {
		for fileType, files := range result.Files {
			t.Logf("类型 %s: %d 个文件", fileType, len(files))
		}
	})
}

func TestGetSupportedExtensions(t *testing.T) {
	extensions := GetSupportedExtensions()
	if len(extensions) == 0 {
		t.Error("支持的扩展名列表为空")
	}

	// 验证是否包含基本的Word文档扩展名
	hasDocx := false
	hasDoc := false
	for _, ext := range extensions {
		switch ext {
		case ".docx":
			hasDocx = true
		case ".doc":
			hasDoc = true
		}
	}

	if !hasDocx {
		t.Error("不支持 .docx 扩展名")
	}
	if !hasDoc {
		t.Error("不支持 .doc 扩展名")
	}
}

func TestIsSupportedExtension(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"test.docx", true},
		{"test.doc", true},
		{"test.pdf", false},
		{"test.txt", false},
		{"test", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := IsSupportedExtension(tt.filename)
			if got != tt.want {
				t.Errorf("IsSupportedExtension(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}
