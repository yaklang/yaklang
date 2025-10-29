package yaklib

import (
	"context"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
)

// OSSRuleFileDownloadStream OSS规则文件下载流
type OSSRuleFileDownloadStream struct {
	Total int64
	Chan  chan *OSSRuleFileItem
}

// OSSRuleFileItem OSS规则文件项（原始内容，未解析）
type OSSRuleFileItem struct {
	RuleName string // 规则名称（从文件名提取）
	Content  string // 规则内容（.sf 文件原始内容）
	Key      string // OSS 对象 key
	Error    error  // 错误
}

// DownloadOSSSyntaxFlowRuleFiles 从OSS下载SyntaxFlow规则文件
// 注意：此函数只下载原始 .sf 文件内容，不解析规则
// 解析和保存的逻辑应该在调用方完成（避免循环导入）
//
// 参数:
//   - ctx: 上下文
//   - ossClient: OSS客户端
//   - bucket: bucket名称
//   - prefix: 规则文件前缀（如 "syntaxflow/"）
//
// 返回: 规则文件下载流
func DownloadOSSSyntaxFlowRuleFiles(
	ctx context.Context,
	ossClient OSSClient,
	bucket string,
	prefix string,
) *OSSRuleFileDownloadStream {
	ch := make(chan *OSSRuleFileItem, 10)
	rsp := &OSSRuleFileDownloadStream{
		Total: 0,
		Chan:  ch,
	}

	go func() {
		defer close(ch)
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("recover from DownloadOSSSyntaxFlowRuleFiles panic: %v", err)
			}
		}()

		// 1. 列出所有 .sf 文件
		objects, err := ossClient.ListObjects(bucket, prefix)
		if err != nil {
			ch <- &OSSRuleFileItem{
				Error: utils.Wrapf(err, "list objects from OSS failed"),
			}
			return
		}

		// 过滤出 .sf 文件
		sfFiles := make([]OSSObject, 0)
		for _, obj := range objects {
			if strings.HasSuffix(obj.Key, ".sf") {
				sfFiles = append(sfFiles, obj)
			}
		}

		rsp.Total = int64(len(sfFiles))
		log.Infof("Found %d .sf files in OSS bucket %s with prefix %s", len(sfFiles), bucket, prefix)

		// 2. 逐个下载规则文件
		for i, obj := range sfFiles {
			select {
			case <-ctx.Done():
				log.Info("context cancelled, stop downloading rules from OSS")
				return
			default:
			}

			log.Infof("[%d/%d] Downloading: %s", i+1, len(sfFiles), obj.Key)

			// 下载文件内容
			content, err := ossClient.GetObject(bucket, obj.Key)
			if err != nil {
				ch <- &OSSRuleFileItem{
					Error: utils.Wrapf(err, "get object %s failed", obj.Key),
				}
				continue
			}

			// 提取规则名称
			ruleName := extractRuleNameFromKey(obj.Key, prefix)

			// 发送原始内容（不解析）
			select {
			case ch <- &OSSRuleFileItem{
				RuleName: ruleName,
				Content:  string(content),
				Key:      obj.Key,
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return rsp
}

// extractRuleNameFromKey 从对象key中提取规则名称
// 例如: "syntaxflow/java/sql-injection.sf" -> "sql-injection"
//
//	"syntaxflow/php/xss-check.sf" -> "xss-check"
func extractRuleNameFromKey(key string, prefix string) string {
	// 移除前缀
	name := strings.TrimPrefix(key, prefix)

	// 移除 .sf 后缀
	name = strings.TrimSuffix(name, ".sf")

	// 移除路径分隔符，只保留文件名
	parts := strings.Split(name, "/")
	if len(parts) > 0 {
		name = parts[len(parts)-1]
	}

	// 如果为空，使用完整路径（去掉后缀）
	if name == "" {
		name = strings.TrimSuffix(key, ".sf")
		name = strings.ReplaceAll(name, "/", "_")
	}

	return name
}

// SaveOSSRule 保存从OSS下载的规则到数据库
// 注意：这个函数接受规则内容和元数据，需要调用方从.sf文件中解析
// 类似于 SaveSyntaxFlowRule，但是专门用于OSS规则
//
// 参数:
//   - db: 数据库连接（如果为nil，使用默认的profile数据库）
//   - ruleName: 规则名称
//   - content: 规则内容（.sf文件的完整内容）
//   - groupNames: 组名列表
func SaveOSSRule(db *gorm.DB, ruleName, content string, groupNames ...string) error {
	if db == nil {
		db = consts.GetGormProfileDatabase()
	}

	if db == nil {
		return utils.Error("empty database")
	}

	if ruleName == "" {
		return utils.Error("rule name is empty")
	}

	if content == "" {
		return utils.Error("rule content is empty")
	}

	// 创建基本规则对象
	rule := &schema.SyntaxFlowRule{
		RuleName: ruleName,
		Content:  content,
		Language: "", // 需要从content解析
		Type:     schema.SFR_RULE_TYPE_SF,
		Severity: schema.SFR_SEVERITY_WARNING,
		Purpose:  schema.SFR_PURPOSE_VULN,
	}

	// 尝试从content中提取基本信息（简化版本）
	// 注意：完整的解析需要语法解析器
	if strings.Contains(content, "type: vuln") {
		rule.Type = schema.SFR_RULE_TYPE_SF
	}
	if strings.Contains(content, "level: high") {
		rule.Severity = schema.SFR_SEVERITY_HIGH
	} else if strings.Contains(content, "level: mid") {
		rule.Severity = schema.SFR_SEVERITY_WARNING
	} else if strings.Contains(content, "level: low") {
		rule.Severity = schema.SFR_SEVERITY_LOW
	}

	// 尝试提取标题（简化版本）
	if idx := strings.Index(content, "title_zh:"); idx > 0 {
		rest := content[idx:]
		if endIdx := strings.Index(rest, "\n"); endIdx > 0 {
			rule.TitleZh = strings.Trim(strings.TrimSpace(rest[:endIdx]), "\"")
		}
	}
	if idx := strings.Index(content, "title:"); idx > 0 {
		rest := content[idx:]
		if endIdx := strings.Index(rest, "\n"); endIdx > 0 {
			rule.Title = strings.Trim(strings.TrimSpace(rest[:endIdx]), "\"")
		}
	}

	// 生成hash
	rule.CalcHash()

	// 保存规则到数据库
	_, err := sfdb.CreateOrUpdateRuleWithGroup(rule, groupNames...)
	if err != nil {
		return utils.Wrapf(err, "save OSS rule %s failed", ruleName)
	}

	log.Infof("successfully saved OSS rule: %s", ruleName)
	return nil
}

// SaveOSSRuleWithFullInfo 保存OSS规则（完整版本）
// 接受完整的规则信息，用于精确控制规则属性
//
// 参数:
//   - db: 数据库连接
//   - ruleName: 规则名称
//   - language: 编程语言
//   - ruleType: 规则类型
//   - severity: 严重程度
//   - purpose: 规则目的
//   - content: 规则内容
//   - title: 标题
//   - titleZh: 中文标题
//   - description: 描述
//   - tag: 标签
//   - cve: CVE编号
//   - cwe: CWE列表
//   - groupNames: 组名列表
func SaveOSSRuleWithFullInfo(
	db *gorm.DB,
	ruleName, language, content string,
	ruleType schema.SyntaxFlowRuleType,
	severity schema.SyntaxFlowSeverity,
	purpose schema.SyntaxFlowRulePurposeType,
	title, titleZh, description, tag, cve string,
	cwe []string,
	groupNames ...string,
) error {
	if db == nil {
		db = consts.GetGormProfileDatabase()
	}

	if db == nil {
		return utils.Error("empty database")
	}

	if ruleName == "" {
		return utils.Error("rule name is empty")
	}

	if content == "" {
		return utils.Error("rule content is empty")
	}

	rule := &schema.SyntaxFlowRule{
		RuleName:    ruleName,
		Language:    language,
		Content:     content,
		Type:        ruleType,
		Severity:    severity,
		Purpose:     purpose,
		Title:       title,
		TitleZh:     titleZh,
		Description: description,
		Tag:         tag,
		CVE:         cve,
		CWE:         cwe,
	}

	// 生成hash
	rule.CalcHash()

	// 保存到数据库
	_, err := sfdb.CreateOrUpdateRuleWithGroup(rule, groupNames...)
	if err != nil {
		return utils.Wrapf(err, "save OSS rule %s failed", ruleName)
	}

	log.Infof("successfully saved OSS rule with full info: %s", ruleName)
	return nil
}
