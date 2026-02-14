package ssaapi

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"go.uber.org/atomic"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/samber/lo"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type Program struct {
	// TODO: one program may have multiple program,
	// 	 	 only one Application and multiple Library
	Program   *ssa.Program
	irProgram *ssadb.IrProgram
	// DBCache *ssa.Cache
	config         *Config
	enableDatabase bool
	// come from database will affect search operation
	comeFromDatabase bool
	//value cache
	nodeId2ValueCache  *utils.CacheWithKey[string, *Value]
	opcode2ValuesCache *utils.CacheWithKey[string, Values]
	funcReturnsCache   *utils.CacheWithKey[int64, []int64]
	id                 *atomic.Int64
	overlay            *ProgramOverLay
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

func (p *Program) GetProgramKind() ssadb.ProgramKind {
	return p.Program.ProgramKind
}

func (p *Program) GetLanguage() ssaconfig.Language {
	return p.Program.Language
}

// GetOverlay 获取增量编译的 ProgramOverLay（如果存在）
func (p *Program) GetOverlay() *ProgramOverLay {
	return p.overlay
}

// IsIncrementalCompile 判断这个 program 是否是增量编译的
// 如果 IsOverlay 为 true，或者有 BaseProgramName/FileHashMap，说明这个 program 属于增量编译流程的一部分
func (p *Program) IsIncrementalCompile() bool {
	if p == nil {
		return false
	}
	if p.irProgram != nil {
		// 使用 IsOverlay 或 BaseProgramName 来判断是否是增量编译
		return p.irProgram.IsOverlay || p.irProgram.BaseProgramName != "" || len(p.irProgram.FileHashMap) > 0
	}
	// 如果没有 irProgram，检查是否有 BaseProgramName 或 FileHashMap（向后兼容）
	if p.Program != nil {
		return p.Program.BaseProgramName != "" || len(p.Program.FileHashMap) > 0
	}
	return false
}

// IsBaseProgram 判断这个增量编译的 program 是否是 base program（第一个全量编译的 program）
// base program 是增量编译流程中的第一个 program，它的 BaseProgramName 为空
func (p *Program) IsBaseProgram() bool {
	if !p.IsIncrementalCompile() {
		return false
	}
	// base program 是增量编译的，但 BaseProgramName 为空
	if p.irProgram != nil {
		return p.irProgram.BaseProgramName == ""
	}
	if p.Program != nil {
		return p.Program.BaseProgramName == ""
	}
	return false
}

// GetBaseProgramName 获取基础程序名称（用于增量编译）
func (p *Program) GetBaseProgramName() string {
	if p == nil {
		return ""
	}
	if p.irProgram != nil && p.irProgram.BaseProgramName != "" {
		return p.irProgram.BaseProgramName
	}
	if p.Program != nil && p.Program.BaseProgramName != "" {
		return p.Program.BaseProgramName
	}
	return ""
}

func (p *Program) GetType(name string) *Type {
	typ := p.Program.GetType(name)
	if utils.IsNil(typ) {
		return nil
	}
	return NewType(typ)
}

func (p *Program) Hash() (string, bool) {
	if p.irProgram != nil {
		// Use the name and created_at to generate the hash,
		// So that the hash will be changed when the program is recompiled.
		hash := utils.CalcSha256(p.irProgram.ProgramName, p.irProgram.UpdatedAt.String())
		return hash, true
	} else if p.Program.Name != "" {
		return utils.CalcSha256(p.Program.Name), true
	} else {
		return "", false
	}
}

// GetConfig 获取编译配置（用于访问文件性能 recorder 等）
func (p *Program) GetConfig() *Config {
	if p == nil {
		return nil
	}
	return p.config
}

func NewProgram(prog *ssa.Program, config *Config) *Program {
	p := &Program{
		Program:            prog,
		nodeId2ValueCache:  utils.NewTTLCacheWithKey[string, *Value](8 * time.Second),
		opcode2ValuesCache: utils.NewLRUCacheWithKey[string, Values](16),
		funcReturnsCache:   utils.NewLRUCacheWithKey[int64, []int64](256),
		id:                 atomic.NewInt64(0),
	}
	if config != nil {
		p.config = config
		p.enableDatabase = config.databaseKind != ssa.ProgramCacheMemory
		if config.DiagnosticsEnabled() {
			prog.SetDiagnosticsRecorder(config.DiagnosticsRecorder())
		} else {
			prog.SetDiagnosticsRecorder(nil)
		}
		// 设置增量编译信息（如果存在）
		if baseProgramName := config.GetBaseProgramName(); baseProgramName != "" {
			prog.BaseProgramName = baseProgramName
		}
		if config.fileHashMap != nil && len(config.fileHashMap) > 0 {
			prog.FileHashMap = config.fileHashMap
		}
	} else {
		prog.SetDiagnosticsRecorder(nil)
	}
	return p
}

func NewTmpProgram(name string) *Program {
	p := &Program{
		Program:            ssa.NewTmpProgram(name),
		config:             &Config{},
		enableDatabase:     false,
		nodeId2ValueCache:  utils.NewTTLCacheWithKey[string, *Value](8 * time.Second),
		opcode2ValuesCache: utils.NewLRUCacheWithKey[string, Values](16),
		funcReturnsCache:   utils.NewLRUCacheWithKey[int64, []int64](256),
		id:                 atomic.NewInt64(0),
	}
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

func (p *Program) Ref(name string) Values {
	return p.refWithExcludeFiles(name, nil)
}

// refWithExcludeFiles 搜索变量，支持排除指定文件
// excludeFiles: 要排除的文件路径列表（规范化后的路径，如 "/test.go"）
func (p *Program) refWithExcludeFiles(name string, excludeFiles []string) Values {
	return lo.FilterMap(
		ssa.MatchInstructionsByVariableWithExcludeFiles(
			context.Background(), p.Program, ssadb.ExactCompare, ssadb.NameMatch, name, excludeFiles,
		),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, err := p.NewValue(i); err != nil {
				return nil, false
			} else {
				return v, true
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

func (v *Value) NewConstValue(i any, rng ...*memedit.Range) *Value {
	return v.ParentProgram.NewConstValue(i, rng...)
}

func (p *Program) NewConstValue(i any, rng ...*memedit.Range) *Value {
	value := ssa.NewConst(i)
	if len(rng) > 0 {
		value.SetRange(rng[0])
	}
	v, err := p.NewValue(value)
	_ = err // ignore error
	return v
}

// normal from ssa value
func (v *Value) NewValue(value ssa.Instruction) *Value {
	var iv *Value
	var err error
	iv, err = v.ParentProgram.NewValue(value)
	if err != nil {
		log.Errorf("NewValue: new value failed: %v", err)
		return nil
	}
	return iv
}

func (p *Program) NewValue(inst ssa.Instruction) (*Value, error) {
	if utils.IsNil(inst) {
		return nil, utils.Errorf("instruction is nil")
	}
	var v *Value
	var uuidStr string
	uuidStr = fmt.Sprintf("uuid-%d", p.id.Inc())
	v = &Value{
		runtimeCtx:    nil,
		ParentProgram: p,
		uuid:          uuidStr,
		EffectOn:      nil,
		DependOn:      nil,
	}

	// if lazy, get the real inst
	checkInst := inst
	// if inst.IsLazy() {
	// 	checkInst = inst.Self()
	// }
	if n, ok := checkInst.(ssa.Value); ok {
		v.innerValue = n
	}
	if n, ok := checkInst.(ssa.User); ok {
		v.innerUser = n
	}
	if v.innerValue == nil && v.innerUser == nil {
		str := inst.String()
		return nil, utils.Errorf("instruction is not a value or user: %s", str)
	}
	return v, nil
}

// from ssa id  (IrCode)
func (p *Program) GetValueById(id int64) (*Value, error) {
	val, ok := p.Program.GetInstructionById(id)
	if !ok || val == nil {
		return nil, utils.Errorf("instruction not found: %d", id)
	}
	v, err := p.NewValue(val)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (p *Program) GetValueByIdMust(id int64) *Value {
	v, err := p.GetValueById(id)
	if err != nil {
		log.Errorf("GetValueByIdMust: %v", err)
	}
	return v
}

// from audit node id
func (v *Value) NewValueFromAuditNode(nodeID string) *Value {
	value := v.ParentProgram.NewValueFromAuditNode(ssadb.GetDB(), nodeID)
	return value
}

func (p *Program) NewValueFromAuditNode(db *gorm.DB, nodeID string) *Value {
	if nodeID == "" {
		return nil
	}
	if db == nil {
		db = ssadb.GetDB()
	}

	// check cache
	if val, ok := p.nodeId2ValueCache.Get(nodeID); ok {
		return val
	}

	auditNode, err := ssadb.GetAuditNodeById(db, nodeID)
	if err != nil {
		log.Errorf("NewValueFromDB: audit node not found: %v", nodeID)
		return nil
	}
	// if auditNode is -1,check it.
	if auditNode.IRCodeID == -1 {
		var rangeIf *memedit.Range
		var memEditor *memedit.MemEditor
		if auditNode.TmpValueFileHash != "" {
			memEditor, err = ssadb.GetEditorByHash(auditNode.TmpValueFileHash)
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
		val := p.NewConstValue(auditNode.TmpValue, rangeIf)
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

func (p *Program) HasSavedDB() bool {
	return p.enableDatabase
}
