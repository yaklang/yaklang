package schema

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// SyntaxFlowRulePurposeType 规则用途类型
// 定义规则的使用目的和场景
type SyntaxFlowRulePurposeType string

// SyntaxFlowRuleType 规则类型
// 定义规则的实现方式（SyntaxFlow 或 Yak 脚本）
type SyntaxFlowRuleType string

// SyntaxFlowSeverity 严重程度级别
// 定义检测到的问题的严重程度
type SyntaxFlowSeverity string

const (
	// SFR_PURPOSE_AUDIT 审计用途
	// 用于代码质量审计、最佳实践检查
	SFR_PURPOSE_AUDIT SyntaxFlowRulePurposeType = "audit"

	// SFR_PURPOSE_VULN 漏洞检测
	// 用于发现安全漏洞和脆弱点
	SFR_PURPOSE_VULN SyntaxFlowRulePurposeType = "vuln"

	// SFR_PURPOSE_CONFIG 配置检查
	// 用于检测配置错误和安全配置问题
	SFR_PURPOSE_CONFIG SyntaxFlowRulePurposeType = "config"

	// SFR_PURPOSE_SECURITY 安全检查
	// 用于一般性安全问题检测
	SFR_PURPOSE_SECURITY SyntaxFlowRulePurposeType = "security"
)

func GetAllSFPurposeTypes() []string {
	return []string{
		string(SFR_PURPOSE_AUDIT),
		string(SFR_PURPOSE_VULN),
		string(SFR_PURPOSE_CONFIG),
		string(SFR_PURPOSE_SECURITY),
	}
}

func GetAllSFSupportLanguage() []string {
	return ssaconfig.GetAllSupportedLanguages()
}

const (
	// SFR_SEVERITY_INFO 信息级别
	// 提示性信息，不影响安全，如代码风格建议
	SFR_SEVERITY_INFO SyntaxFlowSeverity = "info"

	// SFR_SEVERITY_LOW 低危
	// 较小的安全风险，影响有限
	SFR_SEVERITY_LOW SyntaxFlowSeverity = "low"

	// SFR_SEVERITY_WARNING 中危（警告）
	// 中等程度的安全风险，应该修复
	SFR_SEVERITY_WARNING SyntaxFlowSeverity = "middle"

	// SFR_SEVERITY_HIGH 高危
	// 严重的安全风险，需要优先处理
	SFR_SEVERITY_HIGH SyntaxFlowSeverity = "high"

	// SFR_SEVERITY_CRITICAL 严重（危急）
	// 极其严重的安全漏洞，必须立即修复
	SFR_SEVERITY_CRITICAL SyntaxFlowSeverity = "critical"
)

func GetAllSFSeverityTypes() []string {
	return []string{
		string(SFR_SEVERITY_INFO),
		string(SFR_SEVERITY_LOW),
		string(SFR_SEVERITY_WARNING),
		string(SFR_SEVERITY_CRITICAL),
		string(SFR_SEVERITY_HIGH),
	}
}

const (
	// SFR_RULE_TYPE_YAK Yak 脚本类型规则
	// 使用 Yak 脚本语言编写的规则，功能更灵活
	SFR_RULE_TYPE_YAK SyntaxFlowRuleType = "yak"

	// SFR_RULE_TYPE_SF SyntaxFlow 类型规则
	// 使用 SyntaxFlow DSL 编写的规则，专注于数据流分析
	SFR_RULE_TYPE_SF SyntaxFlowRuleType = "sf"
)

func ValidRuleType(i any) SyntaxFlowRuleType {
	switch strings.ToLower(codec.AnyToString(i)) {
	case "yak", "y", "yaklang":
		return SFR_RULE_TYPE_YAK
	case "sf", "syntaxflow":
		return SFR_RULE_TYPE_SF
	default:
		return SFR_RULE_TYPE_SF
	}
}

func ValidSeverityType(i any) SyntaxFlowSeverity {
	switch strings.ToLower(yakunquote.TryUnquote(codec.AnyToString(i))) {
	case "info", "i", "verbose", "debug", "prompt":
		return SFR_SEVERITY_INFO
	case "warning", "w", "middle", "mid", "warn", "medium":
		return SFR_SEVERITY_WARNING
	case "critical", "c", "fatal", "e", "essential":
		return SFR_SEVERITY_CRITICAL
	case "high", "h", "error":
		return SFR_SEVERITY_HIGH
	case "low", "l":
		return SFR_SEVERITY_LOW
	default:
		return SFR_SEVERITY_INFO
	}
}

func ValidPurpose(i any) SyntaxFlowRulePurposeType {
	result := yakunquote.TryUnquote(codec.AnyToString(i))
	switch strings.ToLower(result) {
	case "audit", "a", "audition":
		return SFR_PURPOSE_AUDIT
	case "vuln", "v", "vulnerability", "vul", "vulnerabilities", "weak", "weakness":
		return SFR_PURPOSE_VULN
	case "config", "c", "configuration", "conf", "configure":
		return SFR_PURPOSE_CONFIG
	case "security", "s", "secure", "securely", "secureity":
		return SFR_PURPOSE_SECURITY
	default:
		return SFR_PURPOSE_AUDIT
	}
}

type MapEx[K comparable, V any] map[K]V

func (m *MapEx[K, V]) Scan(value interface{}) error {
	return json.Unmarshal(codec.AnyToBytes(value), m)
}
func (m MapEx[K, V]) Value() (driver.Value, error) {
	return json.Marshal(m)
}

type SlicesEx[K comparable] []K

func (s *SlicesEx[K]) Scan(value interface{}) error {
	return json.Unmarshal(codec.AnyToBytes(value), s)
}

func (s *SlicesEx[K]) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// SyntaxFlowDescInfo 规则描述信息
// 用于描述规则和创建风险告警，支持多变量独立告警配置
type SyntaxFlowDescInfo struct {
	// RuleId 关联的规则ID
	RuleId string `json:"rule_id"`

	// Title 告警标题（英文）
	Title string `json:"title"`

	// TitleZh 告警标题（中文）
	TitleZh string `json:"title_zh"`

	// Description 告警详细描述
	// 说明检测到的问题和影响
	Description string `json:"description"`

	// Solution 解决方案
	// 提供修复建议和最佳实践
	Solution string `json:"solution"`

	// Tag 告警标签
	Tag string `json:"tag"`

	// Severity 严重程度
	// 可选值: info（信息）、low（低危）、middle（中危）、high（高危）、critical（严重）
	Severity SyntaxFlowSeverity `json:"severity"`

	// Purpose 告警目的
	// 可选值: audit（审计）、vuln（漏洞）、config（配置）、security（安全）
	Purpose SyntaxFlowRulePurposeType `json:"purpose"`

	// OnlyMsg 是否仅使用自定义消息
	// true: 只使用 Msg 字段的内容
	// false: 使用完整的结构化信息（Title + Description）
	OnlyMsg bool `json:"only_msg"`

	// Msg 自定义消息内容
	// 当 OnlyMsg=true 时，直接使用此消息作为告警内容
	Msg string `json:"msg"`

	// CVE 关联的 CVE 编号
	CVE string `json:"cve"`

	// CWE 关联的 CWE 列表
	CWE StringArray `json:"cwe"`

	// RiskType 风险类型分类
	RiskType string

	// ExtraInfo 额外信息
	// 键值对形式存储其他自定义信息
	ExtraInfo map[string]string `json:"extra_info"`
}

func ToSyntaxFlowAlertDesc(message *ypb.AlertMessage) *SyntaxFlowDescInfo {
	return &SyntaxFlowDescInfo{
		Title:       message.Title,
		TitleZh:     message.TitleZh,
		Description: message.Description,
		Solution:    message.Solution,
		Tag:         message.Tag,
		Severity:    SyntaxFlowSeverity(message.Severity),
		Purpose:     SyntaxFlowRulePurposeType(message.Purpose),
		OnlyMsg:     false,
		Msg:         message.Msg,
		CVE:         message.Cve,
		RiskType:    message.RiskType,
		ExtraInfo:   message.Extra,
	}
}
func (s *SyntaxFlowDescInfo) ToYpbSyntaxFlowRuleDesc() *ypb.AlertMessage {
	return &ypb.AlertMessage{
		Title:       s.Title,
		TitleZh:     s.TitleZh,
		Description: s.Description,
		Solution:    s.Solution,
		Severity:    string(s.Severity),
		Purpose:     string(s.Purpose),
		Msg:         s.Msg,
		Cve:         s.CVE,
		RiskType:    s.RiskType,
		Tag:         s.Tag,
		Extra:       s.ExtraInfo,
	}
}

func (info *SyntaxFlowDescInfo) String() string {
	if info.OnlyMsg {
		return info.Msg
	}
	return fmt.Sprintf("%s: %s", info.Title, info.Description)
}

func (info *SyntaxFlowDescInfo) ShortMessage() string {
	if info.OnlyMsg {
		return info.Msg
	}
	title := info.TitleZh
	if title == "" {
		title = info.Title
	}
	return fmt.Sprintf("%s: %s", info.TitleZh, info.Msg)
}


// SyntaxFlowRule SyntaxFlow 规则定义
// 用于存储和管理静态代码分析规则，支持多语言代码扫描和漏洞检测
type SyntaxFlowRule struct {
	gorm.Model

	// ============ 标识字段 ============

	// RuleId 规则唯一标识符（UUID）
	// 用于全局唯一标识一个规则，支持跨数据库/跨平台同步
	RuleId string `gorm:"unique_index"`

	// Version 规则版本号
	// 用于版本管理和更新检测，格式如 "1.0.0"、"2.1.3" 等
	// 下载规则时会比较本地和在线版本，决定是否需要更新
	Version string

	// Hash 规则内容哈希值
	// 基于 RuleId、RuleName、Content、Tag、OpCodes 计算
	// 用于快速检测规则内容是否变化，确保数据一致性
	Hash string `json:"hash" gorm:"unique_index"`

	// ============ 基本信息 ============

	// RuleName 规则名称（唯一）
	// 用于标识和查询规则，如 "java-sql-injection"、"runtime-command-exec"
	// 必须唯一，不可重复
	RuleName string `gorm:"unique_index"`

	// Title 规则标题（英文）
	// 简短描述规则用途，如 "SQL Injection Detection"
	Title string

	// TitleZh 规则标题（中文）
	// 中文标题，如 "SQL注入检测"
	TitleZh string

	// ============ 分类和标签 ============

	// Language 规则适用的编程语言
	// 如 "java"、"php"、"javascript" 等
	// 支持的语言列表见 ssaconfig.GetAllSupportedLanguages()
	Language ssaconfig.Language

	// Purpose 规则用途/目的
	// 可选值: audit（审计）、vuln（漏洞）、config（配置）、security（安全）
	Purpose SyntaxFlowRulePurposeType

	// Tag 规则标签
	// 用于分类和筛选，可包含多个标签，如 "injection,security,owasp"
	Tag string

	// CWE 通用弱点枚举列表
	// Common Weakness Enumeration，如 ["CWE-89", "CWE-564"]
	// 用于标准化漏洞分类
	CWE StringArray `gorm:"type:text" json:"cwe"`

	// CVE 通用漏洞编号
	// Common Vulnerabilities and Exposures，如 "CVE-2021-12345"
	CVE string

	// RiskType 风险类型
	// 用于区分不同类型的安全风险
	RiskType string

	// Type 规则类型
	// 可选值: "sf"（SyntaxFlow）、"yak"（Yak 脚本）
	Type SyntaxFlowRuleType

	// Severity 严重程度
	// 可选值: info（信息）、low（低危）、middle（中危）、high（高危）、critical（严重）
	Severity SyntaxFlowSeverity

	// ============ 属性标记 ============

	// IsBuildInRule 是否为内置规则
	// true: 系统自带规则，通常不可修改
	// false: 用户自定义规则
	IsBuildInRule bool

	// Verified 规则是否已验证
	// true: 规则已经过测试和验证，质量有保证
	// false: 未验证的规则，可能需要进一步测试
	Verified bool

	// NeedUpdate 规则相较于远端是否有改动
	// true: 远端下载到本地后有新的改动（需要进行冲突检测）
	// false: 远端下载到本地后没有改动（可以直接覆盖最新版本）
	NeedUpdate bool

	// ============ 核心内容 ============

	// Content 规则主体内容（最重要）
	// SyntaxFlow 规则代码或 Yak 脚本代码
	// 这是规则的核心，定义了检测逻辑
	// 示例:
	//   desc(title: "SQL注入")
	//   executeQuery(* as $sql) as $result;
	//   alert $result;
	Content string

	// Description 规则详细描述
	// 说明规则的检测目标、原理等
	// 如 "检测使用字符串拼接构造 SQL 查询的安全风险"
	Description string

	// AlertDesc 告警描述映射
	// 键为变量名（如 "$result"），值为对应的告警信息
	// 用于为不同的检测结果提供不同的告警描述
	AlertDesc MapEx[string, *SyntaxFlowDescInfo] `gorm:"type:text"`

	// Solution 解决方案
	// 提供修复建议，如 "使用预编译语句（PreparedStatement）"
	Solution string

	// ============ 包含机制 ============

	// AllowIncluded 是否允许被其他规则包含
	// true: 可以作为库规则被其他规则引用
	// false: 独立规则，不能被包含
	AllowIncluded bool

	// IncludedName 包含时使用的名称
	// 当 AllowIncluded=true 时，其他规则通过此名称引用本规则
	IncludedName string

	// ============ 内部字段 ============

	// OpCodes 操作码（本地使用）
	// 编译后的字节码或中间表示，用于优化执行性能
	OpCodes string

	// ============ 关联关系 ============

	// Groups 规则所属的分组列表
	// 多对多关系，一个规则可以属于多个分组
	// 通过中间表 syntax_flow_rule_and_group 关联
	Groups []*SyntaxFlowGroup `gorm:"many2many:syntax_flow_rule_and_group;"`
}

func (s *SyntaxFlowRule) CalcHash() string {
	s.Hash = utils.CalcSha256(s.RuleId, s.RuleName, s.Content, s.Tag, s.OpCodes)
	return s.Hash
}

func (s *SyntaxFlowRule) BeforeSave() error {
	if s.RuleId == "" {
		s.RuleId = uuid.NewString()
	}
	s.CalcHash()
	s.Purpose = ValidPurpose(s.Purpose)
	s.Type = ValidRuleType(s.Type)
	s.Severity = ValidSeverityType(s.Severity)
	return nil
}

func (s *SyntaxFlowRule) BeforeCreate() error {
	if s.RuleId == "" {
		s.RuleId = uuid.NewString()
	}
	s.CalcHash()
	s.Purpose = ValidPurpose(s.Purpose)
	s.Type = ValidRuleType(s.Type)
	s.Severity = ValidSeverityType(s.Severity)
	return nil
}

func (s *SyntaxFlowRule) GetAlertInfo(name string) (*SyntaxFlowDescInfo, bool) {
	if info, ok := s.AlertDesc[name]; ok {
		return info, true
	}
	return nil, false
}

func (s *SyntaxFlowRule) GetInfo() *SyntaxFlowDescInfo {
	// load info from rule self, and create new syntaxflowdescinfo return
	info := &SyntaxFlowDescInfo{
		RuleId:      s.RuleId,
		Title:       s.Title,
		TitleZh:     s.TitleZh,
		Description: s.Description,
		Solution:    s.Solution,
		Tag:         s.Tag,
		Severity:    s.Severity,
		Purpose:     s.Purpose,
		OnlyMsg:     false,
		Msg:         "",
		CVE:         s.CVE,
		RiskType:    s.RiskType,
	}
	return info
}

func (s *SyntaxFlowRule) ToGRPCModel() *ypb.SyntaxFlowRule {
	groupNames := make([]string, 0, len(s.Groups))
	for _, group := range s.Groups {
		groupNames = append(groupNames, group.GroupName)
	}
	alertmsg := make(map[string]*ypb.AlertMessage)
	for name, info := range s.AlertDesc {
		if info.Title == "" {
			info.Title = s.Title
		}
		if info.TitleZh == "" {
			info.TitleZh = s.TitleZh
		}
		if info.Description == "" {
			info.Description = s.Description
		}
		if info.Severity == "" {
			info.Severity = s.Severity
		}
		if info.Solution == "" {
			info.Solution = s.Solution
		}
		alertmsg[name] = info.ToYpbSyntaxFlowRuleDesc()
	}
	sfRule := &ypb.SyntaxFlowRule{
		Id:            int64(s.ID),
		IsBuildInRule: s.IsBuildInRule,
		Language:      string(s.Language),
		RuleName:      s.RuleName,
		Title:         s.Title,
		TitleZh:       s.TitleZh,
		Description:   s.Description,
		Type:          string(s.Type),
		Severity:      string(s.Severity),
		Content:       s.Content,
		Purpose:       string(s.Purpose),
		Verified:      s.Verified,
		AllowIncluded: s.AllowIncluded,
		IncludedName:  s.IncludedName,
		Hash:          s.Hash,
		Tag:           s.Tag,
		GroupName:     groupNames,
		AlertMsg:      alertmsg,
	}
	return sfRule
}
