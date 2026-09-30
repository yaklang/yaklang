package ssatools

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// CreateSSATools 创建所有 SSA 相关的 AI 工具
func CreateSSATools() ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()

	// 注册 ssa-project-info 工具
	if err := registerProjectInfoTool(factory); err != nil {
		return nil, err
	}

	// 注册 ssa-list-files 工具
	if err := registerListFilesTool(factory); err != nil {
		return nil, err
	}

	// 注册 ssa-read-file 工具
	if err := registerReadFileTool(factory); err != nil {
		return nil, err
	}

	// 注册 ssa-grep 工具
	if err := registerGrepTool(factory); err != nil {
		return nil, err
	}

	// 注册 SyntaxFlow 工具 (check-syntaxflow-syntax)
	if err := registerSyntaxFlowTool(factory); err != nil {
		log.Errorf("register check-syntaxflow-syntax tool: %v", err)
	}

	return factory.Tools(), nil
}

// ===== ssa-project-info =====

func registerProjectInfoTool(factory *aitool.ToolFactory) error {
	return factory.RegisterTool(
		"ssa-project-info",
		aitool.WithVerboseName("SSA Project Info"),
		aitool.WithVerboseNameZh("SSA项目信息"),
		aitool.WithDescription("获取SSA项目的详细信息，包括项目元数据、文件列表、编译配置等。优先从数据库读取源码，如果不可用则尝试从原始编译路径读取"),
		aitool.WithKeywords([]string{"SSA", "项目信息", "Program", "内存模式", "源码", "project info", "metadata"}),
		aitool.WithStringParam("program_name",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("SSA项目名称"),
		),
		aitool.WithBoolParam("show_files",
			aitool.WithParam_Default(true),
			aitool.WithParam_Description("是否显示文件列表"),
		),
		aitool.WithIntegerParam("max_files",
			aitool.WithParam_Default(100),
			aitool.WithParam_Description("最多显示的文件数量"),
		),
		aitool.WithSimpleCallback(projectInfoCallback),
	)
}

func projectInfoCallback(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
	programName := params.GetString("program_name")
	showFiles := params.GetBool("show_files")
	maxFiles := int(params.GetInt("max_files"))

	if programName == "" {
		return nil, utils.Errorf("项目名称不能为空")
	}
	if maxFiles <= 0 {
		maxFiles = 100
	}

	result := map[string]any{
		"program_name":    programName,
		"exists":          false,
		"source_type":     "none",
		"has_source_code": false,
		"metadata":        map[string]any{},
		"files":           []any{},
		"file_count":      0,
		"local_path":      "",
		"error":           "",
	}

	lookup := lookupProgram(programName)
	if src := lookup.Source; src != nil && src.Kind == programSourceDatabase {
		result["exists"] = true
		result["source_type"] = string(src.Kind)
		result["has_source_code"] = true
		if src.DB != nil {
			if extraInfo := src.DB.ExtraInfo(src.Root); extraInfo != nil {
				result["metadata"] = extraInfo
			}
		}
		assignLimitedFiles(result, src, showFiles, maxFiles)
		stdout.Write([]byte(fmt.Sprintf("成功获取项目 '%s' 信息，共 %d 个文件\n", programName, result["file_count"])))
		return result, nil
	}

	if lookup.Program != nil {
		result["exists"] = true
		result["metadata"] = programMetadata(lookup.Program)
		if src := lookup.Source; src != nil && src.Kind == programSourceLocal {
			result["source_type"] = string(src.Kind)
			result["has_source_code"] = true
			result["local_path"] = src.LocalRoot
			assignLimitedFiles(result, src, showFiles, maxFiles)
			stdout.Write([]byte(fmt.Sprintf("成功获取项目 '%s' 信息（本地文件系统），共 %d 个文件\n", programName, result["file_count"])))
			return result, nil
		}

		result["source_type"] = "none"
		result["has_source_code"] = false
		result["error"] = "项目存在但源码不可访问：数据库中无源码，且原始编译路径不可用或非本地类型"
		stderr.Write([]byte(result["error"].(string) + "\n"))
		return result, nil
	}

	// 项目不存在
	result["error"] = fmt.Sprintf("项目 '%s' 不存在", programName)
	stderr.Write([]byte(result["error"].(string) + "\n"))
	return result, nil
}

// ===== ssa-list-files =====

func registerListFilesTool(factory *aitool.ToolFactory) error {
	return factory.RegisterTool(
		"ssa-list-files",
		aitool.WithVerboseName("SSA List Files"),
		aitool.WithVerboseNameZh("SSA列文件"),
		aitool.WithDescription("列出SSA项目中的所有源代码文件，优先从数据库读取，fallback到本地文件系统"),
		aitool.WithKeywords([]string{"SSA", "文件列表", "项目文件", "list files", "source code", "源码文件"}),
		aitool.WithStringParam("program_name",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("SSA项目名称"),
		),
		aitool.WithStringParam("path_prefix",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("路径前缀过滤，只显示匹配的文件"),
		),
		aitool.WithIntegerParam("offset",
			aitool.WithParam_Default(0),
			aitool.WithParam_Description("跳过前N个文件"),
		),
		aitool.WithIntegerParam("limit",
			aitool.WithParam_Default(50),
			aitool.WithParam_Description("最多返回的文件数量"),
		),
		aitool.WithSimpleCallback(listFilesCallback),
	)
}

func listFilesCallback(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
	programName := params.GetString("program_name")
	pathPrefix := params.GetString("path_prefix")
	offset := int(params.GetInt("offset"))
	limit := int(params.GetInt("limit"))

	if programName == "" {
		return nil, utils.Errorf("项目名称不能为空")
	}
	// 标准化路径前缀：移除前导斜杠
	pathPrefix = strings.TrimPrefix(pathPrefix, "/")
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}

	result := map[string]any{
		"program_name":   programName,
		"path_prefix":    pathPrefix,
		"source_type":    "none",
		"files":          []any{},
		"total_count":    0,
		"returned_count": 0,
		"offset":         offset,
		"limit":          limit,
		"has_more":       false,
		"error":          "",
	}

	lookup := lookupProgram(programName)
	if lookup.Source == nil {
		result["error"] = fmt.Sprintf("项目 '%s' 源码不可访问：数据库中无源码，且原始编译路径不可用", programName)
		stderr.Write([]byte(result["error"].(string) + "\n"))
		return result, nil
	}

	src := lookup.Source
	result["source_type"] = string(src.Kind)
	allFiles := collectListedFiles(src, pathPrefix)
	assignFilePage(result, allFiles, offset, limit)

	msg := "列出项目 '%s' 文件，返回 %d/%d 个文件\n"
	if src.Kind == programSourceLocal {
		msg = "列出项目 '%s' 文件（本地），返回 %d/%d 个文件\n"
	}
	stdout.Write([]byte(fmt.Sprintf(msg, programName, result["returned_count"], result["total_count"])))
	return result, nil
}

// ===== ssa-read-file =====

func registerReadFileTool(factory *aitool.ToolFactory) error {
	return factory.RegisterTool(
		"ssa-read-file",
		aitool.WithVerboseName("SSA Read File"),
		aitool.WithVerboseNameZh("SSA读文件"),
		aitool.WithDescription("读取SSA项目中的指定源代码文件，优先从数据库读取，fallback到本地文件系统，支持按行分页"),
		aitool.WithKeywords([]string{"SSA", "读取文件", "源代码", "read file", "source code", "按行读取", "分页"}),
		aitool.WithStringParam("program_name",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("SSA项目名称"),
		),
		aitool.WithStringParam("file_path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("要读取的文件路径"),
		),
		aitool.WithIntegerParam("start_line",
			aitool.WithParam_Default(1),
			aitool.WithParam_Description("起始行号，从1开始"),
		),
		aitool.WithIntegerParam("line_count",
			aitool.WithParam_Default(100),
			aitool.WithParam_Description("读取的行数"),
		),
		aitool.WithBoolParam("show_line_numbers",
			aitool.WithParam_Default(true),
			aitool.WithParam_Description("是否显示行号"),
		),
		aitool.WithSimpleCallback(readFileCallback),
	)
}

func readFileCallback(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
	programName := params.GetString("program_name")
	filePath := params.GetString("file_path")
	startLine := int(params.GetInt("start_line"))
	lineCount := int(params.GetInt("line_count"))
	showLineNumbers := params.GetBool("show_line_numbers")

	if programName == "" {
		return nil, utils.Errorf("项目名称不能为空")
	}
	if filePath == "" {
		return nil, utils.Errorf("文件路径不能为空")
	}
	// 标准化路径：移除前导斜杠
	filePath = strings.TrimPrefix(filePath, "/")
	if startLine < 1 {
		startLine = 1
	}
	if lineCount <= 0 {
		lineCount = 100
	}

	result := map[string]any{
		"program_name":   programName,
		"file_path":      filePath,
		"source_type":    "none",
		"start_line":     startLine,
		"line_count":     lineCount,
		"content":        "",
		"lines_returned": 0,
		"total_lines":    0,
		"file_size":      0,
		"has_more":       false,
		"error":          "",
	}

	// 格式化输出内容
	formatContent := func(sourceCode string) (string, int, int, bool, string) {
		allLines := strings.Split(sourceCode, "\n")
		totalLines := len(allLines)

		endLine := startLine + lineCount - 1
		if startLine > totalLines {
			return "", 0, totalLines, false, fmt.Sprintf("起始行号 %d 超出文件总行数 %d", startLine, totalLines)
		}
		if endLine > totalLines {
			endLine = totalLines
		}

		selectedLines := allLines[startLine-1 : endLine]
		linesReturned := len(selectedLines)
		hasMore := endLine < totalLines

		var contentBuilder strings.Builder
		for i, line := range selectedLines {
			lineNum := startLine + i
			if showLineNumbers {
				contentBuilder.WriteString(fmt.Sprintf("%6d | %s\n", lineNum, line))
			} else {
				contentBuilder.WriteString(line + "\n")
			}
		}

		return contentBuilder.String(), linesReturned, totalLines, hasMore, ""
	}

	content, kind, ok := readProgramFile(programName, filePath)
	if ok {
		result["source_type"] = string(kind)
		result["file_size"] = len(content)

		formattedContent, linesReturned, totalLines, hasMore, formatErr := formatContent(string(content))
		if formatErr != "" {
			result["error"] = formatErr
			stderr.Write([]byte(formatErr + "\n"))
		} else {
			result["content"] = formattedContent
			result["lines_returned"] = linesReturned
			result["total_lines"] = totalLines
			result["has_more"] = hasMore
			msg := "读取文件 '%s'，返回 %d/%d 行\n"
			if kind == programSourceLocal {
				msg = "读取文件 '%s'（本地），返回 %d/%d 行\n"
			}
			stdout.Write([]byte(fmt.Sprintf(msg, filePath, linesReturned, totalLines)))
		}
		return result, nil
	}

	result["error"] = fmt.Sprintf("文件 '%s' 在项目 '%s' 中不存在或不可访问", filePath, programName)
	stderr.Write([]byte(result["error"].(string) + "\n"))
	return result, nil
}

// ===== ssa-grep =====

func registerGrepTool(factory *aitool.ToolFactory) error {
	return factory.RegisterTool(
		"ssa-grep",
		aitool.WithVerboseName("SSA Grep"),
		aitool.WithVerboseNameZh("SSA搜索"),
		aitool.WithDescription("在SSA项目的源代码中搜索匹配的文本或正则表达式，优先从数据库读取，fallback到本地文件系统"),
		aitool.WithKeywords([]string{"SSA", "grep", "搜索", "代码搜索", "search", "pattern", "正则表达式", "文本查找"}),
		aitool.WithStringParam("program_name",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("SSA项目名称"),
		),
		aitool.WithStringParam("pattern",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("搜索模式"),
		),
		aitool.WithStringParam("pattern_mode",
			aitool.WithParam_Default("substr"),
			aitool.WithParam_Description("匹配模式: substr (子串), isubstr (不区分大小写), regexp (正则)"),
			aitool.WithParam_Enum("substr", "isubstr", "regexp"),
		),
		aitool.WithStringParam("file_pattern",
			aitool.WithParam_Default(""),
			aitool.WithParam_Description("文件名过滤模式，支持通配符 (如 *.java)"),
		),
		aitool.WithIntegerParam("context_lines",
			aitool.WithParam_Default(3),
			aitool.WithParam_Description("显示匹配行前后的上下文行数"),
		),
		aitool.WithIntegerParam("max_results",
			aitool.WithParam_Default(20),
			aitool.WithParam_Description("最大结果数量"),
		),
		aitool.WithSimpleCallback(grepCallback),
	)
}

func grepCallback(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
	programName := params.GetString("program_name")
	pattern := params.GetString("pattern")
	patternMode := params.GetString("pattern_mode")
	filePattern := params.GetString("file_pattern")
	contextLines := int(params.GetInt("context_lines"))
	maxResults := int(params.GetInt("max_results"))

	if programName == "" {
		return nil, utils.Errorf("项目名称不能为空")
	}
	if pattern == "" {
		return nil, utils.Errorf("搜索模式不能为空")
	}
	if contextLines < 0 {
		contextLines = 0
	}
	if maxResults <= 0 {
		maxResults = 20
	}

	result := map[string]any{
		"program_name":   programName,
		"pattern":        pattern,
		"pattern_mode":   patternMode,
		"file_pattern":   filePattern,
		"source_type":    "none",
		"matches":        []any{},
		"total_matches":  0,
		"files_searched": 0,
		"files_matched":  0,
		"truncated":      false,
		"error":          "",
	}

	// 编译正则表达式
	var patternRe *regexp.Regexp
	var err error
	switch strings.ToLower(patternMode) {
	case "regexp":
		patternRe, err = regexp.Compile(pattern)
		if err != nil {
			result["error"] = fmt.Sprintf("无效的正则表达式: %s", pattern)
			return result, nil
		}
	case "isubstr":
		patternRe = regexp.MustCompile("(?i)" + regexp.QuoteMeta(pattern))
	default: // substr
		patternRe = regexp.MustCompile(regexp.QuoteMeta(pattern))
	}

	// 文件名匹配函数
	matchFileName := func(filename string) bool {
		if filePattern == "" {
			return true
		}
		if strings.HasPrefix(filePattern, "*.") {
			ext := filePattern[1:]
			return strings.HasSuffix(filename, ext)
		}
		return strings.Contains(filename, filePattern)
	}

	// 搜索函数
	matches := []map[string]any{}
	filesSearched := 0
	filesMatched := 0
	totalMatches := 0

	searchInFile := func(filepath string, content []byte) {
		if !matchFileName(filepath) {
			return
		}
		filesSearched++

		lines := strings.Split(string(content), "\n")
		fileHasMatch := false

		for lineIdx, line := range lines {
			if len(matches) >= maxResults {
				break
			}

			matchIndexes := patternRe.FindAllStringIndex(line, -1)
			if len(matchIndexes) == 0 {
				continue
			}

			fileHasMatch = true
			totalMatches += len(matchIndexes)

			// 获取上下文
			startCtx := lineIdx - contextLines
			if startCtx < 0 {
				startCtx = 0
			}
			endCtx := lineIdx + contextLines + 1
			if endCtx > len(lines) {
				endCtx = len(lines)
			}

			// 构建上下文内容
			var contextContent strings.Builder
			for i := startCtx; i < endCtx; i++ {
				prefix := "  "
				if i == lineIdx {
					prefix = "> "
				}
				contextContent.WriteString(fmt.Sprintf("%s%4d | %s\n", prefix, i+1, lines[i]))
			}

			matches = append(matches, map[string]any{
				"file":         filepath,
				"line_number":  lineIdx + 1,
				"line_content": line,
				"context":      contextContent.String(),
				"match_count":  len(matchIndexes),
			})
		}

		if fileHasMatch {
			filesMatched++
		}
	}

	lookup := lookupProgram(programName)
	if lookup.Source != nil {
		src := lookup.Source
		result["source_type"] = string(src.Kind)
		searchProgramSource(src, func() bool { return len(matches) >= maxResults }, func(relPath string, content []byte) {
			searchInFile(relPath, content)
		})

		result["matches"] = matches
		result["total_matches"] = totalMatches
		result["files_searched"] = filesSearched
		result["files_matched"] = filesMatched
		result["truncated"] = len(matches) >= maxResults

		msg := "搜索完成，找到 %d 个匹配（%d 个文件），搜索了 %d 个文件\n"
		if src.Kind == programSourceLocal {
			msg = "搜索完成（本地），找到 %d 个匹配（%d 个文件），搜索了 %d 个文件\n"
		}
		stdout.Write([]byte(fmt.Sprintf(msg, totalMatches, filesMatched, filesSearched)))
		return result, nil
	}

	result["error"] = fmt.Sprintf("项目 '%s' 源码不可访问：数据库中无源码，且原始编译路径不可用", programName)
	stderr.Write([]byte(result["error"].(string) + "\n"))
	return result, nil
}

// ===== 辅助函数 =====

type programSourceKind string

const (
	programSourceDatabase programSourceKind = "database"
	programSourceLocal    programSourceKind = "local_fs"
)

// programSource 是某个 SSA 项目当前可读的源码树。
// 数据库目录非空时只用数据库；否则回退到编译配置里的本地目录。
type programSource struct {
	Kind programSourceKind
	FS   filesys_interface.FileSystem
	// Root 是遍历起点。数据库为 "/{program}"，本地为 "."。
	Root string
	// RelPrefix 要从数据库路径里去掉的前缀。本地为空。
	RelPrefix string
	// LocalRoot 是配置里的本地目录，仅本地源码有值。
	LocalRoot string
	DB        *ssaapi.ProgramFileSystem
}

type programLookup struct {
	Source  *programSource
	Program *ssadb.IrProgram
}

// lookupProgram 优先打开数据库中的源码目录，没有再回退到本地编译路径。
// 项目不存在时 Source 和 Program 都为空；项目在但源码不可读时只有 Program。
func lookupProgram(programName string) programLookup {
	irfs := ssaapi.NewProgramFileSystem()
	programPath := "/" + programName
	if entries, err := irfs.ReadDir(programPath); err == nil && len(entries) > 0 {
		return programLookup{Source: &programSource{
			Kind:      programSourceDatabase,
			FS:        irfs,
			Root:      programPath,
			RelPrefix: programPath,
			DB:        irfs,
		}}
	}

	irProg, err := ssadb.GetProgram(programName, ssadb.Application)
	if err != nil || irProg == nil {
		return programLookup{}
	}
	lookup := programLookup{Program: irProg}
	localPath, ok := localPathFromProgram(irProg)
	if !ok {
		return lookup
	}
	if exists, _ := filesys.NewLocalFs().Exists(localPath); !exists {
		return lookup
	}
	lookup.Source = &programSource{
		Kind:      programSourceLocal,
		FS:        filesys.NewRelLocalFs(localPath),
		Root:      ".",
		LocalRoot: localPath,
	}
	return lookup
}

func localPathFromProgram(irProg *ssadb.IrProgram) (string, bool) {
	if irProg == nil || irProg.ConfigInput == "" {
		return "", false
	}
	return tryGetLocalPathFromConfig(irProg.ConfigInput)
}

func programMetadata(irProg *ssadb.IrProgram) map[string]any {
	return map[string]any{
		"language":     string(irProg.Language),
		"program_kind": string(irProg.ProgramKind),
		"description":  irProg.Description,
		"line_count":   irProg.LineCount,
	}
}

func assignLimitedFiles(result map[string]any, src *programSource, showFiles bool, maxFiles int) {
	fileList := []map[string]any{}
	fileCount := 0
	filesys.Recursive(
		src.Root,
		filesys.WithFileSystem(src.FS),
		filesys.WithFileStat(func(filepath string, info fs.FileInfo) error {
			if info.IsDir() {
				return nil
			}
			fileCount++
			if showFiles && len(fileList) < maxFiles {
				fileList = append(fileList, map[string]any{
					"path": filepath,
					"size": info.Size(),
				})
			}
			return nil
		}),
	)
	result["file_count"] = fileCount
	if !showFiles {
		return
	}
	result["files"] = fileList
	if fileCount > maxFiles {
		result["files_truncated"] = true
		result["files_truncated_message"] = fmt.Sprintf("仅显示前 %d 个文件，共 %d 个文件", maxFiles, fileCount)
	}
}

func collectListedFiles(src *programSource, pathPrefix string) []map[string]any {
	allFiles := []map[string]any{}
	filesys.Recursive(
		src.Root,
		filesys.WithFileSystem(src.FS),
		filesys.WithFileStat(func(filepath string, info fs.FileInfo) error {
			if info.IsDir() {
				return nil
			}
			relPath := filepath
			if src.RelPrefix != "" {
				relPath = strings.TrimPrefix(filepath, src.RelPrefix)
				relPath = strings.TrimPrefix(relPath, "/")
			}
			relPath = strings.TrimPrefix(relPath, "/")
			if pathPrefix != "" && !strings.HasPrefix(relPath, pathPrefix) {
				return nil
			}
			allFiles = append(allFiles, map[string]any{
				"path": relPath,
				"size": info.Size(),
			})
			return nil
		}),
	)
	return allFiles
}

func assignFilePage(result map[string]any, allFiles []map[string]any, offset, limit int) {
	result["total_count"] = len(allFiles)
	startIdx := offset
	endIdx := offset + limit
	if startIdx >= len(allFiles) {
		result["files"] = []any{}
		result["returned_count"] = 0
		result["has_more"] = false
		return
	}
	if endIdx > len(allFiles) {
		endIdx = len(allFiles)
	}
	result["files"] = allFiles[startIdx:endIdx]
	result["returned_count"] = endIdx - startIdx
	result["has_more"] = endIdx < len(allFiles)
}

// readProgramFile 按单个文件读取：数据库里有这个文件就用数据库，否则再试本地路径。
func readProgramFile(programName, relPath string) ([]byte, programSourceKind, bool) {
	irfs := ssaapi.NewProgramFileSystem()
	fullPath := "/" + programName + "/" + relPath
	if content, err := irfs.ReadFile(fullPath); err == nil {
		return content, programSourceDatabase, true
	}

	irProg, err := ssadb.GetProgram(programName, ssadb.Application)
	if err != nil || irProg == nil {
		return nil, "", false
	}
	localPath, ok := localPathFromProgram(irProg)
	if !ok {
		return nil, "", false
	}
	localFs := filesys.NewLocalFs()
	fullLocalPath := localFs.Join(localPath, relPath)
	exists, _ := localFs.Exists(fullLocalPath)
	if !exists {
		return nil, "", false
	}
	content, err := localFs.ReadFile(fullLocalPath)
	if err != nil {
		return nil, "", false
	}
	return content, programSourceLocal, true
}

func searchProgramSource(src *programSource, stop func() bool, visit func(relPath string, content []byte)) {
	filesys.Recursive(
		src.Root,
		filesys.WithFileSystem(src.FS),
		filesys.WithFileStat(func(filepath string, info fs.FileInfo) error {
			if info.IsDir() || stop() {
				return nil
			}
			content, err := readSourceFile(src, filepath)
			if err != nil {
				return nil
			}
			relPath := filepath
			if src.RelPrefix != "" {
				relPath = strings.TrimPrefix(filepath, src.RelPrefix)
				relPath = strings.TrimPrefix(relPath, "/")
			}
			visit(relPath, content)
			return nil
		}),
	)
}

func readSourceFile(src *programSource, filepath string) ([]byte, error) {
	if src.Kind == programSourceLocal {
		localFs := filesys.NewLocalFs()
		return localFs.ReadFile(localFs.Join(src.LocalRoot, filepath))
	}
	return src.FS.ReadFile(filepath)
}

// tryGetLocalPathFromConfig 尝试从 ConfigInput JSON 中提取本地文件路径
func tryGetLocalPathFromConfig(configInput string) (string, bool) {
	if configInput == "" {
		return "", false
	}

	var configInfo map[string]any
	if err := json.Unmarshal([]byte(configInput), &configInfo); err != nil {
		return "", false
	}

	codeSource, ok := configInfo["CodeSource"].(map[string]any)
	if !ok {
		return "", false
	}

	kind, _ := codeSource["kind"].(string)
	localFile, _ := codeSource["local_file"].(string)

	if kind == "local" && localFile != "" {
		return localFile, true
	}

	return "", false
}
