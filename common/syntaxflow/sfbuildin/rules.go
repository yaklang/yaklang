//go:build !irify_exclude

package sfbuildin

import (
	"embed"
	"errors"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/resources_monitor"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var ruleFSWithHash resources_monitor.ResourceMonitor

func GetRuleFS() *embed.FS {
	return nil
}

func SyncRuleFromFileSystem(fsInstance filesys_interface.FileSystem, buildin bool, notifies ...func(process float64, ruleName string)) (err error) {
	return SyncRuleFromFileSystemToDB(consts.GetGormProfileDatabase(), fsInstance, buildin, notifies...)
}

func SyncRuleFromFileSystemToDB(db *gorm.DB, fsInstance filesys_interface.FileSystem, buildin bool, notifies ...func(process float64, ruleName string)) (err error) {
	if db == nil {
		return utils.Errorf("profile db is nil")
	}
	var notify func(process float64, ruleName string)
	if len(notifies) != 0 {
		notify = notifies[0]
		defer notify(1, "同步SyntaxFlow规则包成功！")
	}
	return syncAllEmbedPackagesToDB(db, fsInstance, buildin, notify)
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

func ForceSyncEmbedRuleToDB(db *gorm.DB, notifies ...func(process float64, ruleName string)) (err error) {
	if db == nil {
		return utils.Errorf("profile db is nil")
	}
	log.Infof("start sync embed rule to custom db")
	var notify func(process float64, ruleName string)
	if len(notifies) > 0 {
		notify = notifies[0]
	}
	InitEmbedFSWithNotify(notify)
	return utils.Wrapf(SyncRuleFromFileSystemToDB(db, ruleFSWithHash, true, notifies...), "init builtin rules to custom db error")
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
