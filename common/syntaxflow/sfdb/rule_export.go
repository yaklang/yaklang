package sfdb

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/tidwall/sjson"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

// =============================================================================
// Export Options
// =============================================================================

// RuleExportConfig 导出配置
type RuleExportConfig struct {
	Password         string
	ProgressCallback func(current, total int)
}

// RuleExportOption 导出选项
type RuleExportOption func(*RuleExportConfig)

// WithExportPassword 设置导出密码
func WithExportPassword(password string) RuleExportOption {
	return func(c *RuleExportConfig) {
		c.Password = password
	}
}

// WithExportProgress 设置导出进度回调
func WithExportProgress(callback func(current, total int)) RuleExportOption {
	return func(c *RuleExportConfig) {
		c.ProgressCallback = callback
	}
}

// RuleExportResult 导出结果
type RuleExportResult struct {
	Count int // 导出的规则数量
}

// =============================================================================
// Import Options
// =============================================================================

// RuleImportConfig 导入配置
type RuleImportConfig struct {
	Password         string
	ProgressCallback func(current, total int)
}

// RuleImportOption 导入选项
type RuleImportOption func(*RuleImportConfig)

// WithImportPassword 设置导入密码
func WithImportPassword(password string) RuleImportOption {
	return func(c *RuleImportConfig) {
		c.Password = password
	}
}

// WithImportProgress 设置导入进度回调
func WithImportProgress(callback func(current, total int)) RuleImportOption {
	return func(c *RuleImportConfig) {
		c.ProgressCallback = callback
	}
}

// =============================================================================
// Export Functions
// =============================================================================

// ExportRulesToZip 导出规则到 ZIP 文件
func ExportRulesToZip(ctx context.Context, db *gorm.DB, targetPath string, opts ...RuleExportOption) (*RuleExportResult, error) {
	// 解析配置
	config := &RuleExportConfig{}
	for _, opt := range opts {
		opt(config)
	}

	// 获取规则-组关系（独立查询）
	var rules []*schema.SyntaxFlowRule
	ruleGroupDB := db.Select(`"syntax_flow_rules".id, "syntax_flow_rules".rule_id`).Preload("Groups")
	if err := ruleGroupDB.Find(&rules).Error; err != nil {
		return nil, utils.Wrap(err, "get syntax flow groups failed")
	}

	// 构建元数据
	metadata := make(bizhelper.MetaData)
	metadata["relationship"] = lo.Map(rules, func(item *schema.SyntaxFlowRule, index int) map[string]any {
		groupNames := lo.Map(item.Groups, func(g *schema.SyntaxFlowGroup, _ int) string {
			return g.GroupName
		})
		return map[string]any{
			"rule_id":     item.RuleId,
			"group_names": groupNames,
		}
	})

	// 获取规则数量
	var ruleCount int
	if err := db.Count(&ruleCount).Error; err != nil {
		return nil, utils.Wrap(err, "get syntax flow rule count failed")
	}
	if ruleCount == 0 {
		return nil, utils.Error("no syntax flow rule found")
	}
	metadata["count"] = ruleCount

	// 构建 bizhelper 选项
	bizOpts := make([]bizhelper.ExportOption, 0)
	bizOpts = append(bizOpts, bizhelper.WithExportMetadata(metadata))

	if config.Password != "" {
		bizOpts = append(bizOpts, bizhelper.WithExportPassword(config.Password))
	}

	// 进度回调
	if config.ProgressCallback != nil {
		handled := 0
		bizOpts = append(bizOpts, bizhelper.WithExportAfterWriteHandler(func(name string, w []byte, m map[string]any) {
			handled++
			config.ProgressCallback(handled, ruleCount)
		}))
	}

	// 删除时间戳字段
	bizOpts = append(bizOpts, bizhelper.WithExportPreWriteHandler(func(name string, w []byte, m bizhelper.MetaData) (string, []byte) {
		nw, err := sjson.DeleteBytes(w, "CreatedAt")
		if err == nil {
			w = nw
		}
		nw, err = sjson.DeleteBytes(w, "UpdatedAt")
		if err == nil {
			w = nw
		}
		return name, w
	}))

	bizOpts = append(bizOpts, bizhelper.WithExportIndexField(`"syntax_flow_rules".id`))

	// 导出
	err := bizhelper.ExportTableZip[*schema.SyntaxFlowRule](ctx, db, targetPath, bizOpts...)
	if err != nil {
		return nil, utils.Wrap(err, "export syntax flow rules failed")
	}

	return &RuleExportResult{
		Count: ruleCount,
	}, nil
}

// ExportRulesToBytes 导出规则到内存
func ExportRulesToBytes(ctx context.Context, db *gorm.DB, opts ...RuleExportOption) ([]byte, *RuleExportResult, error) {
	tmpFile, err := os.CreateTemp("", "rule_export_*.zip")
	if err != nil {
		return nil, nil, utils.Wrap(err, "create temp file failed")
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	result, err := ExportRulesToZip(ctx, db, tmpPath, opts...)
	if err != nil {
		return nil, nil, err
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, nil, utils.Wrap(err, "read exported file failed")
	}

	return data, result, nil
}

// =============================================================================
// Import Functions
// =============================================================================

// ImportRulesFromZip 从 ZIP 文件导入规则
func ImportRulesFromZip(ctx context.Context, db *gorm.DB, inputPath string, opts ...RuleImportOption) (bizhelper.MetaData, error) {
	// 解析配置
	config := &RuleImportConfig{}
	for _, opt := range opts {
		opt(config)
	}

	var metadata bizhelper.MetaData

	// 构建 bizhelper 选项
	bizOpts := make([]bizhelper.ImportOption, 0)

	// 默认选项
	bizOpts = append(bizOpts, bizhelper.WithImportUniqueIndexField(`RuleId`))
	bizOpts = append(bizOpts, bizhelper.WithImportAllowOverwrite(true))

	if config.Password != "" {
		bizOpts = append(bizOpts, bizhelper.WithImportPassword(config.Password))
	}

	// 进度回调（需要从 metadata 获取 count）
	var ruleCount int
	if config.ProgressCallback != nil {
		handled := 0
		bizOpts = append(bizOpts, bizhelper.WithImportAfterReadHandler(func(name string, b []byte, m bizhelper.MetaData) {
			if ruleCount == 0 && m != nil {
				ruleCount = utils.InterfaceToInt(m["count"])
			}
			handled++
			if ruleCount > 0 {
				config.ProgressCallback(handled, ruleCount)
			}
		}))
	}

	// 元数据捕获（放在最后）
	bizOpts = append(bizOpts, bizhelper.WithMetaDataHandler(func(m bizhelper.MetaData) error {
		metadata = m
		ruleCount = utils.InterfaceToInt(m["count"])
		return nil
	}))

	// 导入规则
	ruleDB := db.Model(&schema.SyntaxFlowRule{})
	err := bizhelper.ImportTableZip[schema.SyntaxFlowRule](ctx, ruleDB, inputPath, bizOpts...)
	if err != nil {
		return nil, utils.Wrap(err, "import syntax flow rules failed")
	}

	// 恢复规则-组关系
	if err := restoreRuleGroups(db, metadata); err != nil {
		return metadata, utils.Wrap(err, "restore rule groups failed")
	}

	return metadata, nil
}

// ImportRulesFromBytes 从内存导入规则
func ImportRulesFromBytes(ctx context.Context, db *gorm.DB, data []byte, opts ...RuleImportOption) (bizhelper.MetaData, error) {
	tmpFile, err := os.CreateTemp("", "rule_import_*.zip")
	if err != nil {
		return nil, utils.Wrap(err, "create temp file failed")
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, bytes.NewReader(data)); err != nil {
		tmpFile.Close()
		return nil, utils.Wrap(err, "write temp file failed")
	}
	tmpFile.Close()

	return ImportRulesFromZip(ctx, db, tmpPath, opts...)
}

// =============================================================================
// Internal Helpers
// =============================================================================

// restoreRuleGroups 从 metadata 恢复规则-组关系
func restoreRuleGroups(db *gorm.DB, metadata bizhelper.MetaData) error {
	if metadata == nil {
		return nil
	}

	iGroups, ok := metadata["relationship"]
	if !ok {
		return nil
	}

	m, ok := iGroups.([]any)
	if !ok {
		return utils.Error("metadata: invalid relationship type")
	}

	for _, iItem := range m {
		item, ok := iItem.(map[string]any)
		if !ok {
			continue
		}
		ruleId, ok := item["rule_id"].(string)
		if !ok || ruleId == "" {
			continue
		}
		iGroupNames, ok := item["group_names"].([]any)
		if !ok || len(iGroupNames) == 0 {
			continue
		}

		groupNames := lo.Map(iGroupNames, func(item any, _ int) string {
			return utils.InterfaceToString(item)
		})

		groups := GetOrCreateGroups(db, groupNames)
		rules, err := QueryRulesById(db, []string{ruleId})
		if err != nil {
			return utils.Wrap(err, "query rules by id failed")
		}

		if len(rules) == 0 {
			continue
		}

		if len(groups) > 0 && len(rules) > 0 {
			for _, rule := range rules {
				if err := db.Model(rule).Association("Groups").Append(groups).Error; err != nil {
					return utils.Wrapf(err, "append groups to rule %s failed", ruleId)
				}
			}
		}
	}

	return nil
}
