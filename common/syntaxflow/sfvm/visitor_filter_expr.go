package sfvm

import (
	"reflect"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils/yakunquote"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func (y *SyntaxFlowVisitor) VisitFilterExpr(raw sf.IFilterExprContext) error {
	if y == nil || raw == nil {
		return nil
	}
	i, ok := raw.(*sf.FilterExprContext)
	if !ok {
		err := utils.Errorf("BUG: in filterExpr: %s", reflect.TypeOf(raw))
		log.Errorf("%v", err)
		return err
	}

	enter := y.EmitEnterStatement()
	defer func() {
		y.EmitExitStatement(enter)
	}()
	if raw := i.FilterItemFirst(); raw != nil {
		y.VisitFilterItemFirst(raw)
	}

	for _, raw := range i.AllFilterItem() {
		y.VisitFilterItem(raw)
	}
	return nil
}

func (y *SyntaxFlowVisitor) VisitFilterItem(raw sf.IFilterItemContext) error {
	switch filter := raw.(type) {
	case *sf.FirstContext:
		y.VisitFilterItemFirst(filter.FilterItemFirst())
	case *sf.FunctionCallFilterContext:
		//先拿到所有的call，然后再去拿callArgs
		y.EmitGetCall()
		y.EmitOpEmptyCompare()
		if filter.ActualParam() != nil {
			y.VisitActualParam(filter.ActualParam(), filter.Question() != nil)
		}
		//拿到符合要求的call
		y.EmitCondition()
		//检查栈顶，应该可以被里面的值影响到
		y.EmitCheckStackTop()
	case *sf.DeepChainFilterContext:
		if filter.NameFilter().GetText() == "*" {
			err := utils.Error("Syntax ERROR: deep chain filter cannot be ...*")
			log.Errorf("%v", err)
			return err
		}
		y.VisitRecursiveNameFilter(true, true, filter.NameFilter())
	case *sf.FieldIndexFilterContext:
		memberRaw := filter.SliceCallItem()
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
		y.EmitConditionStart()
		y.VisitConditionExpression(filter.ConditionExpression())
		y.EmitCondition()
	case *sf.NextFilterContext:
		y.EmitGetUsers()
	case *sf.DefFilterContext:
		y.EmitGetDefs()
	case *sf.DeepNextFilterContext:
		y.EmitGetBottomUsers()
	case *sf.DeepNextConfigFilterContext:
		config := []*RecursiveConfigItem{}
		if i := filter.Config(); i != nil {
			config = y.VisitRecursiveConfig(i.(*sf.ConfigContext))
		}
		y.EmitGetBottomUsers(config...)
	case *sf.TopDefFilterContext:
		y.EmitGetTopDefs()
	case *sf.TopDefConfigFilterContext:
		config := []*RecursiveConfigItem{}
		if i := filter.Config(); i != nil {
			config = y.VisitRecursiveConfig(i.(*sf.ConfigContext))
		}
		y.EmitGetTopDefs(config...)
	case *sf.MergeRefFilterContext:
		y.EmitMergeRef(strings.TrimLeft(filter.RefVariable().GetText(), "$"))
	case *sf.RemoveRefFilterContext:
		y.EmitRemoveRef(strings.TrimLeft(filter.RefVariable().GetText(), "$"))
	case *sf.IntersectionRefFilterContext:
		y.EmitIntersectionRef(strings.TrimLeft(filter.RefVariable().GetText(), "$"))
	case *sf.VersionInFilterContext:
		if versionIn := filter.VersionInExpression(); versionIn != nil {
			y.VisitVersionInExpression(versionIn)
		}
		y.EmitCondition()
	default:
		panic("BUG: in filterExpr")
	}
	return nil
}

func (y *SyntaxFlowVisitor) VisitFilterItemFirst(raw sf.IFilterItemFirstContext) error {

	if y == nil || raw == nil {
		return nil
	}
	switch i := raw.(type) {
	case *sf.ConstFilterContext:
		var (
			mode string
			rule string
		)
		if i.ConstSearchPrefix() != nil {
			prefix := i.ConstSearchPrefix().(*sf.ConstSearchPrefixContext)
			switch {
			case prefix.ConstSearchModePrefixGlob() != nil:
				mode = "g"
			case prefix.ConstSearchModePrefixRegexp() != nil:
				mode = "r"
			case prefix.ConstSearchModePrefixExact() != nil:
				mode = "e"
			}
		}
		if i.QuotedStringLiteral() != nil {
			rule = i.QuotedStringLiteral().GetText()
			rule = yakunquote.TryUnquote(rule)
		} else {
			rule = y.VisitHereDoc(i.HereDoc())
		}
		if mode == "" {
			if glob, b := y.FormatStringOrGlob(rule); b {
				mode = "g"
				rule = glob
			} else {
				mode = "e"
			}
		}
		y.EmitNativeCall("const", &RecursiveConfigItem{
			Key:            mode,
			Value:          rule,
			SyntaxFlowRule: false,
		})
	case *sf.NamedFilterContext:
		return y.VisitNameFilter(false, i.NameFilter())
	case *sf.FieldCallFilterContext:
		return y.VisitNameFilter(true, i.NameFilter())
	case *sf.NativeCallFilterContext:
		var varname string
		var items []*RecursiveConfigItem

		if nc, ok := i.NativeCall().(*sf.NativeCallContext); ok {
			if identify, ok := nc.UseNativeCall().(*sf.UseNativeCallContext); ok {
				varname = identify.Identifier().GetText()

				if identify.UseDefCalcParams() != nil {
					if configable, ok := identify.UseDefCalcParams().(*sf.UseDefCalcParamsContext); ok {
						if configable.NativeCallActualParams() != nil {
							items = y.VisitNativeCallActualParams(configable.NativeCallActualParams().(*sf.NativeCallActualParamsContext))
						}
					}
				}
			}
		}
		y.EmitNativeCall(varname, items...)
	default:
		panic("BUG: in filter first")
	}

	return nil
}

func (y *SyntaxFlowVisitor) VisitRecursiveNameFilter(recursive bool, isMember bool, i sf.INameFilterContext) error {
	if i == nil {
		return nil
	}

	ret, ok := i.(*sf.NameFilterContext)
	if !ok {
		err := utils.Errorf("BUG: in nameFilter: %s", reflect.TypeOf(i))
		log.Errorf("%v", err)
		return err
	}

	mod := NameMatch
	if isMember {
		mod = KeyMatch
	}

	if s := ret.Star(); s != nil {
		if isMember {
			// get all member
			if recursive {
				err := utils.Errorf("Syntax ERROR: recursive name filter cannot be *")
				log.Errorf("%v", err)
				return err
			} else {
				y.EmitSearchGlob(mod, "*")
			}
		}
		// skip
		return nil
		// } else if id := ret.DollarOutput(); id != nil {
		// 	y.EmitSearchExact(mod, id.GetText())
		// 	return nil
	} else if id := ret.Identifier(); id != nil {
		text := ret.Identifier().GetText()
		filter, isGlob := y.FormatStringOrGlob(text) // emit field
		if isGlob {
			if recursive {
				y.EmitRecursiveSearchGlob(mod, filter)
			} else {
				y.EmitSearchGlob(mod, filter)
			}
		} else {
			if recursive {
				y.EmitRecursiveSearchExact(mod, filter)
			} else {
				y.EmitSearchExact(mod, filter)
			}
		}
		return nil
	} else if re, ok := ret.RegexpLiteral().(*sf.RegexpLiteralContext); ok {
		text := re.RegexpLiteral().GetText()
		text = text[1 : len(text)-1]
		// log.Infof("regexp: %s", text)
		reIns, err := regexp.Compile(text)
		if err != nil {
			log.Errorf("regexp compile failed: %v", err)
			return err
		}
		if recursive {
			y.EmitRecursiveSearchRegexp(mod, reIns.String())
		} else {
			y.EmitSearchRegexp(mod, reIns.String())
		}
		return nil
	}
	err := utils.Errorf("BUG: in nameFilter, unknown type: %s:%s", reflect.TypeOf(ret), ret.GetText())
	log.Errorf("%v", err)
	return err
}

func (y *SyntaxFlowVisitor) VisitNameFilter(isMember bool, i sf.INameFilterContext) (err error) {
	if i == nil {
		return nil
	}

	ret, ok := i.(*sf.NameFilterContext)
	if !ok {
		err := utils.Errorf("BUG: in nameFilter: %s", reflect.TypeOf(i))
		log.Errorf("%v", err)
		return err
	}

	mod := NameMatch
	if isMember {
		mod = KeyMatch
	}

	if s := ret.Star(); s != nil {
		if isMember {
			// get all member
			y.EmitSearchGlob(mod, "*")
		}
		// skip
		return nil
		// } else if id := ret.DollarOutput(); id != nil {
		// 	y.EmitSearchExact(mod, id.GetText())
		// 	return nil
	} else if id := ret.Identifier(); id != nil {
		text := ret.Identifier().GetText()
		filter, isGlob := y.FormatStringOrGlob(text) // emit field
		if isGlob {
			y.EmitSearchGlob(mod, filter)
		} else {
			y.EmitSearchExact(mod, filter)
		}
		return nil
	} else if re, ok := ret.RegexpLiteral().(*sf.RegexpLiteralContext); ok {
		text := re.RegexpLiteral().GetText()
		text = text[1 : len(text)-1]
		// log.Infof("regexp: %s", text)
		reIns, err := regexp.Compile(text)
		if err != nil {
			err := utils.Wrap(err, "regexp compile failed")
			log.Errorf("%v", err)
			return err
		}
		y.EmitSearchRegexp(mod, reIns.String())
		return nil
	}
	err = utils.Errorf("BUG: in nameFilter, unknown type: %s:%s", reflect.TypeOf(ret), ret.GetText())
	log.Error(err)
	return err
}

func (y *SyntaxFlowVisitor) VisitActualParam(i sf.IActualParamContext, haveQuestion bool) error {
	handlerStatement := func(i sf.ISingleParamContext) {
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
		iteratorCtx := y.EmitCreateIterator()
		y.EmitNextIterator(iteratorCtx)
		statement := y.EmitEnterStatement()
		y.EmitPushCallArgs(0, true)
		handlerStatement(ret.SingleParam())
		y.EmitExitStatement(statement)
		if haveQuestion {
			y.EmitOpPopDuplicate()
			y.EmitOpCheckEmpty(iteratorCtx)
		}
		y.EmitLatchIterator(iteratorCtx)
		y.EmitIterEnd(iteratorCtx)
	case *sf.EveryParamContext:
		iterator := y.EmitCreateIterator()
		y.EmitNextIterator(iterator)
		for i, paraI := range ret.AllActualParamFilter() {
			para, ok := paraI.(*sf.ActualParamFilterContext)
			if !ok {
				continue
			}
			single := para.SingleParam()
			if single == nil {
				continue
			}
			statement := y.EmitEnterStatement()
			y.EmitPushCallArgs(i, false)
			handlerStatement(single)
			y.EmitExitStatement(statement)
			if haveQuestion {
				y.EmitOpPopDuplicate()
				y.EmitOpCheckEmpty(iterator)
			}
		}
		if ret.SingleParam() != nil { // the last one get continue other value
			statement := y.EmitEnterStatement()
			y.EmitPushCallArgs(len(ret.AllActualParamFilter()), true)
			handlerStatement(ret.SingleParam())
			y.EmitExitStatement(statement)
			if haveQuestion {
				y.EmitOpPopDuplicate()
				y.EmitOpCheckEmpty(iterator)
			}
		}
		y.EmitLatchIterator(iterator)
		y.EmitIterEnd(iterator)
	default:
		return utils.Errorf("BUG: ActualParamFilter type error: %s", reflect.TypeOf(ret))
	}
	return nil
}
