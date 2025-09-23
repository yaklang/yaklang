package ssadb

import (
	"context"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

type AuditNodeStatus struct {
	// task
	TaskId string `json:"task_id" gorm:"index"`
	// syntaxflow result
	ResultId       uint   `json:"result_id" gorm:"index"`
	ResultVariable string `json:"result_variable" gorm:"index"` // syntaxflow result variable name
	ResultIndex    uint   `json:"result_index" gorm:"index"`
	ResultAlertMsg string `json:"result_alert_msg"`
	RiskHash       string `json:"risk_hash"`
	// rule  info
	RuleName  string `json:"rule_name" gorm:"index"`
	RuleTitle string `json:"rule_title"`
	// program info
	ProgramName string `json:"program_name" gorm:"index"`
}

type AuditNode struct {
	gorm.Model

	AuditNodeStatus
	// is entry node
	IsEntryNode bool `json:"is_entry_node" gorm:"index"`
	// value
	IRCodeID int64 `json:"ir_code_id"`

	// if IrCodeId is -1, TmpCode  will be used
	TmpValue         string `json:"tmp_code"`
	TmpValueFileHash string `json:"tmp_value_file_hash"`
	TmpStartOffset   int    `json:"tmp_start_offset"`
	TmpEndOffset     int    `json:"tmp_end_offset"`

	VerboseName string `json:"verbose_name"`
}

type ResultVariable struct {
	Name     string `json:"result_variable"`
	HasRisk  bool   `json:"has_risk"`
	ValueNum int    `json:"num"`
}

func GetResultVariableByID(db *gorm.DB, resultID uint) ([]*ResultVariable, error) {
	// get andit node by result_id, unique by result_variable, and get number of result_variable
	var items []*ResultVariable
	db = db.Model(&AuditNode{}).
		Where("result_id = ? and is_entry_node = ?", resultID, true).
		Select("result_variable, MAX(risk_hash != '') as has_risk, count(ir_code_id) as num").
		Group("result_variable")
	row, err := db.Rows()
	if err != nil {
		return nil, err
	}
	defer row.Close()

	for row.Next() {
		var item ResultVariable
		// var tmp time.Time
		if err := row.Scan(&item.Name, &item.HasRisk, &item.ValueNum); err != nil {
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

func GetResultNodeByVariableIndex(db *gorm.DB, resultID uint, resultVariable string, resultIndex uint) (uint, error) {
	var node AuditNode
	if err := db.Model(&AuditNode{}).
		Where("result_id = ? and result_variable = ? and result_index = ? and is_entry_node = true ",
			resultID, resultVariable, resultIndex).First(&node).Error; err != nil {
		return 0, err
	}
	return uint(node.ID), nil
}

func GetResultNodeByVariable(db *gorm.DB, resultID uint, resultVariable string) ([]uint, error) {
	// db = db.Debug()
	// get andit node by result_id, unique by result_variable, and get number of result_variable
	var items []uint
	if err := db.Model(&AuditNode{}).Order("result_index ASC, id ASC").
		Where("result_id = ? and result_variable = ? and is_entry_node = true", resultID, resultVariable).
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

func GetDataFlowEdgeByToNodeId(fromId uint) []uint {
	db := GetDB()
	var edges []uint
	db.Model(&AuditEdge{}).
		Where("from_node = ? AND edge_type = ?", fromId, EdgeType_DataFlow).
		Pluck("to_node", &edges)
	return edges
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
	EdgeType_DataFlow  AuditEdgeType = "prev_dataflow"

	// EdgeType_Predecessor 记录审计过程
	EdgeType_Predecessor AuditEdgeType = "predecessor"
)

type AuditEdge struct {
	gorm.Model

	// task
	TaskId string `json:"task_id" gorm:"index"`
	// syntaxflow result
	ResultId uint `json:"result_id" gorm:"index"`

	// edge
	FromNode uint `json:"from_node" gorm:"index"`
	ToNode   uint `json:"to_node" gorm:"index"`

	// program
	ProgramName string `json:"program_name" gorm:"index"`

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
		TaskId:      n.TaskId,
		ResultId:    n.ResultId,
	}
	return ae
}

func (n *AuditNode) CreateEffectsOnEdge(progName string, to uint) *AuditEdge {
	ae := &AuditEdge{
		ProgramName: progName,
		FromNode:    n.ID,
		ToNode:      to,
		EdgeType:    EdgeType_EffectsOn,
		TaskId:      n.TaskId,
		ResultId:    n.ResultId,
	}
	return ae
}

func (n *AuditNode) CreateDataFlowEdge(progName string, to uint) *AuditEdge {
	ae := &AuditEdge{
		ProgramName: progName,
		FromNode:    to,
		ToNode:      n.ID,
		EdgeType:    EdgeType_DataFlow,
		TaskId:      n.TaskId,
		ResultId:    n.ResultId,
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
		TaskId:        n.TaskId,
		ResultId:      n.ResultId,
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
	return bizhelper.YieldModel[*AuditNode](ctx, db)
}

func DeleteAuditNode(DB *gorm.DB, nodes ...*AuditNode) error {
	if len(nodes) == 0 {
		return utils.Errorf("delete type from database id is empty")
	}
	id := lo.Map(nodes, func(item *AuditNode, _ int) uint {
		return item.ID
	})
	return utils.GormTransaction(DB, func(tx *gorm.DB) error {
		// split each 999
		for i := 0; i < len(id); i += 999 {
			end := i + 999
			if end > len(id) {
				end = len(id)
			}
			tx.Where("id IN (?)", id[i:end]).Unscoped().Delete(&IrCode{})
		}
		return nil
	})
}
