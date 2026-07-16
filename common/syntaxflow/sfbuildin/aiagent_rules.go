//go:build !irify_exclude

package sfbuildin

import (
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/resources_monitor"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// SyncAIAgentEmbedRule 同步 AI Agent 规则包到数据库
func SyncAIAgentEmbedRule(notifies ...func(process float64, ruleName string)) (err error) {
	const key = consts.EmbedSfAIAgentRuleKey
	return resources_monitor.NewEmbedResourcesMonitor(key, consts.ExistedAIAgentEmbedFSHash).MonitorModifiedWithAction(func() string {
		hash, _ := AIAgentRuleHash()
		return hash
	}, func() error {
		return syncAIAgentEmbedRuleInternal(notifies...)
	})
}

// ForceSyncAIAgentEmbedRule 强制同步 AI Agent 规则包，忽略哈希检查
func ForceSyncAIAgentEmbedRule(notifies ...func(process float64, ruleName string)) (err error) {
	err = syncAIAgentEmbedRuleInternal(notifies...)
	if err == nil {
		DoneAIAgentEmbedRule()
	}
	return err
}

// ForceSyncAIAgentEmbedRuleToDB 强制同步 AI Agent 规则包到指定数据库
func ForceSyncAIAgentEmbedRuleToDB(db *gorm.DB, notifies ...func(process float64, ruleName string)) (err error) {
	if db == nil {
		return utils.Errorf("profile db is nil")
	}
	log.Infof("start sync aiagent rule to custom db")
	InitAIAgentEmbedFS()
	return utils.Wrapf(SyncRuleFromFileSystemToDB(db, GetAIAgentRuleFS(), true, notifies...), "init aiagent rules to custom db error")
}

func syncAIAgentEmbedRuleInternal(notifies ...func(process float64, ruleName string)) (err error) {
	log.Infof("start sync aiagent embed rule")
	InitAIAgentEmbedFS()
	err = SyncRuleFromFileSystem(GetAIAgentRuleFS(), true, notifies...)
	return utils.Wrapf(err, "init aiagent rules error")
}

// AIAgentRuleHash 计算 AI Agent 规则包的哈希值
func AIAgentRuleHash() (string, error) {
	hash, err := GetAIAgentRuleFS().GetHash()
	if err != nil {
		return "", utils.Wrapf(err, "calc aiagent rule hash error")
	}
	return hash, nil
}

// NeedSyncAIAgentEmbedRule 检查是否需要同步 AI Agent 规则包
func NeedSyncAIAgentEmbedRule() bool {
	return yakit.Get(consts.EmbedSfAIAgentRuleKey) != consts.ExistedAIAgentEmbedFSHash
}

// DoneAIAgentEmbedRule 标记 AI Agent 规则包同步完成
func DoneAIAgentEmbedRule() {
	log.Infof("done sync aiagent embed rule with hash: %s", consts.ExistedAIAgentEmbedFSHash)
	yakit.Set(consts.EmbedSfAIAgentRuleKey, consts.ExistedAIAgentEmbedFSHash)
}
