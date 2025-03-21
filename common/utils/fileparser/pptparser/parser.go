package pptparser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/fileparser/types"
)

// ParsePPT 解析PPT文件并返回分类好的内容
func ParsePPT(filePath string) (map[string][]types.File, error) {
	log.Infof("开始解析PPT文件: %s", filePath)

	// 解析PPT文件，获取所有节点
	var nodes []PPTNode
	var err error

	// 根据文件扩展名选择合适的解析器
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".pptx" {
		log.Infof("使用PPTX解析器解析文件: %s", filePath)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("读取PPTX文件失败: %v", err)
		}
		nodes, err = ParsePPTX(content)
		if err != nil {
			return nil, fmt.Errorf("解析PPTX文件失败: %v", err)
		}
	} else if ext == ".ppt" {
		log.Infof("使用传统PPT解析器解析文件: %s", filePath)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("读取PPT文件失败: %v", err)
		}
		nodes, err = ParsePPTX(content)
		if err != nil {
			return nil, fmt.Errorf("解析PPTX文件失败: %v", err)
		}
	} else {
		return nil, fmt.Errorf("不支持的文件类型: %s", ext)
	}

	if err != nil {
		return nil, fmt.Errorf("解析PPT文件失败: %v", err)
	}

	// 对节点进行分类
	classifier := ClassifyNodes(nodes)

	// 将分类后的节点转换为文件
	files := classifier.DumpToFiles()

	// 记录一些基本信息
	filename := filepath.Base(filePath)
	log.Infof("PPT文件 %s 解析完成，提取出 %d 个节点，生成 %d 个类型的文件",
		filename, len(nodes), len(files))

	return files, nil
}
