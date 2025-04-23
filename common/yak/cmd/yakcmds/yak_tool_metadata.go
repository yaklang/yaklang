package yakcmds

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urfave/cli"
	yaktool "github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

func createYakToolMetadataCommand() *cli.Command {
	command := &cli.Command{}
	command.Name = "yak-tool-metadata"
	command.Description = "yak-tool-metadata 是一个用于生成和更新 yak tool metadata 的命令行工具"
	command.UsageText = `format: yak yak-tool-metadata --input <yak_tool_dir> --output <output_dir>

此命令用于处理 yak 脚本工具的元数据：
1. 读取 yak 工具目录中的所有脚本
2. 提取每个脚本的元数据（使用 ParseYakScriptMetadata 函数）
3. 如果脚本元数据中 Keywords 为空但有 Description，则自动生成 Keywords
4. 重新写入到输出目录

示例:
yak yak-tool-metadata --input ./tools --output ./tools_with_metadata
`
	command.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "input,i",
			Usage: "Yak tool 目录路径",
		},
		cli.StringFlag{
			Name:  "output,o",
			Usage: "输出目录路径",
		},
	}

	command.Action = func(c *cli.Context) error {
		inputDir := c.String("input")
		outputDir := c.String("output")

		if inputDir == "" {
			return fmt.Errorf("input directory is required")
		}

		if outputDir == "" {
			return fmt.Errorf("output directory is required")
		}

		// 检查输入目录是否存在
		if _, err := os.Stat(inputDir); os.IsNotExist(err) {
			return fmt.Errorf("input directory does not exist: %s", inputDir)
		}

		// 创建输出目录（如果不存在）
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %v", err)
		}

		// 处理所有脚本
		err := processYakScripts(inputDir, outputDir)
		if err != nil {
			return fmt.Errorf("failed to process yak scripts: %v", err)
		}

		log.Infof("Successfully processed yak scripts from %s to %s", inputDir, outputDir)
		return nil
	}

	return command
}

// processYakScripts 处理指定目录中的所有 yak 脚本文件
func processYakScripts(inputDir, outputDir string) error {
	fileInfos, err := utils.ReadFilesRecursively(inputDir)
	if err != nil {
		return err
	}

	// 处理每个文件
	for _, info := range fileInfos {
		var filePath string
		if !info.IsDir && strings.HasSuffix(info.Name, ".yak") {
			filePath = info.Path
		} else {
			continue
		}
		// 读取文件内容
		content, err := os.ReadFile(filePath)
		if err != nil {
			log.Errorf("Failed to read file %s: %v", filePath, err)
			continue
		}

		// 解析元数据
		relPath, err := filepath.Rel(inputDir, filePath)
		if err != nil {
			log.Errorf("Failed to get relative path for %s: %v", filePath, err)
			continue
		}

		fileName := filepath.Base(filePath)
		metadata, err := yaktool.ParseYakScriptMetadata(fileName, string(content))
		if err != nil {
			log.Errorf("Failed to parse metadata for %s: %v", filePath, err)
			continue
		}

		// 检查是否需要生成 Keywords
		if len(metadata.Keywords) == 0 && metadata.Description != "" {
			// 从描述中生成关键词
			var err error
			metadata.Keywords, err = yaktool.GenerateKeywordsByDescription(metadata.Description)
			if err != nil {
				log.Errorf("Generated keywords for tool: %s error: %v", metadata.Name, err)
				continue
			}
			log.Infof("Generated keywords for %s: %v", filePath, metadata.Keywords)

			// 生成带有新 Keywords 的脚本内容
			newContent := generateScriptWithKeywords(string(content), metadata.Keywords)
			content = []byte(newContent)
		}

		// 确保输出目录存在
		outputFilePath := filepath.Join(outputDir, relPath)
		outputFileDir := filepath.Dir(outputFilePath)
		if err := os.MkdirAll(outputFileDir, 0755); err != nil {
			log.Errorf("Failed to create output directory for %s: %v", outputFilePath, err)
			continue
		}

		// 写入文件到输出目录
		if err := os.WriteFile(outputFilePath, content, 0644); err != nil {
			log.Errorf("Failed to write file %s: %v", outputFilePath, err)
			continue
		}

		log.Infof("Processed %s -> %s", filePath, outputFilePath)
	}

	return nil
}

// generateScriptWithKeywords 生成带有关键词的脚本内容
func generateScriptWithKeywords(content string, keywords []string) string {
	keywordsLine := fmt.Sprintf("__KEYWORDS__ = \"%s\"\n", strings.Join(keywords, ","))
	prog, err := static_analyzer.SSAParse(content, "yak")
	if err != nil {
		log.Errorf("static_analyzer.SSAParse(string(content), \"yak\") error: %v", err)
		return keywordsLine + "\n" + content
	}

	keywordsIns := prog.Ref("__KEYWORDS__")
	if len(keywordsIns) == 0 {
		// 如果没有找到 __KEYWORDS__ 定义，直接在文件开头添加
		return keywordsLine + "\n" + content
	}

	// 有 __KEYWORDS__ 定义，处理每个实例
	var result string = content
	var replacedFirst bool

	// 按照偏移量从大到小排序，避免替换后位置变化
	sort.Slice(keywordsIns, func(i, j int) bool {
		return keywordsIns[i].GetRange().GetStartOffset() > keywordsIns[j].GetRange().GetStartOffset()
	})

	for _, ins := range keywordsIns {
		rangeIf := ins.GetRange()
		startOffset := rangeIf.GetStartOffset()
		endOffset := rangeIf.GetEndOffset()

		if !replacedFirst {
			// 第一个实例替换为新的关键词行
			result = result[:startOffset] + keywordsLine + result[endOffset:]
			replacedFirst = true
		} else {
			// 后续实例删除
			result = result[:startOffset] + result[endOffset:]
		}
	}

	return result
}
