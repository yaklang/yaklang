package fileparser

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/fileparser/wordparser"
)

// FileType 定义支持的文件类型
type FileType string

const (
	FileTypeWord FileType = "word"
	// 可以在这里添加其他文件类型的支持
)

// ParseResult 定义解析结果
type ParseResult struct {
	FileType FileType                                  // 文件类型
	Files    map[wordparser.FileType][]wordparser.File // 解析出的文件
}

// ParseFile 解析文件，返回解析结果
func ParseFile(filePath string) (*ParseResult, error) {
	// 获取文件扩展名
	ext := strings.ToLower(filepath.Ext(filePath))

	// 根据扩展名识别文件类型
	var fileType FileType
	switch ext {
	case ".docx", ".doc":
		fileType = FileTypeWord
	default:
		return nil, errors.New("不支持的文件类型：" + ext)
	}

	log.Infof("开始解析文件: %s，类型: %s", filePath, fileType)

	var result ParseResult
	result.FileType = fileType

	// 根据文件类型调用对应的解析函数
	switch fileType {
	case FileTypeWord:
		// 解析Word文档
		nodes, err := wordparser.ParseWord(filePath)
		if err != nil {
			return nil, err
		}

		// 分类节点
		classifier := wordparser.ClassifyNodes(nodes)

		// 导出为文件
		result.Files = classifier.DumpToFiles()
		log.Infof("成功解析Word文档，共导出 %d 种类型的文件", len(result.Files))

	default:
		return nil, errors.New("未实现的文件类型处理：" + string(fileType))
	}

	return &result, nil
}

// GetSupportedExtensions 返回支持的文件扩展名列表
func GetSupportedExtensions() []string {
	return []string{
		".docx",
		".doc",
		// 可以在这里添加其他支持的扩展名
	}
}

// IsSupportedExtension 检查文件扩展名是否支持
func IsSupportedExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	for _, supported := range GetSupportedExtensions() {
		if ext == supported {
			return true
		}
	}
	return false
}
