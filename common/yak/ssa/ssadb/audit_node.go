package ssadb

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

type AuditNodeStatus struct {
	// task
	TaskId string `json:"task_id" gorm:"index"`
	// syntaxflow result
	ResultId       uint   `json:"result_id" gorm:"index"`
	ResultVariable string `json:"result_variable"` // syntaxflow result variable name
	ResultAlertMsg string `json:"result_alert_msg"`
	// rule  info
	RuleName  string `json:"rule_name" gorm:"index"`
	RuleTitle string `json:"rule_title"`
	// program info
	ProgramName string `json:"program_name"`
}

type AuditNode struct {
	gorm.Model

	AuditNodeStatus
	// is entry node
	IsEntryNode bool `json:"is_entry_node"`
	// value
	IRCodeID    int64  `json:"ir_code_id"`
	VerboseName string `json:"verbose_name"`
}

type ResultVariable struct {
	Name     string `json:"result_variable"`
	Alert    string `json:"alert"`
	ValueNum int    `json:"num"`
}

func GetResultVariableByID(db *gorm.DB, resultID uint) ([]*ResultVariable, error) {
	// db = db.Debug()
	// get andit node by result_id, unique by result_variable, and get number of result_variable
	var items []*ResultVariable
	db = db.Model(&AuditNode{}).
		Where("result_id = ? and is_entry_node = ?", resultID, true).
		Select("result_variable, result_alert_msg, count(ir_code_id) as num").
		Group("result_variable").
		Order("created_at asc")
	row, err := db.Rows()
	if err != nil {
		return nil, err
	}

	for row.Next() {
		var item ResultVariable
		if err := row.Scan(&item.Name, &item.Alert, &item.ValueNum); err != nil {
			log.Errorf("scan failed: %s", err)
			continue
		}
		items = append(items, &item)
	}
	return items, nil
}

func GetResultValueByVariable(db *gorm.DB, resultID uint, resultVariable string) ([]int64, error) {
	// db = db.Debug()
	// get andit node by result_id, unique by result_variable, and get number of result_variable
	var items []int64
	if err := db.Model(&AuditNode{}).
		Where("result_id = ? and result_variable = ? and is_entry_node = ?", resultID, resultVariable, true).
		Pluck("ir_code_id", &items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func GetEffectOnEdgeByFromNodeId(id int64) []int64 {
	db := GetDB()
	var effectOns []int64
	db.Model(&AuditEdge{}).
		Where(" from_node = ? AND edge_type = ? ", id, EdgeType_EffectsOn).
		Pluck("to_node", &effectOns)
	return effectOns
}

func GetDependEdgeOnByFromNodeId(id int64) []int64 {
	db := GetDB()
	var dependOns []int64
	db.Model(&AuditEdge{}).
		Where("from_node =? AND edge_type = ? ", id, EdgeType_DependsOn).
		Pluck("to_node", &dependOns)
	return dependOns
}

func GetPredecessorEdgeByFromID(fromId int64) []*AuditEdge {
	db := GetDB()
	var edges []*AuditEdge
	db.Model(&AuditEdge{}).
		Where("from_node = ? AND edge_type = ?", fromId, EdgeType_Predecessor).Scan(&edges)
	return edges
}

func GetAuditNodeById(id int64) *AuditNode {
	db := GetDB()
	var an AuditNode
	err := db.Model(&AuditNode{}).Where("ir_code_id = ?", id).First(&an).Error
	if err != nil {
		return nil
	} else {
		return &an
	}
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
	// edge
	FromNode int64
	ToNode   int64

	// program
	ProgramName string `json:"program_name"`

	// type
	EdgeType AuditEdgeType

	// for predecessor
	AnalysisStep  int64
	AnalysisLabel string
}

func (n *AuditNode) CreateDependsOnEdge(progName string, to int64) *AuditEdge {
	ae := &AuditEdge{
		ProgramName: progName,
		FromNode:    int64(n.ID),
		ToNode:      to,
		EdgeType:    EdgeType_DependsOn,
	}
	return ae
}

func CreateEffectsOnEdge(progName string, from, to int64) *AuditEdge {
	ae := &AuditEdge{
		ProgramName: progName,
		FromNode:    from,
		ToNode:      to,
		EdgeType:    EdgeType_EffectsOn,
	}
	return ae
}

func CreateDependsOnEdge(progName string, from, to int64) *AuditEdge {
	ae := &AuditEdge{
		ProgramName: progName,
		FromNode:    from,
		ToNode:      to,
		EdgeType:    EdgeType_DependsOn,
	}
	return ae
}

func CreatePredecessorEdge(progName string, from, to, step int64, label string) *AuditEdge {
	ae := &AuditEdge{
		ProgramName:   progName,
		FromNode:      from,
		ToNode:        to,
		EdgeType:      EdgeType_Predecessor,
		AnalysisStep:  step,
		AnalysisLabel: label,
	}
	return ae
}

func YieldAuditNodeByResultId(DB *gorm.DB, resultId uint) chan *AuditNode {
	db := DB.Model(&AuditNode{}).Where("result_id = ?", resultId)
	return yieldAuditNode(db, context.Background())
}

func YieldAuditNodeByRuleName(DB *gorm.DB, ruleName string) chan *AuditNode {
	db := DB.Model(&AuditNode{}).Where("rule_name = ?", ruleName)
	return yieldAuditNode(db, context.Background())
}

func yieldAuditNode(DB *gorm.DB, ctx context.Context) chan *AuditNode {
	db := DB.Model(&AuditNode{}).Where("is_entry_node = true")
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
