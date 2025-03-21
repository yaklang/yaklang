package schema

import "github.com/jinzhu/gorm"

type ExtractedData struct {
	gorm.Model

	// sourcetype 一般来说是标注数据来源
	SourceType string `gorm:"index"`

	// trace id 表示数据源的 ID
	TraceId string `gorm:"index"`

	// 提取数据的正则数据
	Regexp string

	// 规则 Verbose
	RuleVerbose string

	// UTF8 safe escape
	Data string

	// DataIndex 表示数据的位置
	DataIndex int

	// Length 表示数据的长度
	Length int

	// IsMatchRequest 表示是否是匹配请求
	IsMatchRequest bool

	AnalyzedHTTPFlowId uint
}
