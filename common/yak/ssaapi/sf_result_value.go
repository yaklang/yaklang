package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// ======================================== All Value/Variable ========================================

func (r *SyntaxFlowResult) GetVariableNum() int {
	if r == nil {
		return 0
	}
	if r.variable != nil {
		return r.variable.Len()
	}
	r.GetAllVariable()
	return r.variable.Len()
}

func (r *SyntaxFlowResult) GetAllVariable() *orderedmap.OrderedMap {
	if r == nil {
		return nil
	}
	if r.variable != nil {
		return r.variable
	}

	r.variable = orderedmap.New()
	if r.memResult != nil {
		r.memResult.SymbolTable.ForEach(func(name string, value sfvm.ValueOperator) bool {
			r.variable.Set(name, sfvm.ValuesLen(value))
			return true
		})
		r.memResult.AlertSymbolTable.ForEach(func(key string, value sfvm.ValueOperator) bool {
			if v, ok := r.variable.Get(key); ok && v.(int) > 0 {
				r.alertVariable = append(r.alertVariable, key)
			}
			return true
		})
	}

	if r.dbResult != nil {
		res, err := ssadb.GetResultVariableByID(ssadb.GetDB(), r.GetResultID())
		if err != nil {
			log.Errorf("err: %v", err)
			return nil
		}
		for _, v := range res {
			if v.Name == "_" {
				continue
			}
			r.variable.Set(v.Name, int(v.ValueNum))
			if v.HasRisk {
				r.alertVariable = append(r.alertVariable, v.Name)
			}
		}
		for _, name := range r.dbResult.UnValueVariable {
			if _, ok := r.variable.Get(name); !ok {
				r.variable.Set(name, 0)
			}
		}
	}

	return r.variable
}

func (r *SyntaxFlowResult) GetAllValuesChain() Values {
	var results Values
	m := r.GetAllVariable()
	if m == nil {
		return nil
	}
	m.ForEach(func(name string, value any) {
		vs := r.GetValues(name)
		results = append(results, vs...)
	})
	if len(results) == 0 {
		results = append(results, r.GetUnNameValues()...)
	}
	return results
}

func (r *SyntaxFlowResult) GetValueCount(name string) int {
	if r == nil {
		return 0
	}

	if r.variable == nil {
		r.GetAllVariable()
	}
	if v, ok := r.variable.Get(name); ok {
		if ret, ok := v.(int); ok {
			return ret
		}
	} else if name == "_" {
		return r.GetUnNameValues().Len()
	}
	return 0
}

// ======================================== Single Value ========================================

// Normal value
func (r *SyntaxFlowResult) GetValues(name string) Values {
	if r == nil {
		return nil
	}
	// unname
	if name == "_" {
		return r.GetUnNameValues()
	}
	// cache
	if vs, ok := r.symbol[name]; ok {
		return vs
	}

	// memory
	if r.memResult != nil {
		if v, ok := r.memResult.SymbolTable.Get(name); ok {
			vs := SyntaxFlowVariableToValues(v)
			r.symbol[name] = vs
			return vs
		}
	}
	if r.dbResult != nil {
		vs := r.getValueFromDB(name)
		r.symbol[name] = vs
		return vs
	}
	return nil
}

func (r *SyntaxFlowResult) GetValue(name string, index int) (*Value, error) {
	if r == nil {
		return nil, utils.Errorf("result is nil")
	}

	if name == "_" {
		return r.GetUnNameValues()[index], nil
	}

	if r.dbResult != nil {
		// for new DB data  have index
		id, err := ssadb.GetResultNodeByVariableIndex(ssadb.GetDB(), r.GetResultID(), name, uint(index))
		if err == nil {
			if r.program != nil {
				return r.program.NewValueFromAuditNode(id), nil
			} else {
				// 内存编译的program为空
				return r.getValueFromTmpAuditNode(id), nil
			}
		}
	}
	// the old DB data and memory data can get by this
	vs := r.GetValues(name)
	if len(vs) > int(index) {
		return vs[index], nil
	} else {
		return nil, utils.Errorf("index out of range")
	}
}

// Alert value
func (r *SyntaxFlowResult) GetAlertVariables() []string {
	if r == nil {
		return nil
	}
	if r.alertVariable == nil {
		r.GetAllVariable()
	}
	return r.alertVariable
}

// UnName value
func (r *SyntaxFlowResult) GetUnNameValues() Values {
	if r == nil {
		return nil
	}
	if r.unName != nil {
		return r.unName
	}
	if r.memResult != nil {
		// memory
		r.unName = SyntaxFlowVariableToValues(r.memResult.UnNameValue)
	} else if r.dbResult != nil {
		// database
		r.unName = r.getValueFromDB("_")
	}
	return r.unName
}

func (r *SyntaxFlowResult) GetResultSaveKind() ResultSaveKind {
	if r == nil {
		return resultSaveNone
	}
	return r.saveKind
}

func (r *SyntaxFlowResult) SetResultID(id uint) {
	if r == nil {
		return
	}
	r.id = id
}

func (r *SyntaxFlowResult) GetResultID() uint {
	if r == nil {
		return 0
	}
	if r.id != 0 {
		return r.id
	}
	if r.dbResult != nil {
		return r.dbResult.ID
	}
	return 0
}

func (r *SyntaxFlowResult) getValueFromDB(name string) Values {
	auditNodeIDs, err := ssadb.GetResultNodeByVariable(ssadb.GetDB(), r.GetResultID(), name)
	if err != nil {
		return nil
	}

	vs := make(Values, 0, len(auditNodeIDs))
	if r.program != nil {
		for _, nodeID := range auditNodeIDs {
			v := r.program.NewValueFromAuditNode(nodeID)
			if v != nil {
				vs = append(vs, v)
			}
		}
	} else {
		// 内存编译的时候program为空
		vs = r.getValuesFromTmpAuditNodes(auditNodeIDs)
	}
	return vs
}

func (r *SyntaxFlowResult) getValuesFromTmpAuditNodes(nodeIds []uint) Values {
	auditNodes, err := ssadb.GetAuditNodesByIds(nodeIds)
	if err != nil {
		log.Errorf("NewValueFromDB: audit node not found: %v", nodeIds)
		return nil
	}

	program := NewTmpProgram(r.Name())
	vs := make(Values, 0, len(auditNodes))
	for _, auditNode := range auditNodes {
		var rangeIf *memedit.Range
		var memEditor *memedit.MemEditor
		if auditNode.TmpValueFileHash != "" {
			memEditor, err = ssadb.GetIrSourceFromHash(auditNode.TmpValueFileHash)
			if err != nil {
				log.Errorf("NewValueFromDB: get ir source from hash failed: %v", err)
			} else {
				if auditNode.TmpStartOffset == -1 || auditNode.TmpEndOffset == -1 {
					rangeIf = memEditor.GetRangeOffset(0, memEditor.CodeLength())
				} else {
					rangeIf = memEditor.GetRangeOffset(auditNode.TmpStartOffset, auditNode.TmpEndOffset)
				}
			}
		}
		val := program.NewConstValue(auditNode.TmpValue, rangeIf)
		val.auditNode = auditNode
		vs = append(vs, val)
	}
	return vs
}

func (r *SyntaxFlowResult) getValueFromTmpAuditNode(nodeId uint) *Value {
	auditNode, err := ssadb.GetAuditNodeById(nodeId)
	if err != nil {
		log.Errorf("NewValueFromDB: audit node not found: %d", nodeId)
		return nil
	}

	program := NewTmpProgram(r.Name())
	var rangeIf *memedit.Range
	var memEditor *memedit.MemEditor
	if auditNode.TmpValueFileHash != "" {
		memEditor, err = ssadb.GetIrSourceFromHash(auditNode.TmpValueFileHash)
		if err != nil {
			log.Errorf("NewValueFromDB: get ir source from hash failed: %v", err)
		} else {
			if auditNode.TmpStartOffset == -1 || auditNode.TmpEndOffset == -1 {
				rangeIf = memEditor.GetRangeOffset(0, memEditor.CodeLength())
			} else {
				rangeIf = memEditor.GetRangeOffset(auditNode.TmpStartOffset, auditNode.TmpEndOffset)
			}
		}
	}
	val := program.NewConstValue(auditNode.TmpValue, rangeIf)
	val.auditNode = auditNode
	return val
}
