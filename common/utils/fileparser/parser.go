package fileparser

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/fileparser/excelparser"
	"github.com/yaklang/yaklang/common/utils/fileparser/pptparser"
	"github.com/yaklang/yaklang/common/utils/fileparser/types"
	"github.com/yaklang/yaklang/common/utils/fileparser/wordparser"
)

// FileType 定义支持的文件类型
type FileType string

const (
	FileTypeWord  FileType = "word"
	FileTypeExcel FileType = "excel"
	FileTypePPT   FileType = "ppt" // 新增PPT文件类型
	// 可以在这里添加其他文件类型的支持
)

func ParseFileElements(filePath string) (map[string][]types.File, error) {
	// 获取文件扩展名
	ext := strings.ToLower(filepath.Ext(filePath))

	// 根据扩展名识别文件类型
	var fileType FileType
	switch ext {
	case ".docx", ".doc":
		fileType = FileTypeWord
	case ".xls", ".xlsx":
		fileType = FileTypeExcel
	case ".ppt", ".pptx", ".pptm":
		fileType = FileTypePPT
	default:
		return nil, errors.New("不支持的文件类型：" + ext)
	}

	log.Infof("开始解析文件: %s，类型: %s", filePath, fileType)

	var result map[string][]types.File

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
		result = classifier.DumpToFiles()
		log.Infof("成功解析Word文档，共导出 %d 种类型的文件", len(result))

	case FileTypeExcel:
		// 解析Excel文件
		nodes, err := excelparser.ParseExcelFile(filePath)
		if err != nil {
			return nil, err
		}

		// 使用分类器处理节点并转换为文件
		classifier := excelparser.ClassifyNodes(nodes)
		result = classifier.DumpToFiles()
		log.Infof("成功解析Excel文件，共导出 %d 种类型的文件", len(result))
	case FileTypePPT:
		// 解析PPT文件
		var err error
		result, err = pptparser.ParsePPT(filePath)
		if err != nil {
			return nil, err
		}

		log.Infof("成功解析PPT文件，共导出 %d 种类型的文件", len(result))

	default:
		return nil, errors.New("未实现的文件类型处理：" + string(fileType))
	}

	return result, nil
}

// GetSupportedExtensions 返回支持的文件扩展名列表
func GetSupportedExtensions() []string {
	return []string{
		".docx",
		".doc",
		".xls",
		".xlsx",
		".ppt",
		".pptx",
		".pptm",
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
