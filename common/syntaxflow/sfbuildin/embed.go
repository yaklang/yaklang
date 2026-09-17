//go:build !gzip_embed && !irify_exclude

package sfbuildin

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/resources_monitor"
)

//go:embed buildin/***
var ruleFS embed.FS

var (
	checkOnce sync.Once
	checkErr  error
)

func InitEmbedFS() {
	ruleFSWithHash = resources_monitor.NewStandardResourceMonitor(ruleFS, ".sf")
}

func init() {
	InitEmbedFS()
}

// InitEmbedFSWithNotify 带进度通知的初始化（非 gzip 版本不需要，但保持接口一致）
// 在这里自动执行重复标题检查
func InitEmbedFSWithNotify(notify func(process float64, ruleName string)) {
	// 非 gzip 版本已经在 init() 中初始化完成
	// 首次调用时自动执行重复标题检查
	checkOnce.Do(func() {
		fsInstance := ruleFSWithHash
		checkErr = checkDuplicateTitles(fsInstance)
		if checkErr != nil {
			log.Errorf("check duplicate titles failed: %v", checkErr)
		}
	})
}

// ruleTitleMetadata is the subset of a rule's desc() block that the
// duplicate-title check needs. Scanning for these keys avoids compiling the
// heredoc bodies (desc/solution/reference), which dominate a full parse.
type ruleTitleMetadata struct {
	Title   string
	TitleZh string
}

// extractRuleTitleMetadata pulls title and title_zh out of a rule's desc()
// block without compiling the rule. The desc() block is a flat list of
// `key: value` items, so a line scan is sufficient for the keys we need and
// leaves the heredoc bodies (desc/solution/reference) untouched.
//
// It mirrors the compiler's desc() semantics for these two keys: `lib:` is
// applied when it appears, overwriting `title:` (see
// sfvm.VisitDescriptionStatement), because for a library rule the lib name is
// the rule title.
func extractRuleTitleMetadata(content string) ruleTitleMetadata {
	var meta ruleTitleMetadata
	for _, item := range ruleDescItems(content) {
		switch item.key {
		case "title":
			if item.value != "" {
				meta.Title = item.value
			}
		case "title_zh":
			if item.value != "" {
				meta.TitleZh = item.value
			}
		case "lib":
			if item.value != "" {
				meta.Title = item.value
			}
		}
	}
	return meta
}

// ruleDescItem is one `key: value` entry of a desc() block.
type ruleDescItem struct {
	key   string
	value string
}

// ruleDescItems scans the desc() block and returns its `key: value` entries.
// Heredoc bodies are skipped by tag rather than materialized: rules routinely
// put desc/solution/reference bodies before title_zh, and those bodies are what
// make a full compile expensive.
func ruleDescItems(content string) []ruleDescItem {
	var items []ruleDescItem
	inDesc := false
	heredocTag := ""
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if heredocTag != "" {
			if line == heredocTag {
				heredocTag = ""
			}
			continue
		}
		if !inDesc {
			if line == "desc(" || strings.HasPrefix(line, "desc(") {
				inDesc = true
			}
			continue
		}
		if line == ")" {
			break
		}
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if tag, found := strings.CutPrefix(value, "<<<"); found {
			// Body follows until the tag repeats on its own line.
			heredocTag = strings.TrimSpace(tag)
			continue
		}
		items = append(items, ruleDescItem{
			key:   strings.ToLower(strings.TrimSpace(key)),
			value: trimRuleDescValue(value),
		})
	}
	return items
}

// trimRuleDescValue strips whitespace, a trailing comma, and surrounding
// quotes from a desc() item value.
func trimRuleDescValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ",")
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return strings.TrimSpace(value)
}

// checkDuplicateTitles 检查规则中的 title 和 title_zh 是否重复（仅非 gzip 版本）
func checkDuplicateTitles(fsInstance filesys_interface.FileSystem) error {
	// 用于检查 title 和 title_zh 重复
	titleMap := make(map[string][]string)   // title -> []filePath
	titleZhMap := make(map[string][]string) // title_zh -> []filePath

	// 第一遍：收集所有规则的 title 和 title_zh
	err := filesys.Recursive(".", filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		_, name := fsInstance.PathSplit(s)
		if !strings.HasSuffix(name, ".sf") {
			return nil
		}
		raw, err := fsInstance.ReadFile(s)
		if err != nil {
			return utils.Wrapf(err, "read file[%s] error", s)
		}

		// Read title / title_zh out of the desc() block directly. A full
		// compile here costs a heredoc parse per rule file on every process
		// start; the duplicate check only needs two metadata keys.
		meta := extractRuleTitleMetadata(string(raw))

		// 收集 title 重复
		if meta.Title != "" {
			titleMap[meta.Title] = append(titleMap[meta.Title], s)
		}
		// 收集 title_zh 重复
		if meta.TitleZh != "" {
			titleZhMap[meta.TitleZh] = append(titleZhMap[meta.TitleZh], s)
		}

		return nil
	}))

	if err != nil {
		return err
	}

	// 检查 title 重复
	var duplicateErrors []string
	for title, paths := range titleMap {
		if len(paths) > 1 {
			duplicateErrors = append(duplicateErrors, fmt.Sprintf("重复的 title '%s' 出现在以下文件中:\n  %s", title, strings.Join(paths, "\n  ")))
		}
	}

	// 检查 title_zh 重复
	for titleZh, paths := range titleZhMap {
		if len(paths) > 1 {
			duplicateErrors = append(duplicateErrors, fmt.Sprintf("重复的 title_zh '%s' 出现在以下文件中:\n  %s", titleZh, strings.Join(paths, "\n  ")))
		}
	}

	// 如果有重复，返回错误
	if len(duplicateErrors) > 0 {
		errorMsg := "发现重复的 title 或 title_zh:\n" + strings.Join(duplicateErrors, "\n\n")
		log.Errorf(errorMsg)
		return utils.Errorf(errorMsg)
	}

	return nil
}
