package aicommon

import (
	"bytes"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

const (
	CONTEXT_PROVIDER_TYPE_FILE           = "file"
	CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE = "knowledge_base"
	CONTEXT_PROVIDER_TYPE_AITOOL         = "aitool"
	CONTEXT_PROVIDER_TYPE_AIFORGE        = "aiforge"

	CONTEXT_PROVIDER_KEY_FILE_PATH      = "file_path"
	CONTEXT_PROVIDER_KEY_FILE_CONTENT   = "file_content"
	CONTEXT_PROVIDER_KEY_DIRECTORY_PATH = "directory_path"
	CONTEXT_PROVIDER_KEY_NAME           = "name"

	CONTEXT_PROVIDER_KEY_SYSTEM_FLAG = "system_flag"

	CONTEXT_PROVIDER_VALUE_ALL_KNOWLEDGE_BASE         = "all_knowledge_base"
	CONTEXT_PROVIDER_VALUE_AUTO_SELECT_KNOWLEDGE_BASE = "auto_select_knowledge_base"

	USER_FREE_INPUT_UUID = "user_free_input_uuid"
	USER_INPUT_SOURCE    = "user_input_source"

	USER_INPUT_SOURCE_KEY       = "source"
	USER_INPUT_SOURCE_SCHEDULE  = "schedule"
	USER_INPUT_SCHEDULE_CONTEXT = "scheduled_task_context"
	USER_INPUT_SCHEDULE_UUID    = "uuid"
	USER_INPUT_SCHEDULE_NAME    = "name"
	USER_INPUT_SCHEDULED_AT     = "scheduled_at"
	USER_INPUT_SCHEDULE_TRIGGER = "trigger"
)

type ContextProvider func(config AICallerConfigIf, emitter *Emitter, key string) (string, error)

// Only the main-loop attachment view omits the echoed query. Helper prompts
// receive the original config and retain their existing attachment context.
type mainLoopContextProviderConfig struct{ AICallerConfigIf }

func attachmentUserPrompt(config AICallerConfigIf, userPrompt []string) string {
	if _, mainLoop := config.(*mainLoopContextProviderConfig); mainLoop {
		return ""
	}
	return fmt.Sprintf("User Prompt: %s\n", strings.Join(userPrompt, " "))
}

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

// IsImageContextAttachmentPath reports whether filePath should be treated as an image
// for prompt attachment (by extension or image/* MIME). Used so callers can route
// images to vision pipelines instead of FileContextProvider (text-only).
func IsImageContextAttachmentPath(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	imageExts := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
		".bmp": true, ".tif": true, ".tiff": true, ".heic": true, ".heif": true, ".avif": true,
	}
	if imageExts[ext] {
		return true
	}
	mimeType := mime.TypeByExtension(ext)
	if mimeType != "" {
		if idx := strings.Index(mimeType, ";"); idx != -1 {
			mimeType = strings.TrimSpace(mimeType[:idx])
		}
		return strings.HasPrefix(strings.ToLower(mimeType), "image/")
	}
	return false
}

// MaxFileContentSize 文件内容最大读取大小（1KB）
const MaxFileContentSize = 1024

func FileContextProvider(filePath string, userPrompt ...string) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		// 构建基本信息（即使出错也要包含）
		baseInfo := attachmentUserPrompt(config, userPrompt) + fmt.Sprintf("File: %s\n", filePath)

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

// ArtifactsContextMaxTokens is the maximum size (in tokens) for the artifacts context output.
const ArtifactsContextMaxTokens = 8 * 1024

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
	maxTokens int
	m         sync.RWMutex
	callback  *omap.OrderedMap[string, ContextProvider]
}

func NewContextProviderManager() *ContextProviderManager {
	return &ContextProviderManager{
		maxTokens: 48 * 1024, // 48k tokens
		callback:  omap.NewOrderedMap(make(map[string]ContextProvider)),
	}
}

func (r *ContextProviderManager) RegisterTracedContent(name string, cb ContextProvider) {
	update := newContextProviderTrace()
	mainLoopUpdate := newContextProviderTrace()
	wrapper := func(config AICallerConfigIf, emitter *Emitter, key string) (string, error) {
		result, err := cb(config, emitter, key)
		trace := update
		if _, mainLoop := config.(*mainLoopContextProviderConfig); mainLoop {
			// Helper-only query headers must never return via a change diff.
			trace = mainLoopUpdate
		}
		extra := trace(result, err)
		if err != nil {
			if extra == "" {
				return result, err
			}
			return result + "\n\n" + extra, err
		}
		log.Infof("ContextProvider %s result: %s", name, utils.ShrinkString(result, 200))
		if extra == "" {
			return result, nil
		}
		return result + "\n\n" + extra, nil
	}
	r.Register(name, wrapper)
}

func newContextProviderTrace() func(string, error) string {
	var m = new(sync.Mutex)
	var firstCall = utils.NewOnce()
	var lastErr error
	var lastContent string
	var buf bytes.Buffer

	return func(content string, newErr error) string {
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

			if strings.TrimSpace(diffResult) == "" {
				lastContent = content
				lastErr = newErr
				buf.Reset()
				return
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
}

func (r *ContextProviderManager) Register(name string, cb ContextProvider) {
	r.m.Lock()
	defer r.m.Unlock()
	if r.callback.Have(name) {
		log.Warnf("context provider %s already registered, ignore, if you want to use new callback, unregister first", name)
		return
	}
	r.callback.Set(name, wrapContextProvider(name, cb))
}

func wrapContextProvider(name string, cb ContextProvider) ContextProvider {
	return func(config AICallerConfigIf, emitter *Emitter, key string) (_ string, finalErr error) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("context provider %s panic: %v", name, err)
				utils.PrintCurrentGoroutineRuntimeStack()
				finalErr = utils.Errorf("context provider %s panic: %v", name, err)
			}
		}()
		return cb(config, emitter, key)
	}
}

func (r *ContextProviderManager) Unregister(name string) {
	r.m.Lock()
	defer r.m.Unlock()
	r.callback.Delete(name)
}

func (r *ContextProviderManager) Execute(config AICallerConfigIf, emitter *Emitter) string {
	return r.executeWithTagStrategy(config, emitter, func(name string) string {
		return utils.RandStringBytes(4)
	})
}

func (r *ContextProviderManager) ExecuteStable(config AICallerConfigIf, emitter *Emitter) string {
	return r.executeWithTagStrategy(config, emitter, func(name string) string {
		return stableContextProviderTag(name)
	})
}

func (r *ContextProviderManager) ExecuteWithNonce(config AICallerConfigIf, emitter *Emitter, nonce string) string {
	return r.executeWithTagStrategy(config, emitter, func(name string) string {
		return contextProviderTagWithNonce(nonce, name)
	})
}

// ExecuteMainLoopWithNonce retains attachment contents and coordinates while
// leaving task queries exclusively in Timeline Open / promoted user history.
func (r *ContextProviderManager) ExecuteMainLoopWithNonce(config AICallerConfigIf, emitter *Emitter, nonce string) string {
	return r.ExecuteWithNonce(&mainLoopContextProviderConfig{config}, emitter, nonce)
}

func (r *ContextProviderManager) executeWithTagStrategy(
	config AICallerConfigIf,
	emitter *Emitter,
	tagStrategy func(name string) string,
) string {
	r.m.RLock()
	defer r.m.RUnlock()

	if r.callback.Len() == 0 {
		return ""
	}

	var buf bytes.Buffer
	render := func(name string, cb ContextProvider) bool {
		result, err := cb(config, emitter, name)
		if err != nil {
			result = `[Error getting context: ` + err.Error() + `]`
		}
		flag := ""
		if tagStrategy != nil {
			flag = tagStrategy(name)
		}
		if strings.TrimSpace(flag) == "" {
			flag = "ctx"
		}
		result = normalizeContextProviderDynamicTags(result, flag)
		buf.WriteString(fmt.Sprintf("<|AUTO_PROVIDE_CTX_[%v]_START key=%v|>\n", flag, name))
		buf.WriteString(result)
		buf.WriteString(fmt.Sprintf("\n<|AUTO_PROVIDE_CTX_[%v]_END|>", flag))
		return true
	}
	r.callback.ForEach(render)

	result := buf.String()
	if TokenCountExceeds(result, r.maxTokens) {
		shrinkSize := int(float64(r.maxTokens) * 0.8)
		result = ShrinkTextBlockByTokens(result, shrinkSize)
		log.Warnf("context provider result exceeded maxTokens (%d), shrunk to %d tokens", r.maxTokens, shrinkSize)
	}

	return result
}

func stableContextProviderTag(name string) string {
	result := sanitizeContextProviderTag(name, true)
	if result == "" {
		return "ctx"
	}
	return result
}

func contextProviderTagWithNonce(nonce string, name string) string {
	nameTag := stableContextProviderTag(name)
	nonceTag := sanitizeContextProviderTag(nonce, false)
	if nonceTag == "" {
		return nameTag
	}
	return nonceTag + "_" + nameTag
}

func sanitizeContextProviderTag(name string, lower bool) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if lower {
		name = strings.ToLower(name)
	}
	var buf strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			buf.WriteRune(r)
		default:
			buf.WriteByte('_')
		}
	}
	return strings.Trim(buf.String(), "_")
}

var changesDiffTagRegexp = regexp.MustCompile(`<\|CHANGES_DIFF_[^\s\|\n>]+\|>`)

func normalizeContextProviderDynamicTags(result string, flag string) string {
	if strings.TrimSpace(result) == "" || strings.TrimSpace(flag) == "" {
		return result
	}
	return changesDiffTagRegexp.ReplaceAllString(result, "<|CHANGES_DIFF_"+flag+"|>")
}
