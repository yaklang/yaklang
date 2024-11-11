package schema

import (
	"github.com/jinzhu/gorm"
)

type SyntaxFlowScanTask struct {
	gorm.Model
	TaskId   string `gorm:"unique_index"`
	Programs string
	// rules
	RulesCount int64
	RuleFilter []byte

	Status string // executing / done / paused / error
	Reason string // user cancel / finished / recover failed so on

	// query execute
	FailedQuery  int64 // query failed
	SkipQuery    int64 // language not match, skip this rule
	SuccessQuery int64
	// risk
	RiskCount int64
	// query process
	TotalQuery int64
}
