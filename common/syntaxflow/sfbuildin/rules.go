//go:build !irify_exclude

package sfbuildin

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

var ruleFSWithHash interface {
	GetHash() (string, error)
}

func GetRuleFS() *embed.FS {
	return getRuleFS().(*embed.FS)
}

func SyncRuleFromFileSystem(fsInstance filesys_interface.FileSystem, buildin bool, notifies ...func(process float64, ruleName string)) (err error) {
	var notify func(process float64, ruleName string)
	if len(notifies) != 0 {
		notify = notifies[0]
		defer notify(1, "同步SyntaxFlow规则成功！")
	}

	var (
		handledCount float64
		totalCount   float64
	)
	filesys.Recursive(".", filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		if strings.HasSuffix(info.Name(), ".sf") {
			totalCount++
		}
		return nil
	}))

	// 用于检查 title 和 title_zh 重复
	titleMap := make(map[string][]string)   // title -> []filePath
	titleZhMap := make(map[string][]string) // title_zh -> []filePath

	// 第一遍：收集所有规则的 title 和 title_zh
	err = filesys.Recursive(".", filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
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

	// 第二遍：实际导入规则
	err = filesys.Recursive(".", filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		dirName, name := fsInstance.PathSplit(s)
		if !strings.HasSuffix(name, ".sf") {
			return nil
		}
		raw, err := fsInstance.ReadFile(s)
		if err != nil {
			return utils.Wrapf(err, "read file[%s] error", s)
		}

		var tags []string
		for _, block := range utils.PrettifyListFromStringSplitEx(dirName, "/", "\\", ",", "|") {
			block = strings.ToLower(block)
			if block == "buildin" {
				continue
			}
			if strings.HasPrefix(block, "cwe-") {
				result, err := regexp_utils.NewYakRegexpUtils(`(cwe-\d+)(-(.*))?`).FindStringSubmatch(block)
				if err != nil {
					continue
				}
				tags = append(tags, strings.ToUpper(result[1]))
				tags = append(tags, result[3])
				continue
			} else if strings.HasPrefix(block, "cve-") {
				result, err := regexp_utils.NewYakRegexpUtils(`(cve-\d+-\d+)([_-\.](.*))?`).FindStringSubmatch(block)
				if err != nil {
					continue
				}
				tags = append(tags, strings.ToUpper(result[1]))
				tags = append(tags, result[3])
				continue
			}
			tags = append(tags, block)
		}
		content := string(raw)
		// import builtin rule
		_, err = sfdb.ImportRuleWithoutValid(name, content, buildin, tags...)
		if err != nil {
			log.Warnf("import rule %s error: %s", name, err)
			return err
		}
		handledCount++
		if notify != nil {
			if totalCount > 0 {
				notify(handledCount/totalCount, fmt.Sprintf("更新内置SyntaxFlow规则:%s ", info.Name()))
			} else {
				notify(1, "没有内置SyntaxFlow规则需要更新。")
			}
		}

		return nil
	}))

	return err
}

func SyncEmbedRule(notifies ...func(process float64, ruleName string)) (err error) {
	if !NeedSyncEmbedRule() {
		return nil
	}
	return syncEmbedRuleInternal(notifies...)
}

// ForceSyncEmbedRule 强制同步嵌入规则，忽略哈希检查
func ForceSyncEmbedRule(notifies ...func(process float64, ruleName string)) (err error) {
	return syncEmbedRuleInternal(notifies...)
}

// syncEmbedRuleInternal 内部同步实现
func syncEmbedRuleInternal(notifies ...func(process float64, ruleName string)) (err error) {
	defer DoneEmbedRule()
	log.Infof("start sync embed rule")
	// sfdb.DeleteBuildInRule()
	fsInstance := filesys.NewEmbedFS(ruleFS)
	err = SyncRuleFromFileSystem(fsInstance, true, notifies...)

	return utils.Wrapf(err, "init builtin rules error")
}

// SyntaxFlowRuleHash is deprecated. Use filesys.CreateEmbedFSHash(ruleFS, filesys.WithIncludeExts(".sf")) instead.
// This function is kept for backward compatibility but should not be used in new code.
func SyntaxFlowRuleHash() (string, error) {
	// Use GetHash method to calculate hash for .sf files
	hash, err := ruleFSWithHash.GetHash()
	if err != nil {
		// Check if error is due to no .sf files found
		if errors.Is(err, filesys.ErrNoFileFound) {
			return "", utils.Error("no .sf file found")
		}
		return "", err
	}
	return hash, nil
}

func NeedSyncEmbedRule() bool {
	diffHash := yakit.Get(consts.EmbedSfBuildInRuleKey) != consts.ExistedSyntaxFlowEmbedFSHash
	return diffHash
}

func DoneEmbedRule() {
	log.Infof("done sync embed rule with hash: %s", consts.ExistedSyntaxFlowEmbedFSHash)
	yakit.Set(consts.EmbedSfBuildInRuleKey, consts.ExistedSyntaxFlowEmbedFSHash)
}
