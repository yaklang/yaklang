package sfvm

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"regexp"
	"strings"
)

func (y *SyntaxFlowVisitor) VisitFilterExpr(raw sf.IFilterExprContext) error {
	if y == nil || raw == nil {
		return nil
	}

	switch ret := raw.(type) {
	case *sf.WildcardFilterContext:
		y.EmitSearchGlob("*")
		return nil
	case *sf.CurrentRootFilterContext:
		y.EmitCheckStackTop()
		log.Warnf("TBD for CurrentRootFilter")
		return nil
	case *sf.PrimaryFilterContext:
		filter, glob := y.FormatStringOrGlob(ret.Identifier().GetText()) // emit field
		if glob {
			y.EmitSearchGlob(filter)
		} else {
			y.EmitSearchExact(filter)
		}
		return nil
	case *sf.RegexpLiteralFilterContext:
		regexpRaw := ret.RegexpLiteral().GetText()
		if !(len(regexpRaw) > 2 && regexpRaw[0] == '/' && regexpRaw[len(regexpRaw)-1] == '/') {
			return utils.Errorf("regexp format error: %v", regexpRaw)
		}
		regexpRaw = regexpRaw[1 : len(regexpRaw)-1]
		r, err := regexp.Compile(regexpRaw)
		if err != nil {
			return utils.Errorf("regexp compile error: %v", err)
		}
		y.EmitSearchRegexp(r.String())
		return nil
	case *sf.FieldFilterContext:
		y.EmitGetMembers()
		return y.VisitFilterExpr(ret.FilterExpr())
	case *sf.FieldCallFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr(0))
		if err != nil {
			return err
		}
		y.EmitGetMembers()
		return y.VisitFilterExpr(ret.FilterExpr(1))
	case *sf.FunctionCallFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr())
		if err != nil {
			return err
		}
		actualParamsAtLeast := ret.AllAcutalParamFilter()
		y.EmitPushCallArgs(len(actualParamsAtLeast))
		for idx, actualFilter := range actualParamsAtLeast {
			switch ret := actualFilter.(type) {
			case *sf.EmptyParamContext:
				continue
			case *sf.NamedParamContext:
				y.EmitListIndex(idx)
				t := ret.GetText()
				if strings.HasPrefix(t, "#") {
					y.EmitGetTopDef()
				}
			}
		}
		//y.EmitPop()
	case *sf.FieldIndexFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr())
		if err != nil {
			return err
		}
		memberRaw := ret.SliceCallItem()
		member, ok := memberRaw.(*sf.SliceCallItemContext)
		if !ok {
			panic("BUG: in fieldIndexFilter")
		}
		if member.NumberLiteral() != nil {
			y.EmitListIndex(codec.Atoi(member.NumberLiteral().GetText()))
		} else {
			y.VisitNameFilter(member.NameFilter())
		}
	case *sf.OptionalFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr())
		if err != nil {
			return err
		}
		y.VisitConditionExpression(ret.ConditionExpression())
	case *sf.NextFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr(0))
		if err != nil {
			return err
		}
		y.EmitGetUsers()
		err = y.VisitFilterExpr(ret.FilterExpr(1))
		if err != nil {
			return err
		}
	case *sf.DefFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr(0))
		if err != nil {
			return err
		}
		y.EmitGetDefs()
		err = y.VisitFilterExpr(ret.FilterExpr(1))
		if err != nil {
			return err
		}
	case *sf.DeepNextFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr(0))
		if err != nil {
			return err
		}
		y.EmitGetBottomUsers()
		err = y.VisitFilterExpr(ret.FilterExpr(1))
		if err != nil {
			return err
		}
	case *sf.TopDefFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr(0))
		if err != nil {
			return err
		}
		y.EmitGetTopDefs()
		err = y.VisitFilterExpr(ret.FilterExpr(1))
		if err != nil {
			return err
		}
	case *sf.ConfiggedDeepNextFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr(0))
		if err != nil {
			return err
		}
		if i := ret.RecursiveConfig(); i != nil {
			y.EmitGetBottomUsersWithConfig(y.VisitRecursiveConfig(i.(*sf.RecursiveConfigContext)))
		} else {
			y.EmitGetBottomUsers()
		}
		err = y.VisitFilterExpr(ret.FilterExpr(1))
		if err != nil {
			return err
		}
	case *sf.ConfiggedTopDefFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr(0))
		if err != nil {
			return err
		}
		if i := ret.RecursiveConfig(); i != nil {
			y.EmitGetTopDefsWithConfig(y.VisitRecursiveConfig(i.(*sf.RecursiveConfigContext)))
		} else {
			y.EmitGetBottomUsers()
		}
		err = y.VisitFilterExpr(ret.FilterExpr(1))
		if err != nil {
			return err
		}
	default:
		panic("BUG: in filterExpr")
	}

	return nil
}

func (y *SyntaxFlowVisitor) VisitNameFilter(i sf.INameFilterContext) error {
	if i == nil {
		return nil
	}

	ret, ok := i.(*sf.NameFilterContext)
	if !ok {
		return utils.Error("BUG: in nameFilter")
	}

	if id := ret.Identifier(); id != nil {
		filter, glob := y.FormatStringOrGlob(ret.Identifier().GetText()) // emit field
		if glob {
			y.EmitSearchGlob(filter)
		} else {
			y.EmitSearchExact(filter)
		}
		return nil
	} else if re := ret.RegexpLiteral(); re != nil {
		reIns, err := regexp.Compile(re.GetText())
		if err != nil {
			return err
		}
		y.EmitSearchRegexp(reIns.String())
		return nil
	}
	return utils.Error("BUG: in nameFilter, unknown type")
}
