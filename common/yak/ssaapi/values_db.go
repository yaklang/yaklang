package ssaapi

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type saveValueCtx struct {
	db        *gorm.DB
	visited   map[string]struct{}
	runtimeId string
	ruleId    int
	ruleName  string
}

func recursiveSaveValue(value *Value, s *saveValueCtx, callback func(next *ssadb.AuditNode) error) error {
	if s == nil {
		return utils.Error("saveValueCtx is nil")
	}

	if value == nil {
		return nil
	}

	p := value.GetProgramName()
	if p == "" {
		return utils.Error("program name is empty")
	}

	var id string
	idInt := value.GetId()
	if idInt <= 0 {
		id = codec.Sha256(value.String())
	} else {
		id = codec.Sha256(idInt)
	}

	if _, ok := s.visited[id]; ok {
		return nil
	}
	s.visited[id] = struct{}{}

	an := &ssadb.AuditNode{
		RuntimeId:   s.runtimeId,
		RuleName:    s.ruleName,
		RuleId:      int64(s.ruleId),
		ProgramName: p,
		SsaId:       value.GetId(),
		ConstValue:  codec.AnyToString(value.GetConstValue()),
		IsEntryNode: callback == nil,
	}
	if ret := s.db.Save(an).Error; ret != nil {
		return utils.Wrap(ret, "save AuditNode")
	}

	if callback != nil {
		if err := callback(an); err != nil {
			return utils.Wrap(err, "callback failed")
		}
	}

	for _, i := range value.DependOn {
		if err := recursiveSaveValue(i, s, func(next *ssadb.AuditNode) error {
			edge := an.CreateDependsOnEdge(int64(next.ID))
			if ret := s.db.Save(edge).Error; ret != nil {
				return utils.Wrap(ret, "save AuditEdge")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	for _, i := range value.EffectOn {
		if err := recursiveSaveValue(i, s, func(next *ssadb.AuditNode) error {
			edge := an.CreateEffectsOnEdge(int64(next.ID))
			if ret := s.db.Save(edge).Error; ret != nil {
				return utils.Wrap(ret, "save AuditEdge")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	for _, pred := range value.Predecessors {
		if err := recursiveSaveValue(pred.Node, s, func(next *ssadb.AuditNode) error {
			var step int64
			var label string
			if info := pred.Info; info != nil {
				step = int64(info.Step)
				label = info.Label
			}
			edge := an.CreatePredecessorEdge(int64(next.ID), step, label)
			if ret := s.db.Save(edge).Error; ret != nil {
				return utils.Wrap(ret, "save AuditEdge")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

type SaveValueOption func(c *saveValueCtx)

func OptionSaveValue_RuntimeId(runtimeId string) SaveValueOption {
	return func(c *saveValueCtx) {
		c.runtimeId = runtimeId
	}
}

func OptionSaveValue_RuleId(i any) SaveValueOption {
	return func(c *saveValueCtx) {
		c.ruleId = utils.InterfaceToInt(i)
	}
}

func OptionSaveValue_RuleName(name string) SaveValueOption {
	return func(c *saveValueCtx) {
		c.ruleName = name
	}
}

func SaveValue(value *Value, opts ...SaveValueOption) error {
	db := ssadb.GetDB()
	if db == nil {
		return utils.Error("db is nil")
	}
	ctx := &saveValueCtx{
		db:      db,
		visited: make(map[string]struct{}),
	}
	for _, o := range opts {
		o(ctx)
	}
	return recursiveSaveValue(value, ctx, nil)
}
