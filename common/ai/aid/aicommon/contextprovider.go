package aicommon

import (
	"bytes"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

const (
	CONTEXT_PROVIDER_TYPE_FILE           = "file"
	CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE = "knowledge_base"
	CONTEXT_PROVIDER_TYPE_AITOOL         = "aitool"
	CONTEXT_PROVIDER_TYPE_AIFORGE        = "aiforge"
	CONTEXT_PROVIDER_TYPE_AISKILL        = "aiskill"

	CONTEXT_PROVIDER_KEY_FILE_PATH    = "file_path"
	CONTEXT_PROVIDER_KEY_FILE_CONTENT = "file_content"
	CONTEXT_PROVIDER_KEY_NAME         = "name"

	CONTEXT_PROVIDER_KEY_SYSTEM_FLAG = "system_flag"

	CONTEXT_PROVIDER_VALUE_ALL_KNOWLEDGE_BASE         = "all_knowledge_base"
	CONTEXT_PROVIDER_VALUE_AUTO_SELECT_KNOWLEDGE_BASE = "auto_select_knowledge_base"
)

type ContextProviderEntry struct {
	Name     string
	Provider ContextProvider
	Traced   bool
}

type ContextProvider func(config AICallerConfigIf, emitter *Emitter, key string) (string, error)

// isTextMimeType 判断是否为文本类型的 MIME
func isTextMimeType(mimeType string) bool {
	// 移除 charset 等参数，只保留主类型
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	// text/* 类型都是文本
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}

	// 常见的文本类型 MIME
	textMimeTypes := map[string]bool{
		"application/json":                  true,
		"application/xml":                   true,
		"application/javascript":            true,
		"application/x-javascript":          true,
		"application/typescript":            true,
		"application/x-yaml":                true,
		"application/yaml":                  true,
		"application/x-sh":                  true,
		"application/x-shellscript":         true,
		"application/x-python":              true,
		"application/x-ruby":                true,
		"application/x-perl":                true,
		"application/x-php":                 true,
		"application/sql":                   true,
		"application/graphql":               true,
		"application/ld+json":               true,
		"application/x-httpd-php":           true,
		"application/xhtml+xml":             true,
		"application/atom+xml":              true,
		"application/rss+xml":               true,
		"application/x-www-form-urlencoded": true,
	}

	return textMimeTypes[mimeType]
}

// isTextFileExtension 根据扩展名判断是否为文本文件
func isTextFileExtension(ext string) bool {
	ext = strings.ToLower(ext)
	textExtensions := map[string]bool{
		// 纯文本
		".txt": true, ".text": true, ".log": true,
		".yak": true,
		// Markdown 和文档
		".md": true, ".markdown": true, ".rst": true, ".adoc": true,
		// 配置文件
		".json": true, ".yaml": true, ".yml": true, ".toml": true, ".xml": true,
		".ini": true, ".conf": true, ".cfg": true, ".config": true, ".properties": true,
		".env": true, ".env.local": true, ".env.development": true, ".env.production": true,
		// 数据文件
		".csv": true, ".tsv": true,
		// 编程语言
		".go": true, ".py": true, ".js": true, ".ts": true, ".jsx": true, ".tsx": true,
		".java": true, ".c": true, ".cpp": true, ".cc": true, ".cxx": true, ".h": true, ".hpp": true,
		".cs": true, ".rs": true, ".rb": true, ".php": true, ".swift": true, ".kt": true, ".kts": true,
		".scala": true, ".groovy": true, ".lua": true, ".pl": true, ".pm": true, ".r": true,
		".sql": true, ".graphql": true, ".gql": true,
		// Shell 脚本
		".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".ps1": true, ".bat": true, ".cmd": true,
		// Web 相关
		".html": true, ".htm": true, ".css": true, ".scss": true, ".sass": true, ".less": true,
		".vue": true, ".svelte": true,
		// 其他
		".proto": true, ".thrift": true, ".avro": true,
		".makefile": true, ".dockerfile": true, ".gitignore": true, ".gitattributes": true,
		".editorconfig": true, ".prettierrc": true, ".eslintrc": true, ".babelrc": true,
	}

	return textExtensions[ext]
}

// MaxFileContentSize 文件内容最大读取大小（1KB）
const MaxFileContentSize = 1024

func FileContextProvider(filePath string, userPrompt ...string) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		// 构建基本信息（即使出错也要包含）
		baseInfo := fmt.Sprintf("User Prompt: %s\nFile: %s\n", strings.Join(userPrompt, " "), filePath)

		if !utils.FileExists(filePath) {
			return baseInfo + "[Error: file does not exist]", utils.Errorf("file %s does not exist", filePath)
		}

		ext := filepath.Ext(filePath)
		fileName := filepath.Base(filePath)

		// 1. 先使用 mime.TypeByExtension 获取 MIME 类型
		mimeType := mime.TypeByExtension(ext)

		// 2. 判断是否为文本类型
		isText := false
		if mimeType != "" {
			isText = isTextMimeType(mimeType)
		}
		// 如果 MIME 类型未知，回退到扩展名判断
		if !isText {
			isText = isTextFileExtension(ext)
		}
		// 特殊处理：没有扩展名的文件（如 Makefile, Dockerfile）
		if ext == "" {
			lowerName := strings.ToLower(fileName)
			if lowerName == "makefile" || lowerName == "dockerfile" || lowerName == "vagrantfile" ||
				lowerName == "gemfile" || lowerName == "rakefile" || lowerName == "procfile" {
				isText = true
			}
		}

		// 3. 非文本类型，返回未实现错误（包含文件路径信息）
		if !isText {
			errInfo := fmt.Sprintf("[Error: file type '%s' (MIME: %s) is not supported yet, only text files are supported]", ext, mimeType)
			return baseInfo + errInfo, utils.Errorf("file %s: file type '%s' (MIME: %s) is not supported yet, only text files are supported", filePath, ext, mimeType)
		}

		// 4. 读取文本文件内容
		file, err := os.Open(filePath)
		if err != nil {
			return baseInfo + fmt.Sprintf("[Error: failed to open file: %v]", err), utils.Errorf("failed to open file %s: %w", filePath, err)
		}
		defer file.Close()

		// 获取文件大小
		fileInfo, err := file.Stat()
		if err != nil {
			return baseInfo + fmt.Sprintf("[Error: failed to stat file: %v]", err), utils.Errorf("failed to stat file %s: %w", filePath, err)
		}
		fileSize := fileInfo.Size()

		// 读取内容，限制为最大 1KB
		var content string
		var truncated bool
		if fileSize > MaxFileContentSize {
			// 文件过大，只读取前 1KB
			buffer := make([]byte, MaxFileContentSize)
			n, err := file.Read(buffer)
			if err != nil {
				return baseInfo + fmt.Sprintf("File Size: %d bytes\n[Error: failed to read file: %v]", fileSize, err), utils.Errorf("failed to read file %s: %w", filePath, err)
			}
			content = string(buffer[:n])
			truncated = true
		} else {
			// 文件较小，全部读取
			contentBytes, err := os.ReadFile(filePath)
			if err != nil {
				return baseInfo + fmt.Sprintf("File Size: %d bytes\n[Error: failed to read file: %v]", fileSize, err), utils.Errorf("failed to read file %s: %w", filePath, err)
			}
			content = string(contentBytes)
		}

		// 5. 构建提示词
		var result bytes.Buffer
		result.WriteString(baseInfo)
		if mimeType != "" {
			result.WriteString(fmt.Sprintf("MIME Type: %s\n", mimeType))
		}
		result.WriteString(fmt.Sprintf("File Size: %d bytes\n", fileSize))
		if truncated {
			result.WriteString(fmt.Sprintf("Note: File content truncated to first %d bytes (original size: %d bytes)\n", MaxFileContentSize, fileSize))
		}
		result.WriteString("\n--- File Content ---\n")
		result.WriteString(content)
		if truncated {
			result.WriteString("\n... (content truncated) ...")
		}
		result.WriteString("\n--- End of File Content ---\n")

		return result.String(), nil
	}
}

func KnowledgeBaseContextProvider(knowledgeBaseName string, userPrompt ...string) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		// 构建基本信息（即使出错也要包含）
		baseInfo := fmt.Sprintf("User Prompt: %s\n============== Knowledge Base Info ==============\nName: %s\n", strings.Join(userPrompt, " "), knowledgeBaseName)

		knowledgeBase, err := yakit.GetKnowledgeBaseByName(consts.GetGormProfileDatabase(), knowledgeBaseName)
		if err != nil {
			return baseInfo + fmt.Sprintf("[Error: failed to get knowledge base: %v]", err), utils.Errorf("failed to get knowledge base %s: %w", knowledgeBaseName, err)
		}
		var infoBuffer bytes.Buffer
		infoBuffer.WriteString(baseInfo)
		infoBuffer.WriteString(fmt.Sprintf("Description: %s\n", knowledgeBase.KnowledgeBaseDescription))
		infoBuffer.WriteString(fmt.Sprintf("Type: %s\n", knowledgeBase.KnowledgeBaseType))
		infoBuffer.WriteString(fmt.Sprintf("Tags: %s\n", knowledgeBase.Tags))
		infoBuffer.WriteString("\n============== Important Instructions ==============\n")
		infoBuffer.WriteString(fmt.Sprintf("查询时请指定知识库名称为: %s\n", knowledgeBaseName))
		infoBuffer.WriteString("请基于知识库查询结果来回答用户的问题，确保答案准确且有据可依。\n")
		infoBuffer.WriteString("在回答时，请明确引用知识库中的相关信息。\n")
		return infoBuffer.String(), nil
	}
}

func AIToolContextProvider(aitoolName string, userPrompt ...string) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		// 构建基本信息（即使出错也要包含）
		baseInfo := fmt.Sprintf("User Prompt: %s\n============== AITool Info ==============\nName: %s\n", strings.Join(userPrompt, " "), aitoolName)

		aitool, err := config.GetAiToolManager().GetToolByName(aitoolName)
		if err != nil {
			return baseInfo + fmt.Sprintf("[Error: failed to get aitool: %v]", err), utils.Errorf("failed to get aitool %s: %w", aitoolName, err)
		}
		var infoBuffer bytes.Buffer
		infoBuffer.WriteString(baseInfo)
		infoBuffer.WriteString(fmt.Sprintf("Description: %s\n", aitool.Description))
		infoBuffer.WriteString(fmt.Sprintf("Schema: %s\n", aitool.ToJSONSchemaString()))
		infoBuffer.WriteString("\n============== Important Instructions ==============\n")
		infoBuffer.WriteString("【重要提示】用户已指定使用此工具来完成任务。\n")
		infoBuffer.WriteString(fmt.Sprintf("请优先调用工具 '%s' 来解决用户的问题。\n", aitool.Name))
		infoBuffer.WriteString("在执行任务时，请根据上述工具的Schema正确传入参数。\n")
		infoBuffer.WriteString("如果此工具无法完全满足需求，可以结合其他工具辅助完成，但应以此工具为主。\n")
		return infoBuffer.String(), nil
	}
}

func AIForgeContextProvider(aiforgeName string, userPrompt ...string) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		// 构建基本信息（即使出错也要包含）
		baseInfo := fmt.Sprintf("User Prompt: %s\n============== AIForge Info ==============\nName: %s\n", strings.Join(userPrompt, " "), aiforgeName)

		aiforge, err := yakit.GetAIForgeByName(consts.GetGormProfileDatabase(), aiforgeName)
		if err != nil {
			return baseInfo + fmt.Sprintf("[Error: failed to get aiforge: %v]", err), utils.Errorf("failed to get aiforge %s: %w", aiforgeName, err)
		}
		var infoBuffer bytes.Buffer
		infoBuffer.WriteString(baseInfo)
		infoBuffer.WriteString(fmt.Sprintf("Description: %s\n", aiforge.Description))
		infoBuffer.WriteString(fmt.Sprintf("Params: %s\n", aiforge.Params))
		infoBuffer.WriteString("\n============== Important Instructions ==============\n")
		infoBuffer.WriteString("【重要提示】用户已指定使用此AI蓝图(Forge)来完成任务。\n")
		infoBuffer.WriteString(fmt.Sprintf("请优先调用AI蓝图 '%s' 来解决用户的问题。\n", aiforge.ForgeName))
		infoBuffer.WriteString("此蓝图专门设计用于处理特定类型的任务，能够提供更专业和高效的解决方案。\n")
		infoBuffer.WriteString("在执行时，请根据上述参数Schema正确配置蓝图参数，确保任务顺利完成。\n")
		infoBuffer.WriteString("如果蓝图执行过程中遇到问题，请及时向用户反馈并寻求进一步指导。\n")
		return infoBuffer.String(), nil
	}
}

// AISkillContextProvider returns a ContextProvider that displays skill metadata.
// It loads skill information by name and presents it to the AI for reference.
func AISkillContextProvider(skillName string, userPrompt ...string) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		baseInfo := fmt.Sprintf("User Prompt: %s\n============== AI Skill Info ==============\nName: %s\n", strings.Join(userPrompt, " "), skillName)

		var infoBuffer bytes.Buffer
		infoBuffer.WriteString(baseInfo)
		infoBuffer.WriteString("\n============== Important Instructions ==============\n")
		infoBuffer.WriteString(fmt.Sprintf("The user has specified to use AI Skill '%s'.\n", skillName))
		infoBuffer.WriteString("Use the 'loading_skills' action to load this skill into the context window.\n")
		infoBuffer.WriteString("Once loaded, the skill's SKILL.md content and file tree will be available for reference.\n")
		return infoBuffer.String(), nil
	}
}

func NewContextProvider(typ string, key string, value string, userPrompt ...string) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, providerKey string) (string, error) {
		// 构建基本信息（即使出错也要包含）
		baseInfo := fmt.Sprintf("User Prompt: %s\nType: %s\nKey: %s\nValue: %s\n", strings.Join(userPrompt, " "), typ, key, value)

		switch typ {
		case CONTEXT_PROVIDER_TYPE_FILE:
			switch key {
			case CONTEXT_PROVIDER_KEY_FILE_PATH:
				return FileContextProvider(value, userPrompt...)(config, emitter, providerKey)
			case CONTEXT_PROVIDER_KEY_FILE_CONTENT:
				// TODO: 将文件存到 AI 工作目录
				// 先暂时存到临时文件
				tempFile := consts.TempAIFileFast("file-*.txt", value)
				return FileContextProvider(tempFile, userPrompt...)(config, emitter, providerKey)
			default:
				return baseInfo + fmt.Sprintf("[Error: unknown file context provider key: %s]", key), utils.Errorf("unknown file context provider key: %s (type: %s, value: %s)", key, typ, value)
			}
		case CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE:
			switch key {
			case CONTEXT_PROVIDER_KEY_NAME:
				return KnowledgeBaseContextProvider(value, userPrompt...)(config, emitter, providerKey)
			case CONTEXT_PROVIDER_KEY_SYSTEM_FLAG:
				return KnowledgeBaseSystemFlagContextProvider(value, userPrompt...)(config, emitter, providerKey)
			default:
				return baseInfo + fmt.Sprintf("[Error: unknown knowledge base context provider key: %s]", key), utils.Errorf("unknown knowledge base context provider key: %s (type: %s, value: %s)", key, typ, value)
			}
		case CONTEXT_PROVIDER_TYPE_AITOOL:
			switch key {
			case CONTEXT_PROVIDER_KEY_NAME:
				return AIToolContextProvider(value, userPrompt...)(config, emitter, providerKey)
			default:
				return baseInfo + fmt.Sprintf("[Error: unknown aitool context provider key: %s]", key), utils.Errorf("unknown aitool context provider key: %s (type: %s, value: %s)", key, typ, value)
			}
		case CONTEXT_PROVIDER_TYPE_AIFORGE:
			switch key {
			case CONTEXT_PROVIDER_KEY_NAME:
				return AIForgeContextProvider(value, userPrompt...)(config, emitter, providerKey)
			default:
				return baseInfo + fmt.Sprintf("[Error: unknown aiforge context provider key: %s]", key), utils.Errorf("unknown aiforge context provider key: %s (type: %s, value: %s)", key, typ, value)
			}
		case CONTEXT_PROVIDER_TYPE_AISKILL:
			switch key {
			case CONTEXT_PROVIDER_KEY_NAME:
				return AISkillContextProvider(value, userPrompt...)(config, emitter, providerKey)
			default:
				return baseInfo + fmt.Sprintf("[Error: unknown aiskill context provider key: %s]", key), utils.Errorf("unknown aiskill context provider key: %s (type: %s, value: %s)", key, typ, value)
			}
		}
		return baseInfo + fmt.Sprintf("[Error: unknown context provider type: %s]", typ), utils.Errorf("unknown context provider type: %s (key: %s, value: %s)", typ, key, value)
	}
}

func KnowledgeBaseSystemFlagContextProvider(flag string, userPrompt ...string) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		baseInfo := fmt.Sprintf("User Prompt: %s\n============== Knowledge Base Info ==============\nSystemFlag: %s\n", strings.Join(userPrompt, " "), flag)

		switch flag {
		case CONTEXT_PROVIDER_VALUE_ALL_KNOWLEDGE_BASE:
			db := consts.GetGormProfileDatabase()
			var knowledgeBases []schema.KnowledgeBaseInfo
			if err := db.Model(&schema.KnowledgeBaseInfo{}).Find(&knowledgeBases).Error; err != nil {
				return baseInfo + fmt.Sprintf("[Error: failed to list knowledge bases: %v]", err), utils.Errorf("failed to list knowledge bases: %w", err)
			}
			if len(knowledgeBases) == 0 {
				return baseInfo + "No knowledge base available", nil
			}

			sort.Slice(knowledgeBases, func(i, j int) bool {
				return knowledgeBases[i].KnowledgeBaseName < knowledgeBases[j].KnowledgeBaseName
			})

			var detailBuilder strings.Builder
			detailBuilder.WriteString(baseInfo)
			detailBuilder.WriteString(fmt.Sprintf("Total Knowledge Bases: %d\n\n", len(knowledgeBases)))
			for idx, kb := range knowledgeBases {
				detailBuilder.WriteString(fmt.Sprintf("%d) Name: %s\n", idx+1, kb.KnowledgeBaseName))
				if kb.KnowledgeBaseDescription != "" {
					detailBuilder.WriteString(fmt.Sprintf("   Description: %s\n", kb.KnowledgeBaseDescription))
				}
				if kb.KnowledgeBaseType != "" {
					detailBuilder.WriteString(fmt.Sprintf("   Type: %s\n", kb.KnowledgeBaseType))
				}
				if kb.Tags != "" {
					detailBuilder.WriteString(fmt.Sprintf("   Tags: %s\n", kb.Tags))
				}
				detailBuilder.WriteString("\n")
			}

			content := detailBuilder.String()
			if len(content) > maxInlineKnowledgeBaseBytes {
				filePath := consts.TempAIFileFast("knowledge-bases-*.txt", content)
				if emitter != nil && filePath != "" {
					emitter.EmitPinFilename(filePath)
				}

				var previewBuilder strings.Builder
				previewBuilder.WriteString(baseInfo)
				previewBuilder.WriteString(fmt.Sprintf("All knowledge base info is large (%d bytes). Saved to file: %s\n", len(content), filePath))
				previewBuilder.WriteString("Knowledge Base Names:\n")
				for _, kb := range knowledgeBases {
					previewBuilder.WriteString(fmt.Sprintf("- %s\n", kb.KnowledgeBaseName))
				}
				return previewBuilder.String(), nil
			}

			return content, nil
		default:
			return baseInfo + fmt.Sprintf("[Error: unknown system flag: %s]", flag), utils.Errorf("unknown knowledge base system flag: %s", flag)
		}
	}
}

// ArtifactsContextMaxBytes is the maximum size (in bytes) for the artifacts context output.
// This limits the artifacts summary injected into every prompt to 8KB.
const ArtifactsContextMaxBytes = 8 * 1024

// artifactFileEntry holds metadata for a single file in the artifacts directory.
type artifactFileEntry struct {
	RelPath string
	Size    int64
	ModTime time.Time
}

// ArtifactsContextProvider scans the session's working directory (artifacts dir) and generates
// a structured summary of all task output files. This provider is registered once and executed
// on every prompt build, ensuring all subsequent AI turns can see the artifacts filesystem.
//
// The output is limited to ArtifactsContextMaxBytes (8KB) using utils.ShrinkTextBlock.
func ArtifactsContextProvider(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
	workDir := config.GetOrCreateWorkDir()
	if workDir == "" {
		return "", nil
	}

	// Check if the directory exists
	info, err := os.Stat(workDir)
	if err != nil || !info.IsDir() {
		return "", nil
	}

	var entries []artifactFileEntry
	walkErr := filepath.Walk(workDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if fi.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(workDir, path)
		if err != nil {
			relPath = path
		}
		entries = append(entries, artifactFileEntry{
			RelPath: relPath,
			Size:    fi.Size(),
			ModTime: fi.ModTime(),
		})
		return nil
	})
	if walkErr != nil {
		log.Warnf("artifacts context provider: walk error: %v", walkErr)
	}

	if len(entries) == 0 {
		return "", nil
	}

	// Sort entries by modification time (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ModTime.After(entries[j].ModTime)
	})

	// Group entries by top-level task directory
	taskGroups := omap.NewOrderedMap(make(map[string][]artifactFileEntry))
	var rootFiles []artifactFileEntry

	for _, e := range entries {
		parts := strings.SplitN(e.RelPath, string(filepath.Separator), 2)
		if len(parts) == 1 {
			// File directly in workDir (not in a task subfolder)
			rootFiles = append(rootFiles, e)
		} else {
			taskDir := parts[0]
			existing, _ := taskGroups.Get(taskDir)
			taskGroups.Set(taskDir, append(existing, e))
		}
	}

	var sb strings.Builder
	sb.WriteString("# Session Artifacts\n")
	sb.WriteString(fmt.Sprintf("artifacts_dir: %s\n", workDir))
	sb.WriteString(fmt.Sprintf("total_files: %d\n\n", len(entries)))

	// Write task directory groups
	taskGroups.ForEach(func(taskDir string, files []artifactFileEntry) bool {
		// Find the latest modification time for the group
		var latestMod time.Time
		for _, f := range files {
			if f.ModTime.After(latestMod) {
				latestMod = f.ModTime
			}
		}
		sb.WriteString(fmt.Sprintf("## %s (modified: %s)\n",
			taskDir,
			latestMod.Format("2006-01-02 15:04:05"),
		))
		for _, f := range files {
			// Show only the part after the task directory
			innerPath := strings.TrimPrefix(f.RelPath, taskDir+string(filepath.Separator))
			sb.WriteString(fmt.Sprintf("- %s (%s, %s)\n",
				innerPath,
				formatFileSize(f.Size),
				f.ModTime.Format("15:04:05"),
			))
		}
		sb.WriteString("\n")
		return true
	})

	// Write root-level files (if any)
	if len(rootFiles) > 0 {
		sb.WriteString("## [root files]\n")
		for _, f := range rootFiles {
			sb.WriteString(fmt.Sprintf("- %s (%s, %s)\n",
				f.RelPath,
				formatFileSize(f.Size),
				f.ModTime.Format("15:04:05"),
			))
		}
		sb.WriteString("\n")
	}

	result := sb.String()
	if len(result) > ArtifactsContextMaxBytes {
		result = utils.ShrinkTextBlock(result, ArtifactsContextMaxBytes)
	}
	return result, nil
}

// formatFileSize formats a file size in human-readable form.
func formatFileSize(size int64) string {
	switch {
	case size >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
	case size >= 1024:
		return fmt.Sprintf("%.1fKB", float64(size)/1024)
	default:
		return fmt.Sprintf("%dB", size)
	}
}

type ContextProviderManager struct {
	maxBytes int
	m        sync.RWMutex
	callback *omap.OrderedMap[string, ContextProvider]
}

const maxInlineKnowledgeBaseBytes = 2 * 1024 // 8KB

func NewContextProviderManager() *ContextProviderManager {
	return &ContextProviderManager{
		maxBytes: 10 * 1024, // 10KB
		callback: omap.NewOrderedMap(make(map[string]ContextProvider)),
	}
}

func (r *ContextProviderManager) RegisterTracedContent(name string, cb ContextProvider) {
	var m = new(sync.Mutex)
	var firstCall = utils.NewOnce()
	var lastErr error
	var lastContent string
	var buf bytes.Buffer

	update := func(content string, newErr error) string {
		m.Lock()
		defer m.Unlock()
		var result string
		firstCall.DoOr(func() {
			lastContent = content
			lastErr = newErr
			buf.Reset()
		}, func() {
			var diffResult string
			var err error
			if lastContent != "" && content != "" {
				diffResult, err = yakdiff.DiffToString(lastContent, content)
				if err != nil {
					log.Warnf("diff to string failed: %v", err)
				}
			} else if lastContent == "" {
				diffResult = "last-content is empty, new content added"
			}

			if newErr == nil && lastErr != nil {
				diffResult += fmt.Sprintf("\n[Error resolved: %v]", lastErr)
			} else if newErr != nil && lastErr == nil {
				diffResult += fmt.Sprintf("\n[New error occurred: %v]", newErr)
			} else if newErr != nil && lastErr != nil && newErr.Error() != lastErr.Error() {
				diffResult += fmt.Sprintf("\n[Error changed from: %v to: %v]", lastErr, newErr)
			}

			diff, err := utils.RenderTemplate(`<|CHANGES_DIFF_{{ .nonce }}|>
{{ .diff }}
<|CHANGES_DIFF_{{ .nonce }}|>`, map[string]any{
				"diff":  diffResult,
				"nonce": utils.RandStringBytes(4),
			})
			if err != nil {
				log.Warnf("render template failed: %v", err)
			} else {
				buf.WriteString(diff)
				buf.WriteString("\n")
			}
			result = buf.String()
			lastContent = content
			lastErr = newErr
			buf.Reset()
		})
		return result
	}

	wrapper := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		result, err := cb(config, emitter, key)
		extra := update(result, err)
		if err != nil {
			if extra == "" {
				return result, err
			}
			return result + "\n\n" + extra + "", err
		}
		log.Infof("ContextProvider %s result: %s", name, utils.ShrinkString(result, 200))
		if extra == "" {
			return result, nil
		}
		return result + "\n\n" + extra, nil
	}
	r.Register(name, wrapper)
}

func (r *ContextProviderManager) Register(name string, cb ContextProvider) {
	r.m.Lock()
	defer r.m.Unlock()
	if r.callback.Have(name) {
		log.Warnf("context provider %s already registered, ignore, if you want to use new callback, unregister first", name)
		return
	}
	r.callback.Set(name, func(config AICallerConfigIf, emitter *Emitter, key string) (_ string, finalErr error) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("context provider %s panic: %v", name, err)
				utils.PrintCurrentGoroutineRuntimeStack()
				finalErr = utils.Errorf("context provider %s panic: %v", name, err)
			}
		}()
		return cb(config, emitter, key)
	})
}

func (r *ContextProviderManager) Unregister(name string) {
	r.m.Lock()
	defer r.m.Unlock()
	r.callback.Delete(name)
}

func (r *ContextProviderManager) Execute(config AICallerConfigIf, emitter *Emitter) string {
	r.m.RLock()
	defer r.m.RUnlock()

	if r.callback.Len() == 0 {
		return ""
	}

	var buf bytes.Buffer
	r.callback.ForEach(func(name string, cb ContextProvider) bool {
		result, err := cb(config, emitter, name)
		if err != nil {
			result = `[Error getting context: ` + err.Error() + `]`
		}
		flag := utils.RandStringBytes(4)
		buf.WriteString(fmt.Sprintf("<|AUTO_PROVIDE_CTX_[%v]_START key=%v|>\n", flag, name))
		buf.WriteString(result)
		buf.WriteString(fmt.Sprintf("\n<|AUTO_PROVIDE_CTX_[%v]_END|>", flag))
		return true
	})

	result := buf.String()
	if len(result) > r.maxBytes {
		shrinkSize := int(float64(r.maxBytes) * 0.8)
		result = utils.ShrinkString(result, shrinkSize)
		log.Warnf("context provider result exceeded maxBytes (%d), shrunk to %d characters", r.maxBytes, shrinkSize)
	}

	return result
}
