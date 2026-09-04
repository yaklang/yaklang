//go:build !irify_exclude

package sfbuildin

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// GenerateRuleVersionsFromLocalFS walks the given local rule directories,
// parses every .sf file, and produces rule_versions.json entries. The existing
// file at baselinePath (if any) is used as the version baseline so unchanged
// rules keep their version while changed/new rules get a bumped version.
func GenerateRuleVersionsFromLocalFS(dirs []string, baselinePath string) ([]sfdb.RuleInfo, error) {
	baseline := map[string]*sfdb.RuleInfo{}
	var baselineOrder []string
	if data, err := os.ReadFile(baselinePath); err == nil {
		var rules []sfdb.RuleInfo
		if err := json.Unmarshal(data, &rules); err == nil {
			for _, r := range rules {
				if _, dup := baseline[r.RuleID]; !dup {
					baselineOrder = append(baselineOrder, r.RuleID)
				}
				baseline[r.RuleID] = &r
			}
		} else {
			log.Warnf("failed to parse baseline rule versions %s: %v", baselinePath, err)
		}
	}

	var ruleInfos []sfdb.RuleInfo
	for _, dir := range dirs {
		err := filesys.Recursive(dir, filesys.WithFileSystem(filesys.NewLocalFs()), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
			if !strings.HasSuffix(info.Name(), ".sf") {
				return nil
			}
			raw, err := os.ReadFile(s)
			if err != nil {
				return utils.Wrapf(err, "read rule file %s failed", s)
			}
			ruleInfo, err := buildRuleInfoFromLocalFile(dir, s, string(raw))
			if err != nil {
				return err
			}
			if existing, ok := baseline[ruleInfo.RuleID]; ok {
				if existing.Version == "" {
					ruleInfo.Version = sfdb.UpdateVersion("")
				} else if existing.Hash != ruleInfo.Hash {
					ruleInfo.Version = sfdb.UpdateVersion(existing.Version)
				} else {
					ruleInfo.Version = existing.Version
				}
			} else {
				ruleInfo.Version = sfdb.UpdateVersion("")
			}
			ruleInfos = append(ruleInfos, *ruleInfo)
			return nil
		}))
		if err != nil {
			return nil, err
		}
	}

	sort.Slice(ruleInfos, func(i, j int) bool {
		return ruleInfos[i].RuleID < ruleInfos[j].RuleID
	})
	return orderByBaseline(ruleInfos, baselineOrder), nil
}

// orderByBaseline restores the ordering of the existing rule_versions.json so
// that regenerating an unchanged file is byte-identical (no diff, therefore no
// bot commit). Rules that are new to the baseline are appended in rule-id order.
func orderByBaseline(ruleInfos []sfdb.RuleInfo, baselineOrder []string) []sfdb.RuleInfo {
	byID := make(map[string]sfdb.RuleInfo, len(ruleInfos))
	for _, r := range ruleInfos {
		byID[r.RuleID] = r
	}

	ordered := make([]sfdb.RuleInfo, 0, len(ruleInfos))
	emitted := make(map[string]bool, len(ruleInfos))
	for _, id := range baselineOrder {
		if r, ok := byID[id]; ok && !emitted[id] {
			ordered = append(ordered, r)
			emitted[id] = true
		}
	}
	for _, r := range ruleInfos {
		if !emitted[r.RuleID] {
			ordered = append(ordered, r)
			emitted[r.RuleID] = true
		}
	}
	return ordered
}

// buildRuleInfoFromLocalFile parses a local .sf file and computes the same
// RuleId/RuleName/Hash that the database import path would produce, without
// touching a database.
func buildRuleInfoFromLocalFile(rootDir, filePath, content string) (*sfdb.RuleInfo, error) {
	rule, err := sfdb.CheckSyntaxFlowRuleContent(content)
	if err != nil {
		return nil, utils.Wrapf(err, "parse rule %s failed", filePath)
	}

	relPath, err := filepath.Rel(rootDir, filePath)
	if err != nil {
		relPath = filePath
	}
	dirName := filepath.Dir(relPath)

	var tags []string
	for _, block := range utils.PrettifyListFromStringSplitEx(dirName, "/", "\\", ",", "|") {
		block = strings.ToLower(block)
		if block == "" || block == "." || block == "buildin" {
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
			result, err := regexp_utils.NewYakRegexpUtils(`(cve-\d+-\d+)([_-\\.](.*))?`).FindStringSubmatch(block)
			if err != nil {
				continue
			}
			tags = append(tags, strings.ToUpper(result[1]))
			tags = append(tags, result[3])
			continue
		}
		tags = append(tags, block)
	}

	contentTag := rule.Tag
	merged := ""
	for _, t := range tags {
		merged = sfvm.AppendRuleTag(merged, t)
	}
	for _, t := range strings.Split(contentTag, "|") {
		merged = sfvm.AppendRuleTag(merged, t)
	}
	rule.Tag = merged

	if rule.TitleZh != "" {
		rule.RuleName = rule.TitleZh
	} else if rule.Title != "" {
		rule.RuleName = rule.Title
	} else {
		rule.RuleName = filepath.Base(filePath)
	}

	rule.CalcHash()
	return &sfdb.RuleInfo{
		RuleID:   rule.RuleId,
		RuleName: rule.RuleName,
		Hash:     rule.Hash,
	}, nil
}
