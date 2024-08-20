package ssadb

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

type AuditNode struct {
	gorm.Model

	RuntimeId   string `json:"runtime_id" gorm:"index"`
	RuleName    string `json:"rule_name" gorm:"index"`
	RuleId      int64  `json:"rule_id" gorm:"index"`
	ProgramName string `json:"program_name" gorm:"index"`
	IsEntryNode bool   `json:"is_entry_node"`

	SsaId      int64  `json:"ssa_id"`
	ConstValue string `json:"const_value"`
}

type AuditEdgeType string

const (
	EdgeType_DependsOn AuditEdgeType = "depends_on"
	EdgeType_EffectsOn AuditEdgeType = "effects_on"

	// EdgeType_Predecessor 记录审计过程
	EdgeType_Predecessor AuditEdgeType = "predecessor"
)

type AuditEdge struct {
	gorm.Model

	RuntimeId   string `json:"runtime_id" gorm:"index"`
	RuleName    string `json:"rule_name" gorm:"index"`
	RuleId      int64  `json:"rule_id" gorm:"index"`
	ProgramName string `json:"program_name" gorm:"index"`

	FromNode int64
	ToNode   int64
	EdgeType AuditEdgeType

	AnalysisStep  int64
	AnalysisLabel string
}

func (n *AuditNode) CreateDependsOnEdge(to int64) *AuditEdge {
	ae := &AuditEdge{
		RuntimeId:   n.RuntimeId,
		RuleName:    n.RuleName,
		RuleId:      n.RuleId,
		ProgramName: n.ProgramName,
		FromNode:    int64(n.ID),
		ToNode:      to,
		EdgeType:    EdgeType_DependsOn,
	}
	return ae
}

func (n *AuditNode) CreateEffectsOnEdge(to int64) *AuditEdge {
	ae := &AuditEdge{
		RuntimeId:   n.RuntimeId,
		RuleName:    n.RuleName,
		RuleId:      n.RuleId,
		ProgramName: n.ProgramName,
		FromNode:    int64(n.ID),
		ToNode:      to,
		EdgeType:    EdgeType_EffectsOn,
	}
	return ae
}

func (n *AuditNode) CreatePredecessorEdge(to int64, step int64, label string) *AuditEdge {
	ae := &AuditEdge{
		RuntimeId:     n.RuntimeId,
		RuleName:      n.RuleName,
		RuleId:        n.RuleId,
		ProgramName:   n.ProgramName,
		FromNode:      int64(n.ID),
		ToNode:        to,
		EdgeType:      EdgeType_Predecessor,
		AnalysisStep:  step,
		AnalysisLabel: label,
	}
	return ae
}

func YieldAuditNodeByRuntimeId(db *gorm.DB, runtimeId string) chan *AuditNode {
	db = db.Model(&AuditNode{}).Where("runtime_id = ?", runtimeId)
	return yieldAuditNode(db, context.Background())
}

func YieldAuditNodeByRuleName(db *gorm.DB, ruleName string) chan *AuditNode {
	db = db.Model(&AuditNode{}).Where("rule_name = ?", ruleName)
	return yieldAuditNode(db, context.Background())
}

func yieldAuditNode(db *gorm.DB, ctx context.Context) chan *AuditNode {
	db = db.Model(&AuditNode{}).Where("is_entry_node = true")
	outC := make(chan *AuditNode)
	go func() {
		defer close(outC)

		var page = 1
		for {
			var items []*AuditNode
			if _, b := bizhelper.Paging(db, page, 100, &items); b.Error != nil {
				log.Errorf("paging failed: %s", b.Error)
				return
			}

			page++
			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}

			if len(items) < 100 {
				return
			}
		}
	}()
	return outC
}
