package sfdb

// RuleSourceType 规则来源类型
type RuleSourceType string

const (
	RuleSourceTypeDatabase RuleSourceType = "database" // 数据库（默认）
	RuleSourceTypeOSS      RuleSourceType = "oss"      // OSS对象存储（已废弃，OSS逻辑移至yaklib）
)
