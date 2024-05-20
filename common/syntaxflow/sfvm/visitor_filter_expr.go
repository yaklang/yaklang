package sfvm

import (
	"reflect"
	"regexp"

	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func (y *SyntaxFlowVisitor) VisitFilterExpr(raw sf.IFilterExprContext) error {
	if y == nil || raw == nil {
		return nil
	}

	switch ret := raw.(type) {
	case *sf.CurrentRootFilterContext:
		if id := ret.Identifier(); id != nil {
			y.EmitNewRef(id.GetText())
			return nil
		}
		return nil
	case *sf.PrimaryFilterContext:
		return y.VisitNameFilter(false, ret.NameFilter())
	case *sf.FieldFilterContext:
		return y.VisitNameFilter(true, ret.NameFilter())
	case *sf.FieldCallFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr())
		if err != nil {
			return err
		}
		return y.VisitNameFilter(true, ret.NameFilter())
	case *sf.FunctionCallFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr())
		if err != nil {
			return err
		}
		y.EmitGetCall()
		if ret.ActualParam() != nil {
			y.VisitActualParam(ret.ActualParam())
		}
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
			y.VisitNameFilter(true, member.NameFilter())
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

func (y *SyntaxFlowVisitor) VisitNameFilter(isMember bool, i sf.INameFilterContext) error {
	if i == nil {
		return nil
	}

	ret, ok := i.(*sf.NameFilterContext)
	if !ok {
		return utils.Errorf("BUG: in nameFilter: %s", reflect.TypeOf(i))
	}

	if s := ret.Star(); s != nil {
		if isMember {
			// get all member
			y.EmitSearchGlob(true, "*")
		}
		// skip
		return nil
	} else if id := ret.Identifier(); id != nil {
		text := ret.Identifier().GetText()
		filter, isGlob := y.FormatStringOrGlob(text) // emit field
		if isGlob {
			y.EmitSearchGlob(isMember, filter)
		} else {
			y.EmitSearchExact(isMember, filter)
		}
		return nil
	} else if re, ok := ret.RegexpLiteral().(*sf.RegexpLiteralContext); ok {
		text := re.RegexpLiteral().GetText()
		text = text[1 : len(text)-1]
		// log.Infof("regexp: %s", text)
		reIns, err := regexp.Compile(text)
		if err != nil {
			return err
		}
		y.EmitSearchRegexp(isMember, reIns.String())
		return nil
	}
	return utils.Errorf("BUG: in nameFilter, unknown type: %s", reflect.TypeOf(ret))
}

func (y *SyntaxFlowVisitor) VisitActualParam(i sf.IActualParamContext) error {
	handlerItem := func(i sf.ISingleParamContext) {
		ret, ok := (i).(*sf.SingleParamContext)
		if !ok {
			return
		}

		if ret.FilterStatement() != nil {
			y.VisitFilterStatement(ret.FilterStatement())
		}
		// TODO: handler recursive config
	}

	switch ret := i.(type) {
	case *sf.AllParamContext:
		y.EmitPushAllCallArgs()
		handlerItem(ret.SingleParam())
	case *sf.EveryParamContext:
		for i, paraI := range ret.AllActualParamFilter() {
			para, ok := paraI.(*sf.ActualParamFilterContext)
			if !ok {
				continue
			}
			single := para.SingleParam()
			if single == nil {
				continue
			}
			y.EmitDuplicate()
			y.EmitPushCallArgs(i)
			handlerItem(single)
		}
		if ret.SingleParam() != nil {
			y.EmitDuplicate()
			y.EmitPushCallArgs(len(ret.AllActualParamFilter()))
			handlerItem(ret.SingleParam())
		}
	default:
		return utils.Errorf("BUG: ActualParamFilter type error: %s", reflect.TypeOf(ret))
	}
	return nil
}
