package sfdb

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
)

type RuleInfo struct {
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	Hash     string `json:"hash"`
	Version  string `json:"version"`
}

//go:embed rule_versions.json
var embedRuleVersions []byte

var embedRuleVersionMap map[string]*RuleInfo

func GetVersionFromEmbed(ruleId string) (string, error) {
	if ruleId == "" {
		return "", fmt.Errorf("ruleId is empty")
	}
	versionMap := getEmbedVersionMap()
	if ruleInfo, ok := versionMap[ruleId]; ok {
		return ruleInfo.Version, nil
	} else {
		return "", fmt.Errorf("ruleId %s not found", ruleId)
	}
}

func GetRuleInfo(ruleId string) (*RuleInfo, error) {
	if ruleId == "" {
		return nil, fmt.Errorf("ruleId is empty")
	}
	versionMap := getEmbedVersionMap()
	if ruleInfo, ok := versionMap[ruleId]; ok {
		return ruleInfo, nil
	} else {
		return nil, fmt.Errorf("ruleId %s not found", ruleId)
	}
}

func getEmbedVersionMap() map[string]*RuleInfo {
	if embedRuleVersionMap != nil {
		return embedRuleVersionMap
	}
	embedRuleVersionMap = make(map[string]*RuleInfo)
	var rules []RuleInfo
	if err := json.Unmarshal(embedRuleVersions, &rules); err == nil {
		for _, rule := range rules {
			embedRuleVersionMap[rule.RuleID] = &rule
		}
	}
	return embedRuleVersionMap
}

func EmbedRuleVersion() []RuleInfo {
	// existingRules := GetVersionMap()
	db := consts.GetGormProfileDatabase()
	ruleCh := YieldBuildInSyntaxFlowRules(db, context.Background())
	var ruleInfos []RuleInfo
	now := time.Now()

	for rule := range ruleCh {
		var version string
		// 检查是否需要更新version
		if existingRule, err := GetRuleInfo(rule.RuleId); err == nil {
			// 存在相同的RuleId，检查版本号
			// 版本号为空，从0001开始构建版本号
			if existingRule.Version == "" {
				version = generateVersion(now, "")
			} else if existingRule.Hash != rule.Hash {
				// 版本号不为空、hash不同，增加版本号
				version = generateVersion(now, existingRule.Version)
			} else {
				// 版本号不变
				version = existingRule.Version
			}
		} else {
			// 新规则，直接使用当前时间生成版本号
			version = generateVersion(now, "")
		}
		ruleInfo := RuleInfo{
			RuleID:   rule.RuleId,
			RuleName: rule.RuleName,
			Hash:     rule.Hash,
			Version:  version,
		}
		ruleInfos = append(ruleInfos, ruleInfo)
	}
	return ruleInfos
}

// 解析版本号，返回日期部分和序号
func parseVersion(version string) (date string, sequence int) {
	parts := strings.Split(version, ".")
	if len(parts) != 2 {
		return "", 0
	}
	date = parts[0]
	seq, _ := strconv.Atoi(parts[1])
	return date, seq
}

// 生成新的版本号
func generateVersion(now time.Time, existingVersion string) string {
	currentDate := fmt.Sprintf("%d%02d%02d", now.Year(), now.Month(), now.Day())

	if existingVersion == "" {
		return currentDate + ".0001"
	}

	existingDate, seq := parseVersion(existingVersion)
	if existingDate == currentDate {
		return fmt.Sprintf("%s.%04d", currentDate, seq+1)
	}

	return currentDate + ".0001"
}

func UpdateVersion(existingVersion string) string {
	now := time.Now()
	version := generateVersion(now, existingVersion)
	return version
}

func CheckNewerVersion(base, check string) bool {
	if check == "" { // base is newer
		return true
	}

	if base == "" { // check is newer
		return false
	}
	return base >= check
}
