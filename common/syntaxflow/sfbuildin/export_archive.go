//go:build !irify_exclude

package sfbuildin

import (
	"archive/zip"
	"encoding/json"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/tidwall/sjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin/standards"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

type archiveMetadata struct {
	Relationship []archiveRelationship `json:"relationship"`
}

type archiveRelationship struct {
	RuleID     string   `json:"rule_id"`
	GroupNames []string `json:"group_names"`
}

// ExportBuiltinRulesToArchive 将 embed.FS 中的内置规则导出为 ZIP 归档。
// ZIP 中每条规则以 schema.SyntaxFlowRule 的 JSON 序列化存储，消费者负责兼容解析。
func ExportBuiltinRulesToArchive(w io.Writer, notifies ...func(process float64, ruleName string)) error {
	InitEmbedFSWithNotify(nil)

	var sfFiles []struct {
		path string
		name string
	}

	err := filesys.Recursive(".", filesys.WithFileSystem(ruleFSWithHash), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		if strings.HasSuffix(info.Name(), ".sf") {
			sfFiles = append(sfFiles, struct{ path, name string }{s, info.Name()})
		}
		return nil
	}))
	if err != nil {
		return utils.Wrap(err, "enumerate builtin rules")
	}

	var notify func(process float64, ruleName string)
	if len(notifies) > 0 {
		notify = notifies[0]
	}

	enricher, _ := standards.GetGlobalEnricher()

	records := make([]*schema.SyntaxFlowRule, 0, len(sfFiles))
	relationships := make([]archiveRelationship, 0)

	for i, f := range sfFiles {
		raw, err := ruleFSWithHash.ReadFile(f.path)
		if err != nil {
			log.Warnf("read rule file %s: %v", f.path, err)
			continue
		}

		rule, err := sfdb.CheckSyntaxFlowRuleContent(string(raw))
		if err != nil {
			log.Warnf("parse rule %s: %v", f.path, err)
			continue
		}

		rule, groups := prepareExportRule(f.path, f.name, rule, string(raw), enricher)
		records = append(records, rule)

		if len(groups) > 0 {
			relationships = append(relationships, archiveRelationship{
				RuleID:     rule.RuleId,
				GroupNames: groups,
			})
		}

		if notify != nil && len(sfFiles) > 0 {
			notify(float64(i+1)/float64(len(sfFiles)), f.name)
		}
	}

	if len(records) == 0 {
		return utils.Error("no valid builtin rules found")
	}

	zw := zip.NewWriter(w)
	defer zw.Close()

	// meta.json
	metaBytes, err := json.MarshalIndent(archiveMetadata{Relationship: relationships}, "", "  ")
	if err != nil {
		return utils.Wrap(err, "marshal metadata")
	}
	metaWriter, err := zw.Create("meta.json")
	if err != nil {
		return utils.Wrap(err, "create meta.json in zip")
	}
	if _, err := metaWriter.Write(metaBytes); err != nil {
		return utils.Wrap(err, "write meta.json")
	}

	// 规则条目：直接序列化 schema.SyntaxFlowRule，清理 gorm 字段
	for _, rule := range records {
		data, err := json.MarshalIndent(rule, "", "  ")
		if err != nil {
			log.Warnf("marshal rule %s: %v", rule.RuleId, err)
			continue
		}

		// 删除 gorm.Model 产生的字段，避免暴露内部 ID 和时间戳
		data, _ = sjson.DeleteBytes(data, "ID")
		data, _ = sjson.DeleteBytes(data, "CreatedAt")
		data, _ = sjson.DeleteBytes(data, "UpdatedAt")
		data, _ = sjson.DeleteBytes(data, "DeletedAt")

		filename := strings.ReplaceAll(rule.RuleId, "/", "_") + ".json"
		writer, err := zw.Create(filename)
		if err != nil {
			log.Warnf("create %s in zip: %v", filename, err)
			continue
		}
		if _, err := writer.Write(data); err != nil {
			log.Warnf("write %s: %v", filename, err)
			continue
		}
	}

	return zw.Close()
}

func prepareExportRule(
	filePath string,
	fileName string,
	rule *schema.SyntaxFlowRule,
	content string,
	enricher *standards.RuleMetadataEnricher,
) (*schema.SyntaxFlowRule, []string) {
	// 生成稳定的 RuleId
	if strings.TrimSpace(rule.RuleId) == "" {
		rule.RuleId = uuid.NewSHA1(uuid.NameSpaceURL, []byte(filePath)).String()
	}

	// RuleName：复用 sfdb 内置规则的命名逻辑
	if rule.TitleZh != "" {
		rule.RuleName = rule.TitleZh
	} else if rule.Title != "" {
		rule.RuleName = rule.Title
	}
	if rule.RuleName == "" {
		rule.RuleName = fileName
	}

	// 推断语言
	if rule.Language == "" {
		languageRaw, _, _ := strings.Cut(fileName, "-")
		lang, err := ssaconfig.ValidateLanguage(languageRaw)
		if err == nil {
			rule.Language = lang
		}
	}

	// 规则类型
	if rule.Type == "" {
		ruleType, _ := sfdb.CheckSyntaxFlowRuleType(fileName)
		rule.Type = ruleType
	}

	// 标记为内置规则
	rule.IsBuildInRule = true

	// 内容
	rule.Content = content

	// Hash
	rule.CalcHash()

	groups := inferGroups(filePath, rule, enricher)
	return rule, groups
}

func inferGroups(filePath string, rule *schema.SyntaxFlowRule, enricher *standards.RuleMetadataEnricher) []string {
	var groups []string
	seen := make(map[string]struct{})

	// 1. 使用标准增强器（OWASP 映射、框架分组等）
	if enricher != nil {
		for _, g := range enricher.EnrichGroupNames(rule.RuleName, filePath, rule.CWE) {
			if _, ok := seen[g]; !ok {
				seen[g] = struct{}{}
				groups = append(groups, g)
			}
		}
	}

	// 2. 从目录路径提取基本分组
	parts := strings.Split(filepath.ToSlash(filePath), "/")
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" || part == "buildin" || strings.HasSuffix(part, ".sf") {
			continue
		}
		if _, ok := seen[part]; !ok {
			seen[part] = struct{}{}
			groups = append(groups, part)
		}
	}

	return groups
}
