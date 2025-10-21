package ssadb

import (
	"context"
	"math/rand"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/oklog/ulid"
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
	// ResultAlertMsg string `json:"result_alert_msg"`
	RiskHash string `json:"risk_hash"`
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

	NodeID string `json:"node_id" gorm:"index;type:char(26)"`

	// if IrCodeId is -1, TmpCode  will be used
	TmpValue         string `json:"tmp_code"`
	TmpValueFileHash string `json:"tmp_value_file_hash"`
	TmpStartOffset   int    `json:"tmp_start_offset"`
	TmpEndOffset     int    `json:"tmp_end_offset"`

	VerboseName string `json:"verbose_name"`
}

// 你需要一个熵源 (entropy source)
// 在你的应用启动时初始化一次即可
var ulidEntropy = ulid.Monotonic(rand.New(rand.NewSource(time.Now().UnixNano())), 0)

// 生成一个新的 ULID 字符串的辅助函数
func NewULID() string {
	// Monotonic 确保了即使在同一毫秒内快速连续调用，ID 也是递增的
	id := ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)
	return id.String()
}

func NewAuditNode() *AuditNode {
	return &AuditNode{
		NodeID: NewULID(),
	}
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

func GetResultNodeByVariableIndex(db *gorm.DB, resultID uint, resultVariable string, resultIndex uint) (string, error) {
	var node AuditNode
	if err := db.Model(&AuditNode{}).
		Where("result_id = ? and result_variable = ? and result_index = ? and is_entry_node = true ",
			resultID, resultVariable, resultIndex).First(&node).Error; err != nil {
		return "", err
	}
	return node.NodeID, nil
}

func GetResultNodeByVariable(db *gorm.DB, resultID uint, resultVariable string) ([]string, error) {
	// db = db.Debug()
	// get andit node by result_id, unique by result_variable, and get number of result_variable
	var items []string
	if err := db.Model(&AuditNode{}).Order("result_index ASC, id ASC").
		Where("result_id = ? and result_variable = ? and is_entry_node = true", resultID, resultVariable).
		Pluck("node_id", &items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func GetResultNodeByRiskHash(db *gorm.DB, riskHash string) ([]string, error) {
	var items []string
	if err := db.Model(&AuditNode{}).
		Where("risk_hash = ? and is_entry_node = true", riskHash).
		Pluck("node_id", &items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func GetEffectOnEdgeByFromNodeId(id string) []string {
	db := GetDB()
	var effectOns []string
	db.Model(&AuditEdge{}).
		Where(" from_node = ? AND edge_type = ? ", id, EdgeType_EffectsOn).
		Pluck("to_node", &effectOns)
	return effectOns
}

func GetDependEdgeOnByFromNodeId(id string) []string {
	db := GetDB()
	var dependOns []string
	db.Model(&AuditEdge{}).
		Where("from_node =? AND edge_type = ? ", id, EdgeType_DependsOn).
		Pluck("to_node", &dependOns)
	return dependOns
}

func GetDataFlowEdgeByToNodeId(fromId uint) []string {
	db := GetDB()
	var edges []string
	db.Model(&AuditEdge{}).
		Where("from_node = ? AND edge_type = ?", fromId, EdgeType_DataFlow).
		Pluck("to_node", &edges)
	return edges
}

func GetPredecessorEdgeByFromID(fromId string) []*AuditEdge {
	db := GetDB()
	var edges []*AuditEdge
	db.Model(&AuditEdge{}).
		Where("from_node = ? AND edge_type = ?", fromId, EdgeType_Predecessor).
		Scan(&edges)
	return edges
}

func GetAuditNodeById(id string) (*AuditNode, error) {
	db := GetDB()
	var an AuditNode
	if err := db.Model(&AuditNode{}).Where("node_id = ?", id).First(&an).Error; err != nil {
		return nil, err
	} else {
		return &an, nil
	}
}

func GetAuditNodesByIds(ids []string) ([]*AuditNode, error) {
	db := GetDB()
	var ans []*AuditNode
	if err := db.Model(&AuditNode{}).Where("node_id IN (?)", ids).Find(&ans).Error; err != nil {
		return nil, err
	} else {
		return ans, nil
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

func ValidEdgeType(edgeType string) AuditEdgeType {
	switch edgeType {
	case string(EdgeType_DependsOn):
		return EdgeType_DependsOn
	case string(EdgeType_EffectsOn):
		return EdgeType_EffectsOn
	case string(EdgeType_DataFlow):
		return EdgeType_DataFlow
	default:
		return EdgeType_Predecessor
	}
}

type AuditEdge struct {
	gorm.Model

	// task
	TaskId string `json:"task_id" gorm:"index"`
	// syntaxflow result
	ResultId uint `json:"result_id" gorm:"index"`

	// edge
	FromNode string `json:"from_node" gorm:"index;type:char(26)"`
	ToNode   string `json:"to_node" gorm:"index;type:char(26)"`

	// program
	ProgramName string `json:"program_name" gorm:"index"`

	// type
	EdgeType AuditEdgeType `json:"edge_type" gorm:"index"`

	// for predecessor
	AnalysisStep  int64
	AnalysisLabel string
}

func (n *AuditNode) CreateDependsOnEdge(progName string, to *AuditNode) *AuditEdge {
	ae := &AuditEdge{
		ProgramName: progName,
		FromNode:    n.NodeID,
		ToNode:      to.NodeID,
		EdgeType:    EdgeType_DependsOn,
		TaskId:      n.TaskId,
		ResultId:    n.ResultId,
	}
	return ae
}

func (n *AuditNode) CreateEffectsOnEdge(progName string, to *AuditNode) *AuditEdge {
	ae := &AuditEdge{
		ProgramName: progName,
		FromNode:    n.NodeID,
		ToNode:      to.NodeID,
		EdgeType:    EdgeType_EffectsOn,
		TaskId:      n.TaskId,
		ResultId:    n.ResultId,
	}
	return ae
}

func (n *AuditNode) CreatePredecessorEdge(progName string, to *AuditNode, step int64, label string) *AuditEdge {
	ae := &AuditEdge{
		ProgramName:   progName,
		FromNode:      n.NodeID,
		ToNode:        to.NodeID,
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
			tx.Where("id IN (?)", id[i:end]).Unscoped().Delete(&AuditNode{})
		}
		return nil
	})
}
