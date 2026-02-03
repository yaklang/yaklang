//go:build !irify_exclude

package sfbuildin

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
	"github.com/yaklang/yaklang/common/utils/resources_monitor"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var ruleFSWithHash resources_monitor.ResourceMonitor

func GetRuleFS() *embed.FS {
	return nil
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

	// 导入规则
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
	const key = consts.EmbedSfBuildInRuleKey
	return resources_monitor.NewEmbedResourcesMonitor(key, consts.ExistedSyntaxFlowEmbedFSHash).MonitorModifiedWithAction(func() string {
		hash, _ := SyntaxFlowRuleHash()
		return hash
	}, func() error {
		return syncEmbedRuleInternal(notifies...)
	})
}

// ForceSyncEmbedRule 强制同步嵌入规则，忽略哈希检查
func ForceSyncEmbedRule(notifies ...func(process float64, ruleName string)) (err error) {
	err = syncEmbedRuleInternal(notifies...)
	if err == nil {
		DoneEmbedRule()
	}
	return err
}

// syncEmbedRuleInternal 内部同步实现（不处理 hash 更新，由调用者决定）
func syncEmbedRuleInternal(notifies ...func(process float64, ruleName string)) (err error) {
	log.Infof("start sync embed rule")
	// sfdb.DeleteBuildInRule()

	var notify func(process float64, ruleName string)
	if len(notifies) > 0 {
		notify = notifies[0]
	}

	// 对于 gzip 版本，设置进度通知回调，以便在解压过程中显示进度
	// 注意：这需要在 GetRuleFileSystem() 之前调用
	InitEmbedFSWithNotify(notify)

	err = SyncRuleFromFileSystem(ruleFSWithHash, true, notifies...)

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
