//go:build !gzip_embed && !irify_exclude

package sfbuildin

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
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

		content := string(raw)
		// 解析规则内容获取 title 和 title_zh
		rule, err := sfdb.CheckSyntaxFlowRuleContent(content)
		if err != nil {
			// 如果解析失败，跳过重复检查，但会在后续导入时处理错误
			return nil
		}

		// 收集 title 重复
		if rule.Title != "" {
			titleMap[rule.Title] = append(titleMap[rule.Title], s)
		}
		// 收集 title_zh 重复
		if rule.TitleZh != "" {
			titleZhMap[rule.TitleZh] = append(titleZhMap[rule.TitleZh], s)
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
