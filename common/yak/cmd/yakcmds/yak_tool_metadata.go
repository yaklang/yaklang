package yakcmds

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/urfave/cli"
	yaktool "github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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
3. 使用AI分析代码内容，自动生成Description和Keywords
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
		cli.BoolFlag{
			Name:  "force,f",
			Usage: "强制更新所有脚本的元数据，即使已经有元数据",
		},
		cli.IntFlag{
			Name:  "concurrency,c",
			Usage: "并发处理的数量",
			Value: 20,
		},
	}

	command.Action = func(c *cli.Context) error {
		inputDir := c.String("input")
		outputDir := c.String("output")
		forceUpdate := c.Bool("force")
		concurrency := c.Int("concurrency")

		if inputDir == "" {
			return fmt.Errorf("input directory is required")
		}

		if outputDir == "" {
			return fmt.Errorf("output directory is required")
		}

		if concurrency <= 0 {
			concurrency = 20 // 使用默认值
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
		err := processYakScripts(inputDir, outputDir, forceUpdate, concurrency)
		if err != nil {
			return fmt.Errorf("failed to process yak scripts: %v", err)
		}

		log.Infof("Successfully processed yak scripts from %s to %s", inputDir, outputDir)
		return nil
	}

	return command
}

// 用于并发处理的工作任务
type yakScriptTask struct {
	fileInfo    utils.FileInfo
	inputDir    string
	outputDir   string
	forceUpdate bool
}

// processYakScripts 处理指定目录中的所有 yak 脚本文件
func processYakScripts(inputDir, outputDir string, forceUpdate bool, concurrency int) error {
	fileInfos, err := utils.ReadFilesRecursively(inputDir)
	if err != nil {
		return err
	}

	// 过滤出.yak文件
	var yakFiles []utils.FileInfo
	for _, info := range fileInfos {
		if !info.IsDir && strings.HasSuffix(info.Name, ".yak") {
			yakFiles = append(yakFiles, *info)
		}
	}

	log.Infof("Found %d Yak script files to process with concurrency %d", len(yakFiles), concurrency)

	// 创建任务通道和错误通道
	taskChan := make(chan yakScriptTask, len(yakFiles))
	errorChan := make(chan error, len(yakFiles))
	var wg sync.WaitGroup
	swg := utils.NewSizedWaitGroup(1)
	// 启动工作协程
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskChan {
				swg.Add(1)
				err := processYakScript(task.fileInfo, task.inputDir, task.outputDir, task.forceUpdate)
				if err != nil {
					errorChan <- fmt.Errorf("error processing %s: %v", task.fileInfo.Path, err)
				}
				swg.Done()
			}
		}()
	}

	// 分发任务
	for _, fileInfo := range yakFiles {
		taskChan <- yakScriptTask{
			fileInfo:    fileInfo,
			inputDir:    inputDir,
			outputDir:   outputDir,
			forceUpdate: forceUpdate,
		}
	}
	close(taskChan)

	// 等待所有工作完成
	wg.Wait()
	swg.Wait()
	close(errorChan)

	// 收集错误
	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		log.Errorf("Encountered %d errors during processing", len(errors))
		for _, err := range errors {
			log.Error(err)
		}
		return fmt.Errorf("encountered %d errors during processing", len(errors))
	}

	return nil
}

// processYakScript 处理单个 yak 脚本文件
func processYakScript(info utils.FileInfo, inputDir, outputDir string, forceUpdate bool) error {
	filePath := info.Path

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		log.Errorf("Failed to read file %s: %v", filePath, err)
		return err
	}

	// 解析元数据
	relPath, err := filepath.Rel(inputDir, filePath)
	if err != nil {
		log.Errorf("Failed to get relative path for %s: %v", filePath, err)
		return err
	}

	fileName := filepath.Base(filePath)
	metadata, err := yaktool.ParseYakScriptMetadata(fileName, string(content))
	if err != nil {
		log.Errorf("Failed to parse metadata for %s: %v", filePath, err)
		return err
	}

	// 检查是否需要生成元数据
	needUpdate := forceUpdate || len(metadata.Keywords) == 0 || metadata.Description == ""
	if needUpdate { // 从代码中生成描述和关键词
		generatedMetadata, err := yaktool.GenerateMetadataFromCodeContent(fileName, string(content))
		if err != nil {
			log.Errorf("Failed to generate metadata for tool: %s error: %v", metadata.Name, err)
			return err
		}

		// 如果原元数据缺失，使用生成的元数据
		if metadata.Description == "" || forceUpdate {
			metadata.Description = generatedMetadata.Description
			log.Infof("Generated description for %s: %s", filePath, metadata.Description)
		}

		if len(metadata.Keywords) == 0 || forceUpdate {
			metadata.Keywords = generatedMetadata.Keywords
			log.Infof("Generated keywords for %s: %v", filePath, metadata.Keywords)
		}

		// 生成带有新Description和Keywords的脚本内容
		newContent := generateScriptWithMetadata(string(content), metadata.Description, metadata.Keywords)
		content = []byte(newContent)
	}

	// 确保输出目录存在
	outputFilePath := filepath.Join(outputDir, relPath)
	outputFileDir := filepath.Dir(outputFilePath)
	if err := os.MkdirAll(outputFileDir, 0755); err != nil {
		log.Errorf("Failed to create output directory for %s: %v", outputFilePath, err)
		return err
	}

	// 写入文件到输出目录
	if err := os.WriteFile(outputFilePath, content, 0644); err != nil {
		log.Errorf("Failed to write file %s: %v", outputFilePath, err)
		return err
	}

	log.Infof("Processed %s -> %s", filePath, outputFilePath)
	return nil
}

// generateScriptWithMetadata 生成带有描述和关键词的脚本内容
func generateScriptWithMetadata(content string, description string, keywords []string) string {
	prog, err := static_analyzer.SSAParse(content, "yak")
	if err != nil {
		log.Errorf("Failed to parse metadata: %v", err)
		return content
	}

	contentLines := strings.Split(content, "\n")
	descRanges := make([]struct{ typ, start, end int }, 0)
	keywordsRanges := make([]struct{ typ, start, end int }, 0)

	// Find __DESC__ variables and their ranges
	prog.Ref("__DESC__").ForEach(func(value *ssaapi.Value) {
		if !value.IsConstInst() {
			return
		}
		descRange := value.GetRange()
		if descRange != nil {
			start := descRange.GetStart().GetLine()
			end := descRange.GetEnd().GetLine()
			descRanges = append(descRanges, struct{ typ, start, end int }{typ: 0, start: start, end: end})
		}
	})

	// Find __KEYWORDS__ variables and their ranges
	prog.Ref("__KEYWORDS__").ForEach(func(value *ssaapi.Value) {
		if !value.IsConstInst() {
			return
		}
		keywordsRange := value.GetRange()
		if keywordsRange != nil {
			start := keywordsRange.GetStart().GetLine()
			end := keywordsRange.GetEnd().GetLine()
			keywordsRanges = append(keywordsRanges, struct{ typ, start, end int }{typ: 1, start: start, end: end})
		}
	})

	allRange := append(descRanges, keywordsRanges...)
	// Sort ranges in reverse order to avoid index shifts when modifying the content
	sort.Slice(allRange, func(i, j int) bool {
		return allRange[i].start > allRange[j].start
	})

	// Replace or remove all __DESC__ variables
	for _, r := range allRange {
		// 确保索引在有效范围内
		if r.start <= 0 || r.end >= len(contentLines) {
			log.Warnf("Invalid range: start=%d, end=%d, content length=%d", r.start, r.end, len(contentLines))
			continue
		}

		switch r.typ {
		case 0:
			if r.start-1 >= 0 && r.end+1 <= len(contentLines) {
				contentLines = append(contentLines[:r.start-1], contentLines[r.end:]...)
			}
		case 1:
			if r.start-1 >= 0 && r.end+1 <= len(contentLines) {
				contentLines = append(contentLines[:r.start-1], contentLines[r.end:]...)
			}
		}
	}

	// Generate new declarations
	newDesc := ""
	if strings.Contains(description, "\n") {
		// Use heredoc format for multiline descriptions
		newDesc = fmt.Sprintf("__DESC__ = <<<EOF\n%s\nEOF\n\n", description)
	} else {
		newDesc = fmt.Sprintf("__DESC__ = %q\n\n", description)
	}
	newKeywords := fmt.Sprintf("__KEYWORDS__ = %q\n\n", strings.Join(keywords, ","))

	newContent := strings.TrimSpace(strings.Join(contentLines, "\n"))
	// Add new declarations at the beginning of the file
	newContent = newDesc + newKeywords + newContent
	return newContent
}
