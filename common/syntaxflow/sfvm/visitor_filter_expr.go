package sfvm

import (
	"reflect"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
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
		// Call-arg filtering relies on grouping by the parent call; enable condition scope
		// so args can map back to their originating call via AnchorBitVector.
		y.EmitConditionScopeStart()
		y.EmitOpEmptyCompare()
		if filter.ActualParam() != nil {
			y.VisitActualParam(filter.ActualParam(), filter.Question() != nil)
		} else {
			// no actual-param filter: keep original call filtering behavior
			y.EmitCondition()
		}
		y.EmitConditionScopeEnd()
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
		y.EmitConditionScopeStart()
		y.VisitConditionExpression(filter.ConditionExpression())
		y.EmitCondition()
		y.EmitConditionScopeEnd()
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
		y.EmitConditionScopeStart()
		if versionIn := filter.VersionInExpression(); versionIn != nil {
			y.VisitVersionInExpression(versionIn)
		}
		y.EmitCondition()
		y.EmitConditionScopeEnd()
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

	mod := ssadb.NameMatch
	if isMember {
		mod = ssadb.KeyMatch
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

	mod := ssadb.NameMatch
	if isMember {
		mod = ssadb.KeyMatch
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
	var visitCallArgConditionExpression func(expr sf.IConditionExpressionContext, argStart int, containOther bool) error
	visitCallArgConditionExpression = func(expr sf.IConditionExpressionContext, argStart int, containOther bool) error {
		if y == nil || expr == nil {
			return nil
		}
		switch c := expr.(type) {
		case *sf.FilterExpressionAndContext:
			conds := c.AllConditionExpression()
			for idx, exp := range conds {
				y.EmitConditionScopeStart()
				if err := visitCallArgConditionExpression(exp, argStart, containOther); err != nil {
					return err
				}
				y.EmitConditionScopeEnd()
				if idx > 0 {
					y.EmitOperator("&&")
				}
			}
			return nil
		case *sf.FilterExpressionOrContext:
			conds := c.AllConditionExpression()
			for idx, exp := range conds {
				y.EmitConditionScopeStart()
				if err := visitCallArgConditionExpression(exp, argStart, containOther); err != nil {
					return err
				}
				y.EmitConditionScopeEnd()
				if idx > 0 {
					y.EmitOperator("||")
				}
			}
			return nil
		case *sf.NotConditionContext:
			y.EmitConditionScopeStart()
			if err := visitCallArgConditionExpression(c.ConditionExpression(), argStart, containOther); err != nil {
				return err
			}
			y.EmitConditionScopeEnd()
			y.EmitOperator("!")
			return nil
		case *sf.ParenConditionContext:
			return visitCallArgConditionExpression(c.ConditionExpression(), argStart, containOther)

		// Leaf conditions in call-arg filter context (?(...)):
		// They are interpreted as "exists an actual-param derived value that satisfies this condition",
		// then mapped back to the parent call list via OpFilter to produce a call-level ConditionEntry.
		case *sf.FilterConditionContext:
			y.EmitPushCallArgs(argStart, containOther)
			if err := y.VisitFilterExpr(c.FilterExpr()); err != nil {
				return err
			}
			// Map derived values back to the parent call list (call anchor), not to args.
			y.EmitFilter()
			return nil
		case *sf.FilterExpressionBinaryCompareContext:
			y.EmitPushCallArgs(argStart, containOther)
			if err := y.VisitFilterExpr(c.FilterExpr()); err != nil {
				return err
			}
			// The filter-expr produces a derived list (e.g. `*<len>`). Compare/condition should be
			// evaluated on that derived list while preserving its anchor bits back to the call list,
			// so start the scope after the filter-expr has produced the derived values.
			y.EmitConditionScopeStart()
			if c.NumberLiteral() != nil {
				n := y.VisitNumberLiteral(c.NumberLiteral())
				y.EmitPushLiteral(n)
			} else if c.Identifier() != nil {
				y.EmitPushLiteral(yakunquote.TryUnquote(c.Identifier().GetText()))
			} else if c.BoolLiteral() != nil {
				if yakunquote.TryUnquote(c.BoolLiteral().GetText()) == "true" {
					y.EmitPushLiteral(true)
				} else {
					y.EmitPushLiteral(false)
				}
			}
			y.EmitOperator(c.GetOp().GetText())
			y.EmitCondition()
			y.EmitConditionScopeEnd()
			y.EmitFilter()
			return nil
		case *sf.OpcodeTypeConditionContext:
			opcodes := c.AllOpcodesCondition()
			ops := make([]string, 0, len(opcodes))
			for _, opcode := range opcodes {
				ops = append(ops, opcode.GetText())
			}
			y.EmitPushCallArgs(argStart, containOther)
			y.EmitConditionScopeStart()
			y.EmitCompareOpcode(ops)
			y.EmitCondition()
			y.EmitConditionScopeEnd()
			y.EmitFilter()
			return nil
		case *sf.StringContainAnyConditionContext:
			res := y.VisitStringLiteralWithoutStarGroup(c.StringLiteralWithoutStarGroup())
			y.EmitPushCallArgs(argStart, containOther)
			y.EmitConditionScopeStart()
			y.EmitCompareString(res, MatchHaveAny)
			y.EmitCondition()
			y.EmitConditionScopeEnd()
			y.EmitFilter()
			return nil
		case *sf.StringContainHaveConditionContext:
			res := y.VisitStringLiteralWithoutStarGroup(c.StringLiteralWithoutStarGroup())
			y.EmitPushCallArgs(argStart, containOther)
			y.EmitConditionScopeStart()
			y.EmitCompareString(res, MatchHave)
			y.EmitCondition()
			y.EmitConditionScopeEnd()
			y.EmitFilter()
			return nil
		default:
			// Fallback: treat as an arg-level condition expression, then lift to call-level via OpFilter.
			y.EmitPushCallArgs(argStart, containOther)
			y.EmitConditionScopeStart()
			y.VisitConditionExpression(expr)
			y.EmitCondition()
			y.EmitConditionScopeEnd()
			y.EmitFilter()
			return nil
		}
	}

	handleConditionExpression := func(single sf.ISingleParamContext, argStart int, containOther bool) bool {
		ret, ok := single.(*sf.SingleParamContext)
		if !ok || ret == nil || ret.ConditionExpression() == nil {
			return false
		}
		_ = haveQuestion
		// In call-arg filter context, condition expressions are lifted to call-level conditions.
		// This makes `a?(xx && yy)` composable and allows mixed expressions like `*<len>==2 && opcode:function`.
		_ = visitCallArgConditionExpression(ret.ConditionExpression(), argStart, containOther)
		return true
	}
	switch ret := i.(type) {
	case *sf.AllParamContext:
		statement := y.EmitEnterStatement()
		if sp, ok := ret.SingleParam().(*sf.SingleParamContext); ok && sp != nil {
			if sp.FilterStatement() != nil {
				y.EmitPushCallArgs(0, true)
				y.VisitFilterStatement(sp.FilterStatement())
				y.EmitOpPopDuplicate()
				y.EmitFilter()
				y.EmitCondition()
			} else if handleConditionExpression(sp, 0, true) {
				y.EmitCondition()
			}
		}
		y.EmitExitStatement(statement)
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
			statement := y.EmitEnterStatement()
			if sp, ok := single.(*sf.SingleParamContext); ok && sp != nil {
				if sp.FilterStatement() != nil {
					y.EmitPushCallArgs(i, false)
					y.VisitFilterStatement(sp.FilterStatement())
					y.EmitOpPopDuplicate()
					y.EmitFilter()
					y.EmitCondition()
				} else if handleConditionExpression(sp, i, false) {
					y.EmitCondition()
				}
			}
			y.EmitExitStatement(statement)
		}
		if ret.SingleParam() != nil { // the last one get continue other value
			statement := y.EmitEnterStatement()
			if sp, ok := ret.SingleParam().(*sf.SingleParamContext); ok && sp != nil {
				if sp.FilterStatement() != nil {
					y.EmitPushCallArgs(len(ret.AllActualParamFilter()), true)
					y.VisitFilterStatement(sp.FilterStatement())
					y.EmitOpPopDuplicate()
					y.EmitFilter()
					y.EmitCondition()
				} else if handleConditionExpression(sp, len(ret.AllActualParamFilter()), true) {
					y.EmitCondition()
				}
			}
			y.EmitExitStatement(statement)
		}
	default:
		return utils.Errorf("BUG: ActualParamFilter type error: %s", reflect.TypeOf(ret))
	}
	return nil
}
