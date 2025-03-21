package fileparser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/utils/fileparser/resources"
)

func TestParseFile(t *testing.T) {
	exts := []string{"docx", "pptx", "xlsx"}
	for _, ext := range exts {
		fileName := "test." + ext
		testFileContent, err := resources.FS.ReadFile(fileName)
		if err != nil {
			t.Fatalf("读取文件失败: %v", err)
		}

		tmpDir, err := os.MkdirTemp("", "yaklang_test_output_*")
		if err != nil {
			t.Fatalf("创建临时目录失败: %v", err)
		}
		defer os.RemoveAll(tmpDir)
		tempFile, err := os.CreateTemp(tmpDir, fileName)
		if err != nil {
			t.Fatalf("创建临时文件失败: %v", err)
		}
		defer os.Remove(tempFile.Name())
		os.MkdirAll(tmpDir, 0755)
		err = os.WriteFile(filepath.Join(tmpDir, fileName), testFileContent, 0644)
		if err != nil {
			t.Fatalf("创建临时文件失败: %v", err)
		}
		result, err := ParseFileElements(filepath.Join(tmpDir, fileName))
		if err != nil {
			t.Fatalf("解析文件失败: %v", err)
		}
		for _, files := range result {
			for _, file := range files {
				filePath := filepath.Join(tmpDir, file.FileName)
				// 先检测文件所有文件夹是否存在，不存在则创建文件夹
				dir := filepath.Dir(filePath)
				if _, err := os.Stat(dir); os.IsNotExist(err) {
					err := os.MkdirAll(dir, 0755)
					if err != nil {
						t.Errorf("创建文件夹失败 %s: %v", dir, err)
					}
				}
				err := os.WriteFile(filePath, file.BinaryData, 0644)
				if err != nil {
					t.Errorf("写入文件失败 %s: %v", filePath, err)
				}
			}
		}
	}
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
