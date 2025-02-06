package ssaapi

import (
	"context"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"sort"
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type Program struct {
	// TODO: one program may have multiple program,
	// 	 	 only one Application and multiple Library
	Program    *ssa.Program
	ssaProgram *schema.SSAProgram
	// DBCache *ssa.Cache
	config *config

	enableDatabase bool
	// come from database will affect search operation
	comeFromDatabase bool
	//value cache
	nodeId2ValueCache *utils.CacheWithKey[uint, *Value]
}

type Programs []*Program

func (p *Program) IsFromDatabase() bool {
	return p.comeFromDatabase
}

func (p *Program) GetProgramName() string {
	if p == nil || p.Program == nil {
		return ""
	}
	return p.Program.Name
}

func (p *Program) GetProgramKind() ssa.ProgramKind {
	return p.Program.ProgramKind
}

func (p *Program) GetLanguage() string {
	return p.Program.Language
}

func (p *Program) GetType(name string) *Type {
	typ := p.Program.GetType(name)
	if utils.IsNil(typ) {
		return nil
	}
	return NewType(typ)
}

func (p *Program) Hash() (string, bool) {
	if p.ssaProgram != nil {
		// Use the name and created_at to generate the hash,
		// So that the hash will be changed when the program is recompiled.
		hash := utils.CalcSha256(p.ssaProgram.Name, p.ssaProgram.CreatedAt.String())
		return hash, true
	} else if p.Program.Name != "" {
		return utils.CalcSha256(p.Program.Name), true
	} else {
		return "", false
	}
}

func NewProgram(prog *ssa.Program, config *config) *Program {
	p := &Program{
		Program:           prog,
		config:            config,
		enableDatabase:    config.ProgramName != "",
		nodeId2ValueCache: utils.NewTTLCacheWithKey[uint, *Value](8 * time.Second),
	}

	// if config.DatabaseProgramName == "" {
	// 	p.DBCache = prog.Cache
	// } else {
	// 	p.DBCache = ssa.GetCacheFromPool(config.DatabaseProgramName)
	// }
	return p
}

func (p *Program) DBDebug() {
	if p == nil || p.Program == nil {
		return
	}
	p.Program.Cache.DB = p.Program.Cache.DB.Debug()
}

func (p *Program) Show() *Program {
	p.Program.Show()
	return p
}

func (p Programs) Show() Programs {
	for _, prog := range p {
		prog.Show()
	}
	return p
}

func (p *Program) IsNil() bool {
	return utils.IsNil(p) || utils.IsNil(p.Program)
}

func (p *Program) GetErrors() ssa.SSAErrors {
	return p.Program.GetErrors()
}

func (p *Program) GetInstructionById(id int64) ssa.Instruction {
	return p.Program.GetInstructionById(id)
}

func (p *Program) Ref(name string) Values {
	return lo.FilterMap(
		ssa.MatchInstructionByExact(
			context.Background(), p.Program, ssadb.NameMatch, name,
		),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, ok := i.(ssa.Value); ok {
				return p.NewValue(v), true
			} else {
				return nil, false
			}
		},
	)
}

func (p *Program) GetAllOffsetItemsBefore(offset int) []*ssa.OffsetItem {
	offsetSortedSlice := p.Program.OffsetSortedSlice
	index := sort.SearchInts(offsetSortedSlice, offset)
	if index < len(offsetSortedSlice) && offsetSortedSlice[index] > offset && index > 0 {
		index--
	}
	beforeSlice := offsetSortedSlice[:index]

	return lo.Filter(
		lo.Map(beforeSlice, func(offset int, _ int) *ssa.OffsetItem {
			return p.Program.OffsetMap[offset]
		}),
		func(v *ssa.OffsetItem, _ int) bool {
			return v.GetVariable() != nil
		},
	)
}

func (v *Value) NewTopDefValue(value ssa.Value) *Value {
	return v.NewValue(value).AppendEffectOn(v)
}

func (v *Value) NewBottomUseValue(value ssa.Value) *Value {
	return v.NewValue(value).AppendDependOn(v)
}

// normal from ssa value
func (v *Value) NewValue(value ssa.Value) *Value {
	return v.ParentProgram.NewValue(value)
}
func (p *Program) NewValue(n ssa.Value) *Value {
	if utils.IsNil(n) {
		return nil
	}
	v := &Value{
		runtimeCtx:    omap.NewEmptyOrderedMap[ContextID, *Value](),
		node:          n,
		ParentProgram: p,
	}
	return v
}

// from ssa id  (IrCode)
func (p *Program) GetValueById(id int64) (*Value, error) {
	val, ok := p.Program.GetInstructionById(id).(ssa.Value)
	if val == nil {
		return nil, utils.Errorf("instruction not found: %d", id)
	}
	if !ok {
		return nil, utils.Errorf("[%T] not an instruction node", val)
	}
	ret := p.NewValue(val)
	return ret, nil
}

func (p *Program) GetValueByIdMust(id int64) *Value {
	v, err := p.GetValueById(id)
	if err != nil {
		log.Errorf("GetValueByIdMust: %v", err)
	}
	return v
}

// from audit node id
func (v *Value) NewValueFromAuditNode(nodeID uint) *Value {
	value := v.ParentProgram.NewValueFromAuditNode(nodeID)
	return value
}

func (p *Program) NewValueFromAuditNode(nodeID uint) *Value {
	if nodeID == 0 {
		return nil
	}

	// check cache
	if val, ok := p.nodeId2ValueCache.Get(nodeID); ok {
		return val
	}

	auditNode, err := ssadb.GetAuditNodeById(nodeID)
	if err != nil {
		log.Errorf("NewValueFromDB: audit node not found: %d", nodeID)
		return nil
	}
	// if auditNode is -1,check it.
	if auditNode.IRCodeID == -1 {
		var offset memedit.RangeIf
		var memEditor *memedit.MemEditor
		if auditNode.TmpValueFileHash == "" {
			memEditor = memedit.NewMemEditor(auditNode.TmpValue)
		} else {
			memEditor, err = ssadb.GetIrSourceFromHash(auditNode.TmpValueFileHash)
			if err != nil {
				log.Errorf("NewValueFromDB: get ir source from hash failed: %v", err)
				return nil
			}
		}
		if auditNode.TmpStartOffset == -1 || auditNode.TmpEndOffset == -1 {
			offset = memEditor.GetRangeOffset(0, memEditor.CodeLength())
		} else {
			offset = memEditor.GetRangeOffset(auditNode.TmpStartOffset, auditNode.TmpEndOffset)
		}
		val := p.NewValue(ssa.NewConstWithRange(auditNode.TmpValue, offset))
		val.auditNode = auditNode
		return val
	}
	val, err := p.GetValueById(auditNode.IRCodeID)
	if err != nil {
		log.Errorf("NewValueFromDB: get value by id failed: %v", err)
		return nil
	}
	val.auditNode = auditNode

	// save cache
	p.nodeId2ValueCache.Set(nodeID, val)

	return val
}
