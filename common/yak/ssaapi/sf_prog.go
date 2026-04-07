package ssaapi

import (
	"context"
	"fmt"
	"regexp"

	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/utils/memedit"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/gobwas/glob"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

var _ sfvm.ValueOperator = &Program{}

func (p *Program) CompareConst(comparator *sfvm.ConstComparator) bool {
	return false
}

func (p *Program) ShouldUseConditionCandidate() bool {
	return true
}

func (p *Program) NewConst(i any, rng ...*memedit.Range) sfvm.ValueOperator {
	return p.NewConstValue(i, rng...)
}

func (p *Program) CompareOpcode(opcodeItems *sfvm.OpcodeComparator) (sfvm.Values, []bool) {
	ctx := opcodeItems.Context
	var res Values = lo.FilterMap(
		ssa.MatchInstructionByOpcodes(ctx, p.Program, opcodeItems.Opcodes...),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			val, err := p.NewValue(i)
			if err != nil {
				log.Errorf("CompareOpcode: new value failed: %v", err)
				return val, false
			}
			return val, true
		},
	)
	// Return matched values; VM will normalize bool mask against source width.
	return ToSFVMValues(res), nil
}

func (p *Program) CompareString(comparator *sfvm.StringComparator) (sfvm.Values, []bool) {
	var res []sfvm.ValueOperator
	ctx := comparator.Context

	matchCallByString := func(condition *sfvm.StringCondition) sfvm.Values {
		callMatcher := sfvm.NewStringComparator(sfvm.MatchHave, ctx)
		callMatcher.Conditions = []*sfvm.StringCondition{condition}
		var out []sfvm.ValueOperator
		for _, inst := range ssa.MatchInstructionByOpcodes(ctx, p.Program, ssa.SSAOpcodeCall) {
			val, err := p.NewValue(inst)
			if err != nil || val == nil {
				continue
			}
			names := getValueNames(val)
			names = append(names, codec.AnyToString(val.String()))
			if callMatcher.Matches(names...) {
				out = append(out, val)
			}
		}
		return sfvm.NewValues(out)
	}
	matchConstByString := func(condition *sfvm.StringCondition) sfvm.Values {
		matchMode := ssadb.ConstType
		switch condition.FilterMode {
		case sfvm.GlobalConditionFilter:
			_, out, _ := p.GlobMatch(ctx, matchMode, condition.Pattern)
			return out
		case sfvm.RegexpConditionFilter:
			_, out, _ := p.RegexpMatch(ctx, matchMode, condition.Pattern)
			return out
		case sfvm.ExactConditionFilter:
			_, out, _ := p.RegexpMatch(ctx, matchMode, fmt.Sprintf(".*%s.*", regexp.QuoteMeta(condition.Pattern)))
			return out
		default:
			return sfvm.NewEmptyValues()
		}
	}

	matchValue := func(condition *sfvm.StringCondition) sfvm.Values {
		var v sfvm.Values
		matchMode := ssadb.NameMatch
		switch condition.FilterMode {
		case sfvm.GlobalConditionFilter:
			_, v, _ = p.GlobMatch(ctx, matchMode, condition.Pattern)
		case sfvm.RegexpConditionFilter:
			_, v, _ = p.RegexpMatch(ctx, matchMode, condition.Pattern)
		case sfvm.ExactConditionFilter:
			_, v, _ = p.RegexpMatch(ctx, matchMode, fmt.Sprintf(".*%s.*", regexp.QuoteMeta(condition.Pattern)))
		}
		callMatches := matchCallByString(condition)
		constMatches := matchConstByString(condition)
		if v.IsEmpty() {
			return sfvm.MergeValues(callMatches, constMatches)
		}
		return sfvm.MergeValues(v, callMatches, constMatches)
	}

	switch comparator.MatchMode {
	case sfvm.MatchHave:
		set := sfvm.NewValueSet()
		for i, condition := range comparator.Conditions {
			matched := matchValue(condition)
			if matched == nil {
				continue
			}
			otherSet := sfvm.NewValueSet()
			matched.Recursive(func(vo sfvm.ValueOperator) error {
				if ret, ok := vo.(ssa.GetIdIF); ok {
					id := ret.GetId()
					if i == 0 {
						set.Add(id, vo)
					} else {
						otherSet.Add(id, vo)
					}
				}
				return nil
			})
			if i != 0 {
				set = set.And(otherSet)
			}
		}
		res = set.List()
	case sfvm.MatchHaveAny:
		for _, condition := range comparator.Conditions {
			matched := matchValue(condition)
			if !matched.IsEmpty() {
				res = append(res, matched...)
			}
		}
	}
	// Return matched values; VM will normalize bool mask against source width.
	return sfvm.NewValues(res), nil
}

func (p *Program) String() string {
	return p.Program.GetProgramName()
}
func (p *Program) IsMap() bool { return false }

func (p *Program) IsEmpty() bool {
	return p == nil || p.Program == nil
}

func (p *Program) GetAnchorBitVector() *utils.BitVector {
	return nil
}

func (p *Program) SetAnchorBitVector(*utils.BitVector) {}

func (p *Program) AppendPredecessor(sfvm.ValueOperator, ...sfvm.AnalysisContextOption) error {
	// return nil will not change the predecessor
	// no not return any error here!!!!!
	return nil
}

func (p *Program) GetFields() (sfvm.Values, error) {
	return sfvm.NewEmptyValues(), nil
}

func (p *Program) IsList() bool {
	//TODO implement me
	return false
}

func (p *Program) GetOpcode() string {
	return ssa.SSAOpcode2Name[ssa.SSAOpcodeUnKnow]
}

func (p *Program) GetBinaryOperator() string {
	return ssa.SSAOpcode2Name[ssa.SSAOpcodeUnKnow]
}

func (p *Program) GetUnaryOperator() string {
	return ssa.SSAOpcode2Name[ssa.SSAOpcodeUnKnow]
}

func (p *Program) Recursive(f func(operator sfvm.ValueOperator) error) error {
	return f(p)
}

func (p *Program) ExactMatch(ctx context.Context, mod ssadb.MatchMode, s string) (bool, sfvm.Values, error) {
	return p.matchVariable(ctx, ssadb.ExactCompare, mod, s)
}

func (p *Program) GlobMatch(ctx context.Context, mod ssadb.MatchMode, g string) (bool, sfvm.Values, error) {
	return p.matchVariable(ctx, ssadb.GlobCompare, mod, g)
}

func (p *Program) RegexpMatch(ctx context.Context, mod ssadb.MatchMode, re string) (bool, sfvm.Values, error) {
	return p.matchVariable(ctx, ssadb.RegexpCompare, mod, re)
}

func (p *Program) matchVariable(ctx context.Context, compareMode ssadb.CompareMode, mod ssadb.MatchMode, pattern string) (bool, sfvm.Values, error) {
	return p.matchVariableWithExcludeFiles(ctx, compareMode, mod, pattern, nil)
}

// valueTransitivelyMergesFormalParam 判断 SSA 值（含嵌套 phi）是否在数据流上合并了给定形式参数。
func valueTransitivelyMergesFormalParam(ir *ssa.Program, val ssa.Value, paramID int64, visited map[int64]struct{}) bool {
	if utils.IsNil(val) {
		return false
	}
	id := val.GetId()
	if _, ok := visited[id]; ok {
		return false
	}
	visited[id] = struct{}{}
	if pv, ok := ssa.ToParameter(val); ok && pv != nil && pv.GetId() == paramID {
		return true
	}
	phi, ok := ssa.ToPhi(val)
	if !ok || phi == nil {
		return false
	}
	for _, eid := range phi.Edge {
		v, ok := ir.GetInstructionById(eid)
		if !ok {
			continue
		}
		ev, ok := ssa.ToValue(v)
		if !ok {
			continue
		}
		if valueTransitivelyMergesFormalParam(ir, ev, paramID, visited) {
			return true
		}
	}
	return false
}

// appendSameFuncPhisMergingFormalParameter 在同函数内扫描 phi：与形式参数同名且边上（递归）出现该参数时，视为同一变量合并点。
// 用于循环等场景下 Point 反向链不完整、GetPointer 为空时的补全（不沿 GetUsers 做 BFS）。
func appendSameFuncPhisMergingFormalParameter(p *Program, param *ssa.Parameter, pname string, paramID int64, seen map[int64]struct{}, out *Values) {
	ir := p.Program
	if ir == nil || param == nil || pname == "" {
		return
	}
	fn := param.GetFunc()
	if fn == nil {
		return
	}
	paramFnID := fn.GetId()
	for _, bid := range fn.Blocks {
		block, ok := fn.GetBasicBlockByID(bid)
		if !ok || block == nil {
			continue
		}
		for _, phiID := range block.Phis {
			inst, ok := ir.GetInstructionById(phiID)
			if !ok {
				continue
			}
			phi, ok := ssa.ToPhi(inst)
			if !ok || phi == nil || phi.GetName() != pname {
				continue
			}
			pf := phi.GetFunc()
			if pf == nil || pf.GetId() != paramFnID {
				continue
			}
			pid := phi.GetId()
			if _, dup := seen[pid]; dup {
				continue
			}
			merged := false
			for _, eid := range phi.Edge {
				v, ok := ir.GetInstructionById(eid)
				if !ok {
					continue
				}
				ev, ok := ssa.ToValue(v)
				if !ok {
					continue
				}
				if valueTransitivelyMergesFormalParam(ir, ev, paramID, make(map[int64]struct{})) {
					merged = true
					break
				}
			}
			if !merged {
				continue
			}
			nv, err := p.NewValue(phi)
			if err != nil || nv == nil {
				continue
			}
			seen[pid] = struct{}{}
			*out = append(*out, nv)
		}
	}
}

// appendPointerLinkedPhisFromParameters 对每个匹配到的形式参数：
// 1) 追加 GetPointer() 上 reference 指向该形参的 phi（Point 语义）；
// 2) 再按「同函数、同名 phi、边上合并该参数」补全循环等场景下未出现在 GetPointer 的 phi。
func appendPointerLinkedPhisFromParameters(p *Program, values Values) Values {
	if p == nil || len(values) == 0 {
		return values
	}
	seen := make(map[int64]struct{}, len(values)*2)
	for _, v := range values {
		if v != nil {
			seen[v.GetId()] = struct{}{}
		}
	}
	out := append(Values(nil), values...)
	for _, v := range values {
		if v == nil {
			continue
		}
		paramInst := v.getInstruction()
		param, ok := ssa.ToParameter(paramInst)
		if !ok || param == nil || param.IsFreeValue {
			continue
		}
		paramID := paramInst.GetId()
		pname := param.GetName()
		ptrSrc, ok := paramInst.(ssa.PointerIF)
		if ok && ptrSrc != nil {
			for _, ptr := range ptrSrc.GetPointer() {
				if utils.IsNil(ptr) {
					continue
				}
				phiInst, ok := ssa.ToPhi(ptr)
				if !ok || phiInst == nil {
					continue
				}
				ref := ptr.GetReference()
				if utils.IsNil(ref) || ref.GetId() != paramID {
					continue
				}
				if pname != "" && phiInst.GetName() != pname {
					continue
				}
				pid := ptr.GetId()
				if _, dup := seen[pid]; dup {
					continue
				}
				nv, err := p.NewValue(phiInst)
				if err != nil || nv == nil {
					continue
				}
				seen[pid] = struct{}{}
				out = append(out, nv)
			}
		}
		appendSameFuncPhisMergingFormalParameter(p, param, pname, paramID, seen, &out)
	}
	return out
}

// matchVariableWithExcludeFiles 搜索变量，支持排除指定文件
// excludeFiles: 要排除的文件路径列表（规范化后的路径，如 "/test.go"）
func (p *Program) matchVariableWithExcludeFiles(ctx context.Context, compareMode ssadb.CompareMode, mod ssadb.MatchMode, pattern string, excludeFiles []string) (bool, sfvm.Values, error) {
	var values Values = lo.FilterMap(
		ssa.MatchInstructionsByVariableWithExcludeFiles(ctx, p.Program, compareMode, mod, pattern, excludeFiles),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, err := p.NewValue(i); err != nil {
				log.Errorf("matchVariable: new value failed: %v", err)
				return nil, false
			} else {
				return v, true
			}
		},
	)
	values = appendPointerLinkedPhisFromParameters(p, values)
	// 将 Values 转换为 sfvm.ValueOperator
	return len(values) > 0, ToSFVMValues(values), nil
}

func (p *Program) ListIndex(i int) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported list index")
}

func (p *Program) Merge(sfv ...sfvm.ValueOperator) (sfvm.Values, error) {
	groups := make([]sfvm.Values, 0, len(sfv)+1)
	groups = append(groups, sfvm.ValuesOf(p))
	for _, value := range sfv {
		if utils.IsNil(value) {
			continue
		}
		groups = append(groups, sfvm.ValuesOf(value))
	}
	return sfvm.MergeValues(groups...), nil
}

func (p *Program) Remove(...sfvm.ValueOperator) (sfvm.Values, error) {
	return nil, utils.Error("ssa.Program is not supported remove")
}

func (p *Program) GetCallActualParams(int, bool) (sfvm.Values, error) {
	return nil, utils.Error("ssa.Program is not supported call all actual params")
}

func (p *Program) GetSyntaxFlowDef() (sfvm.Values, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow def")
}
func (p *Program) GetSyntaxFlowUse() (sfvm.Values, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow use")
}
func (p *Program) GetSyntaxFlowTopDef(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow top def")
}

func (p *Program) GetSyntaxFlowBottomUse(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.Values, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow bottom use")
}

func (p *Program) GetCalled() (sfvm.Values, error) {
	return nil, utils.Error("ssa.Program is not supported called")
}

type Index struct {
	Start int
	End   int
}
type FileFilter struct {
	matchFile    func(string) bool
	matchContent func(string) []Index
}

func NewFileFilter(file, matchType string, match []string) *FileFilter {
	var matchFile []func(string) bool
	if matchFile == nil {
		matchFile = []func(string) bool{
			func(s string) bool {
				return s == file
			},
		}
	}
	if reg, err := regexp.Compile(file); err == nil {
		matchFile = append(matchFile, func(s string) bool {
			return reg.Match([]byte(s))
		})
	}
	if glob, err := glob.Compile(file); err == nil {
		matchFile = append(matchFile, func(s string) bool {
			return glob.Match(s)
		})
	}
	if matchFile == nil {
		matchFile = append(matchFile, func(s string) bool {
			return s == file
		})
	}

	var matchContent []func(data string) []Index
	for _, rule := range match {
		switch matchType {
		case "regexp":
			reg := regexp_utils.NewYakRegexpUtils(rule)
			// reg, err := regexp2.Compile(rule, regexp2.None)
			// if err != nil {
			// 	log.Errorf("regexp compile error: %s", err)
			// 	continue
			// }
			matchContent = append(matchContent, func(data string) []Index {
				indexs, err := reg.FindAllSubmatchIndex(data)
				if err != nil {
					log.Warnf("regexp match error: %s", err)
					return nil
				}
				if len(indexs) == 0 {
					return nil
				}
				res := make([]Index, 0)
				for _, index := range indexs {
					res = append(res, Index{Start: index[0], End: index[1]})
				}
				return res
			})
		case "xpath":
			matcher, err := NewFileXPathMatcher(rule)
			if err != nil {
				log.Errorf("xpath match error: %s", err)
				continue
			}
			matchContent = append(matchContent, func(data string) []Index {
				results, err := matcher.Match(data)
				if err != nil {
					log.Errorf("xpath match error: %s", err)
					return nil
				}
				res := make([]Index, 0)
				for _, result := range results {
					// TODO:使用string.Index会导致遇到重复内容位置会不正确;
					// 此外，如果遇到中文，位置也会不正确。
					substrings := utils.IndexAllSubstrings(data, result)
					for _, subString := range substrings {
						res = append(res, Index{Start: subString[1], End: subString[1] + len(result)})
					}
				}
				return res
			})
		case "jsonpath": // json path
			jsonFilter, err := jsonpath.Prepare(rule)
			if err != nil {
				log.Errorf("json path parse error: %s", err)
				continue
			}
			matchContent = append(matchContent, func(data string) []Index {
				structuredData, err := parseStructuredContent(data)
				if err != nil {
					log.Errorf("structured parse error: %s", err)
					return nil
				}

				matched, err := jsonFilter(structuredData)
				if err != nil {
					log.Errorf("json path match content error: %s", err)
					return nil
				}

				var searchResults []interface{}
				switch ret := matched.(type) {
				case nil:
					return nil
				case []interface{}:
					searchResults = ret
				default:
					searchResults = []interface{}{ret}
				}

				res := make([]Index, 0)
				for _, searchResult := range searchResults {
					str := codec.AnyToString(searchResult)
					substrings := utils.IndexAllSubstrings(data, str)
					for _, subString := range substrings {
						res = append(res, Index{Start: subString[1], End: subString[1] + len(str)})
					}
				}

				return res
			})
		}
	}

	return &FileFilter{
		matchFile: func(s string) bool {
			for _, f := range matchFile {
				if f(s) {
					return true
				}
			}
			return false
		},
		matchContent: func(data string) []Index {
			var allResults []Index
			for _, matcher := range matchContent {
				results := matcher(data)
				if results != nil {
					allResults = append(allResults, results...)
				}
			}
			if len(allResults) == 0 {
				return nil
			}
			return allResults
		},
	}
}

func (p *Program) getEditor(filename, hash string) (*memedit.MemEditor, error) {
	if editor, ok := p.Program.GetEditor(filename); ok {
		return editor, nil
	}

	if p.Program.DatabaseKind == ssa.ProgramCacheMemory {
		return nil, utils.Errorf("get editor by filename %s not found", filename)
	}
	// if have database, get source code from database
	if editor, err := ssadb.GetEditorByHash(hash); err != nil {
		return nil, utils.Errorf("get ir source from hash error: %s", err)
	} else {
		p.Program.SetEditor(filename, editor)
		return editor, nil
	}
}

func (p *Program) ForEachExtraFile(callBack func(string, *memedit.MemEditor) bool) {
	p.foreach(p.Program.ExtraFile, callBack)
}

func (p *Program) ForEachAllFile(callBack func(string, *memedit.MemEditor) bool) {
	p.foreach(p.Program.FileList, callBack)
}
func (p *Program) foreach(file2Hash map[string]string, callBack func(string, *memedit.MemEditor) bool) {
	handler := func(filename, hash string) bool {
		editor, err := p.getEditor(filename, hash)
		if err != nil {
			log.Errorf("get editor [%s] not found: %v", filename, err)
			return true
		}
		return callBack(filename, editor)
	}
	for filename, hash := range file2Hash {
		if !handler(filename, hash) {
			break
		}
	}
}

func (p *Program) FileFilter(path string, match string, rule map[string]string, rule2 []string) (sfvm.Values, error) {
	filter := NewFileFilter(path, match, rule2)
	if filter == nil {
		return nil, nil
	}

	var res []sfvm.ValueOperator
	addRes := func(index Index, editor *memedit.MemEditor, offsetMap *memedit.RuneOffsetMap) {
		// get range of match string
		if startRune, ok := offsetMap.ByteOffsetToRuneIndex(index.Start); ok {
			index.Start = startRune
		}
		if endRune, ok := offsetMap.ByteOffsetToRuneIndex(index.End); ok {
			index.End = endRune
		}
		rangeIf := editor.GetRangeOffset(index.Start, index.End)
		val := p.NewConstValue(rangeIf.GetText(), rangeIf)
		res = append(res, val)
	}

	matchFile := false
	p.ForEachAllFile(func(s string, me *memedit.MemEditor) bool {
		if me == nil {
			return true
		}
		offsetMap := memedit.NewRuneOffsetMap(me.GetSourceCode())
		if filter.matchFile(s) {
			matchFile = true
			if filter.matchContent != nil {
				matches := filter.matchContent(me.GetSourceCode())
				for _, match := range matches {
					addRes(match, me, offsetMap)
				}
			}
		}
		return true
	})
	if len(res) == 0 {
		if matchFile {
			return nil, utils.Errorf("no file contains data matching rule %v %v", rule, rule2)
		}
		return nil, utils.Errorf("no file matched by path %s", path)
	}
	return sfvm.NewValues(res), nil
}
