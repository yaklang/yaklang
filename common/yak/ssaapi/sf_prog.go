package ssaapi

import (
	"context"
	"encoding/json"
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

func (p *Program) CompareConst(comparator *sfvm.ConstComparator) []bool {
	return []bool{false}
}

func (p *Program) NewConst(i any, rng ...*memedit.Range) sfvm.ValueOperator {
	return p.NewConstValue(i, rng...)
}

func (p *Program) CompareOpcode(opcodeItems *sfvm.OpcodeComparator) (sfvm.ValueOperator, []bool) {
	var boolRes []bool
	ctx := opcodeItems.Context
	var res Values = lo.FilterMap(
		ssa.MatchInstructionByOpcodes(ctx, p.Program, opcodeItems.Opcodes...),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, err := p.NewValue(i); err != nil {
				log.Errorf("CompareOpcode: new value failed: %v", err)
				return v, false
			} else {
				boolRes = append(boolRes, true)
				return v, true
			}
		},
	)
	return res, boolRes
}

func (p *Program) CompareString(comparator *sfvm.StringComparator) (sfvm.ValueOperator, []bool) {
	var res []sfvm.ValueOperator
	var boolRes []bool
	ctx := comparator.Context

	matchValue := func(condition *sfvm.StringCondition) sfvm.ValueOperator {
		var v sfvm.ValueOperator
		switch condition.FilterMode {
		case sfvm.GlobalConditionFilter:
			_, v, _ = p.GlobMatch(ctx, ssadb.NameMatch, condition.Pattern)
		case sfvm.RegexpConditionFilter:
			_, v, _ = p.RegexpMatch(ctx, ssadb.NameMatch, condition.Pattern)
		case sfvm.ExactConditionFilter:
			_, v, _ = p.RegexpMatch(ctx, ssadb.NameMatch, fmt.Sprintf(".*%s.*", condition.Pattern))
		}
		return v
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
			if matched != nil {
				res = append(res, matched)
			}
		}
	}
	result := sfvm.NewValues(res)
	result.Recursive(func(operator sfvm.ValueOperator) error {
		boolRes = append(boolRes, true)
		return nil
	})
	return result, boolRes
}

func (p *Program) String() string {
	return p.Program.GetProgramName()
}
func (p *Program) IsMap() bool { return false }

func (p *Program) IsEmpty() bool {
	return p == nil || p.Program == nil
}

func (p *Program) AppendPredecessor(sfvm.ValueOperator, ...sfvm.AnalysisContextOption) error {
	// return nil will not change the predecessor
	// no not return any error here!!!!!
	return nil
}

func (p *Program) GetFields() (sfvm.ValueOperator, error) {
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

func (p *Program) ExactMatch(ctx context.Context, mod int, s string) (bool, sfvm.ValueOperator, error) {
	var values Values = lo.FilterMap(
		ssa.MatchInstructionByExact(ctx, p.Program, mod, s),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, err := p.NewValue(i); err != nil {
				log.Errorf("ExactMatch: new value failed: %v", err)
				return nil, false
			} else {
				return v, true
			}
		},
	)
	return len(values) > 0, values, nil
}

func (p *Program) GlobMatch(ctx context.Context, mod int, g string) (bool, sfvm.ValueOperator, error) {
	var values Values = lo.FilterMap(
		ssa.MatchInstructionByGlob(ctx, p.Program, mod, g),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, err := p.NewValue(i); err != nil {
				log.Errorf("GlobMatch: new value failed: %v", err)
				return nil, false
			} else {
				return v, true
			}
		},
	)
	return len(values) > 0, values, nil
}

func (p *Program) RegexpMatch(ctx context.Context, mod int, re string) (bool, sfvm.ValueOperator, error) {
	var values Values = lo.FilterMap(
		ssa.MatchInstructionByRegexp(ctx, p.Program, mod, re),
		func(i ssa.Instruction, _ int) (*Value, bool) {
			if v, err := p.NewValue(i); err != nil {
				log.Errorf("RegexpMatch: new value failed: %v", err)
				return nil, false
			} else {
				return v, true
			}
		},
	)
	return len(values) > 0, values, nil
}

func (p *Program) ListIndex(i int) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported list index")
}

func (p *Program) Merge(sfv ...sfvm.ValueOperator) (sfvm.ValueOperator, error) {
	sfv = append(sfv, p)
	return MergeSFValueOperator(sfv...), nil
}

func (p *Program) Remove(...sfvm.ValueOperator) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported remove")
}

func (p *Program) GetCallActualParams(int, bool) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported call all actual params")
}

func (p *Program) GetSyntaxFlowDef() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow def")
}
func (p *Program) GetSyntaxFlowUse() (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow use")
}
func (p *Program) GetSyntaxFlowTopDef(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow top def")
}

func (p *Program) GetSyntaxFlowBottomUse(sfResult *sfvm.SFFrameResult, sfConfig *sfvm.Config, config ...*sfvm.RecursiveConfigItem) (sfvm.ValueOperator, error) {
	return nil, utils.Error("ssa.Program is not supported syntax flow bottom use")
}

func (p *Program) GetCalled() (sfvm.ValueOperator, error) {
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
					log.Errorf("regexp match error: %s", err)
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
				m := make(map[string]interface{})
				err := json.Unmarshal([]byte(data), &m)
				if err != nil {
					log.Errorf("json parse error: %s", err)
					return nil
				}

				matched, err := jsonFilter(m)
				if err != nil {
					log.Errorf("json path match content error: %s", err)
					return nil
				}

				searchResults, ok := matched.([]interface{})
				if !ok {
					return nil
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

func (p *Program) FileFilter(path string, match string, rule map[string]string, rule2 []string) (sfvm.ValueOperator, error) {
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
