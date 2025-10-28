package sfdb

// RuleSourceType 规则来源类型
type RuleSourceType string

const (
	RuleSourceTypeDatabase RuleSourceType = "database" // 数据库（默认）
	RuleSourceTypeOSS      RuleSourceType = "oss"      // OSS对象存储
)

// OSSRuleSourceConfig OSS规则来源配置
type OSSRuleSourceConfig struct {
	Endpoint        string  `json:"endpoint"`          // OSS endpoint
	AccessKeyID     string  `json:"access_key_id"`     // Access Key ID
	AccessKeySecret string  `json:"access_key_secret"` // Access Key Secret
	Bucket          string  `json:"bucket"`            // Bucket名称
	Prefix          string  `json:"prefix"`            // 规则前缀
	Region          string  `json:"region"`            // 区域
	EnableCache     bool    `json:"enable_cache"`      // 启用缓存
	Type            OSSType `json:"type"`              // OSS类型
}
