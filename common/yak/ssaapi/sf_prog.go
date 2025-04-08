package ssaapi

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/antchfx/xpath"
	"github.com/gobwas/glob"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/htmlquery"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var _ sfvm.ValueOperator = &Program{}

func (p *Program) CompareConst(comparator *sfvm.ConstComparator) []bool {
	return []bool{false}
}

func (p *Program) NewConst(i any, rng ...memedit.RangeIf) sfvm.ValueOperator {
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
				return nil, false
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
		return nil
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

func (p *Program) FileFilter(path string, match string, rule map[string]string, rule2 []string) (sfvm.ValueOperator, error) {
	if p.comeFromDatabase {
		log.Infof("file filter support with database")
	}
	var res []sfvm.ValueOperator
	addRes := func(str, content string) {
		// get range of match string
		start := strings.Index(content, match)
		editor := memedit.NewMemEditor(content)
		editor.SetUrl(path)
		rangeIf := editor.GetRangeOffset(start, start+len(match))
		val := p.NewConstValue(str, rangeIf)
		res = append(res, val)
	}
	matchFile := false
	handler := func(data string) {
		matchFile = true
		for _, rule := range rule2 {
			switch match {
			case "regexp":
				reg, err := regexp.Compile(rule)
				if err != nil {
					log.Errorf("regexp compile error: %s", err)
					continue
				}
				matches := reg.FindAllStringSubmatch(data, -1)
				// skip first captured group
				// if "url=(.*)", exclude "url="
				// match[0] contain "url=", match[1] not contain
				for _, match := range matches {
					if len(match) > 1 {
						addRes(match[1], data)
					}
				}
			case "xpath":
				top, err := htmlquery.Parse(strings.NewReader(data))
				if err != nil {
					continue
				}

				xexp, err := xpath.Compile(rule)
				if err != nil {
					log.Errorf("xpath compile error: %s", err)
					continue
				}

				t := xexp.Evaluate(htmlquery.CreateXPathNavigator(top))
				switch t := t.(type) {
				case *xpath.NodeIterator:
					for t.MoveNext() {
						nav := t.Current().(*htmlquery.NodeNavigator)
						node := nav.Current()
						str := htmlquery.InnerText(node)
						addRes(str, data)
					}
				default:
					str := codec.AnyToString(t)
					addRes(str, data)
				}
			case "json": // json path
			}
		}
	}

	for filename, hash := range p.Program.ExtraFile {
		var data string
		if p.Program.EnableDatabase {
			// if have database, get source code from database
			editor, err := ssadb.GetIrSourceFromHash(hash)
			if err != nil {
				log.Errorf("get ir source from hash error: %s", err)
				continue
			}
			data = editor.GetSourceCode()
		} else {
			// if no database, get source code from memory
			data = hash
		}
		if reg, err := regexp.Compile(path); err == nil {
			if reg.Match([]byte(filename)) {
				handler(data)
			}
		}

		if glob, err := glob.Compile(path); err == nil {
			if glob.Match(filename) {
				handler(data)
			}
		}

		if filename == path {
			handler(data)
		}
	}
	if len(res) == 0 {
		if matchFile {
			return nil, utils.Errorf("no file contain data match rule %v %v", rule, rule2)
		}
		return nil, utils.Errorf("no file matched by path %s", path)
	}
	return sfvm.NewValues(res), nil
}
