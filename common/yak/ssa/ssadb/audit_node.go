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
	// get andit node by result_id, unique by result_variable, and get number of result_variable
	var items []*ResultVariable
	db = db.Model(&AuditNode{}).
		Where("result_id = ? and is_entry_node = ?", resultID, true).
		Select("result_variable, result_alert_msg, count(ir_code_id) as num").
		Group("result_variable, result_alert_msg")
	row, err := db.Rows()
	if err != nil {
		return nil, err
	}

	for row.Next() {
		var item ResultVariable
		// var tmp time.Time
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

func GetResultNodeByVariable(db *gorm.DB, resultID uint, resultVariable string) ([]uint, error) {
	// db = db.Debug()
	// get andit node by result_id, unique by result_variable, and get number of result_variable
	var items []uint
	if err := db.Model(&AuditNode{}).
		Where("result_id = ? and result_variable = ? and is_entry_node = ?", resultID, resultVariable, true).
		Pluck("id", &items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func GetEffectOnEdgeByFromNodeId(id uint) []uint {
	db := GetDB()
	var effectOns []uint
	db.Model(&AuditEdge{}).
		Where(" from_node = ? AND edge_type = ? ", id, EdgeType_EffectsOn).
		Pluck("to_node", &effectOns)
	return effectOns
}

func GetDependEdgeOnByFromNodeId(id uint) []uint {
	db := GetDB()
	var dependOns []uint
	db.Model(&AuditEdge{}).
		Where("from_node =? AND edge_type = ? ", id, EdgeType_DependsOn).
		Pluck("to_node", &dependOns)
	return dependOns
}

func GetPredecessorEdgeByFromID(fromId uint) []*AuditEdge {
	db := GetDB()
	var edges []*AuditEdge
	db.Model(&AuditEdge{}).
		Where("from_node = ? AND edge_type = ?", fromId, EdgeType_Predecessor).
		Scan(&edges)
	return edges
}

func GetAuditNodeById(id uint) (*AuditNode, error) {
	db := GetDB()
	var an AuditNode
	if err := db.Model(&AuditNode{}).Where("id = ?", id).First(&an).Error; err != nil {
		return nil, err
	} else {
		return &an, nil
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
	FromNode uint `json:"from_node" gorm:"index"`
	ToNode   uint `json:"to_node" gorm:"index"`

	// program
	ProgramName string `json:"program_name"`

	// type
	EdgeType AuditEdgeType `json:"edge_type" gorm:"index"`

	// for predecessor
	AnalysisStep  int64
	AnalysisLabel string
}

func (n *AuditNode) CreateDependsOnEdge(progName string, to uint) *AuditEdge {
	ae := &AuditEdge{
		ProgramName: progName,
		FromNode:    n.ID,
		ToNode:      to,
		EdgeType:    EdgeType_DependsOn,
	}
	return ae
}

func (n *AuditNode) CreateEffectsOnEdge(progName string, to uint) *AuditEdge {
	ae := &AuditEdge{
		ProgramName: progName,
		FromNode:    n.ID,
		ToNode:      to,
		EdgeType:    EdgeType_EffectsOn,
	}
	return ae
}
func (n *AuditNode) CreatePredecessorEdge(progName string, to uint, step int64, label string) *AuditEdge {
	ae := &AuditEdge{
		ProgramName:   progName,
		FromNode:      n.ID,
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

		paginator := bizhelper.NewFastPaginator(db, 100)
		for {
			var items []*AuditNode
			if err, ok := paginator.Next(&items); !ok {
				break
			} else if err != nil {
				log.Errorf("paging failed: %s", err)
				continue
			}

			for _, d := range items {
				select {
				case <-ctx.Done():
					return
				case outC <- d:
				}
			}
		}
	}()
	return outC
}
