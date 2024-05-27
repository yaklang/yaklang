package sfvm

import (
	"reflect"
	"regexp"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func (y *SyntaxFlowVisitor) VisitFilterExpr(raw sf.IFilterExprContext) error {
	if y == nil || raw == nil {
		return nil
	}

	switch ret := raw.(type) {
	// variable
	case *sf.CurrentRootFilterContext:
		if id := ret.Identifier(); id != nil {
			y.EmitNewRef(id.GetText())
			return nil
		}
		log.Infof("current root filter identifier is nil")
		return nil
	// filter name  from input
	case *sf.PrimaryFilterContext:
		if !y.filterExpr {
			y.EmitDuplicate()
		}
		return y.VisitNameFilter(false, ret.NameFilter())
	// filter field from input
	case *sf.FieldFilterContext:
		if !y.filterExpr {
			y.EmitDuplicate()
		}
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
		recoverFilterExpr := y.EnterFilterExpr()
		err = y.VisitFilterExpr(ret.FilterExpr(1))
		recoverFilterExpr()
		if err != nil {
			return err
		}
	case *sf.DefFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr(0))
		if err != nil {
			return err
		}
		y.EmitGetDefs()
		recoverFilterExpr := y.EnterFilterExpr()
		err = y.VisitFilterExpr(ret.FilterExpr(1))
		recoverFilterExpr()
		if err != nil {
			return err
		}
	case *sf.DeepNextFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr(0))
		if err != nil {
			return err
		}
		y.EmitGetBottomUsers()
		recoverFilterExpr := y.EnterFilterExpr()
		err = y.VisitFilterExpr(ret.FilterExpr(1))
		recoverFilterExpr()
		if err != nil {
			return err
		}
	case *sf.TopDefFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr(0))
		if err != nil {
			return err
		}
		y.EmitGetTopDefs()
		recoverFilterExpr := y.EnterFilterExpr()
		err = y.VisitFilterExpr(ret.FilterExpr(1))
		recoverFilterExpr()
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
		recoverFilterExpr := y.EnterFilterExpr()
		err = y.VisitFilterExpr(ret.FilterExpr(1))
		recoverFilterExpr()
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
		recoverFilterExpr := y.EnterFilterExpr()
		err = y.VisitFilterExpr(ret.FilterExpr(1))
		recoverFilterExpr()
		if err != nil {
			return err
		}
	case *sf.UseDefCalcFilterContext:
		err := y.VisitFilterExpr(ret.FilterExpr(0))
		if err != nil {
			return err
		}
		err = y.VisitFilterExpr(ret.FilterExpr(1))
		if err != nil {
			return err
		}
		log.Warn("TBD: UseDefCalcFilterContext")
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
	} else if id := ret.DollarOutput(); id != nil {
		y.EmitSearchExact(isMember, id.GetText())
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
	return utils.Errorf("BUG: in nameFilter, unknown type: %s:%s", reflect.TypeOf(ret), ret.GetText())
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
			y.EmitPop()
		}
		if ret.SingleParam() != nil {
			y.EmitDuplicate()
			y.EmitPushCallArgs(len(ret.AllActualParamFilter()))
			handlerItem(ret.SingleParam())
			y.EmitPop()
		}
	default:
		return utils.Errorf("BUG: ActualParamFilter type error: %s", reflect.TypeOf(ret))
	}
	return nil
}
