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
	// A deferred file-filter field call not followed by a call is a plain
	// member access (e.g. `$a.regexp` alone) — emit it now.
	if y.pendingFileFilterField != nil {
		if err := y.VisitNameFilter(true, y.pendingFileFilterField); err != nil {
			return err
		}
		y.pendingFileFilterField = nil
	}
	return nil
}

func (y *SyntaxFlowVisitor) VisitFilterItem(raw sf.IFilterItemContext) error {
	// Flush a deferred file-filter field call when the next item is not a
	// FunctionCallFilter (e.g. `$a.regexp + $b` → member access on $a).
	if _, isCall := raw.(*sf.FunctionCallFilterContext); !isCall && y.pendingFileFilterField != nil {
		if err := y.VisitNameFilter(true, y.pendingFileFilterField); err != nil {
			return err
		}
		y.pendingFileFilterField = nil
	}
	switch filter := raw.(type) {
	case *sf.FirstContext:
		y.VisitFilterItemFirst(filter.FilterItemFirst())
	case *sf.FunctionCallFilterContext:
		// Chained context search: $a.regexp(/x/) — the deferred field call is
		// consumed here and emitted as a FileFilter op on the hit values.
		if y.pendingFileFilterField != nil {
			method := strings.ToLower(y.pendingFileFilterField.GetText())
			y.pendingFileFilterField = nil
			params, err := y.extractFileFilterParams(filter.ActualParam())
			if err != nil {
				return err
			}
			paramMap := make(map[string]string)
			switch method {
			case "pattern_regex_not", "pattern-regex-not", "patternregexnot":
				paramMap["__sf_pattern_not_list"] = "1"
			case "pattern_not_regex", "pattern-not-regex", "patternnotregex", "not_regexp", "not_re":
				paramMap["__sf_pattern_not"] = "1"
			}
			y.EmitFileFilterReg("", paramMap, params)
			return nil
		}
		//先拿到所有的call，然后再去拿callArgs
		y.EmitGetCall()
		if filter.Question() != nil {
			// Call-arg filtering relies on grouping by the parent call; enable anchor scope
			// so args can map back to their originating call via AnchorBitVector.
			y.EmitAnchorScopeStart()
			y.EmitOpEmptyCompare()
			if filter.ActualParam() != nil {
				y.VisitActualParam(filter.ActualParam(), true)
			} else {
				// no actual-param filter: keep original call filtering behavior
				y.EmitCondition()
			}
			y.EmitAnchorScopeEnd()
		} else if filter.ActualParam() != nil {
			// Plain arg extraction still needs per-call grouping so nested filters like
			// `*<slice(...)>` run against each call's arg set instead of the flattened union.
			y.EmitAnchorScopeStart()
			y.VisitActualParam(filter.ActualParam(), false)
			y.EmitAnchorScopeEnd()
		}
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
		y.EmitAnchorScopeStart()
		y.VisitConditionExpression(filter.ConditionExpression())
		y.EmitCondition()
		y.EmitAnchorScopeEnd()
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
	case *sf.InsideRefFilterContext:
		y.EmitInsideRef(strings.TrimLeft(filter.RefVariable().GetText(), "$"))
	case *sf.NotInsideRefFilterContext:
		y.EmitNotInsideRef(strings.TrimLeft(filter.RefVariable().GetText(), "$"))
	case *sf.VersionInFilterContext:
		y.EmitAnchorScopeStart()
		if versionIn := filter.VersionInExpression(); versionIn != nil {
			y.VisitVersionInExpression(versionIn)
		}
		y.EmitCondition()
		y.EmitAnchorScopeEnd()
	default:
		panic("BUG: in filterExpr")
	}
	return nil
}

// extractFileFilterParams collects regex sources from a FunctionCallFilter's
// actualParam for the chained file-filter form ($a.regexp(/x/, /y/)). Each
// singleParam must be a simple regex literal, quoted string, identifier, star,
// or heredoc; anything else is a compile error.
func (y *SyntaxFlowVisitor) extractFileFilterParams(actualParam sf.IActualParamContext) ([]string, error) {
	var out []string
	if actualParam == nil {
		return out, nil
	}
	collect := func(sp sf.ISingleParamContext) error {
		single, ok := sp.(*sf.SingleParamContext)
		if !ok || single == nil {
			return nil
		}
		if single.ConditionExpression() != nil {
			return utils.Errorf("chained file filter: condition-expression params unsupported")
		}
		fs := single.FilterStatement()
		if fs == nil {
			return nil
		}
		var filterExpr sf.IFilterExprContext
		switch st := fs.(type) {
		case *sf.PureFilterExprContext:
			filterExpr = st.FilterExpr()
		case *sf.RefFilterExprContext:
			return utils.Errorf("chained file filter: ref params unsupported")
		default:
			return utils.Errorf("chained file filter: unsupported param statement")
		}
		if filterExpr == nil {
			return nil
		}
		first := filterExpr.FilterItemFirst()
		if first == nil {
			return nil
		}
		switch ff := first.(type) {
		case *sf.NamedFilterContext:
			nf := ff.NameFilter()
			if nf == nil {
				return nil
			}
			if nf.RegexpLiteral() != nil {
				text := nf.RegexpLiteral().GetText()
				out = append(out, text[1:len(text)-1])
			} else if nf.Identifier() != nil {
				out = append(out, yakunquote.TryUnquote(nf.Identifier().GetText()))
			} else if nf.Star() != nil {
				out = append(out, "*")
			}
		case *sf.ConstFilterContext:
			if ff.QuotedStringLiteral() != nil {
				out = append(out, yakunquote.TryUnquote(ff.QuotedStringLiteral().GetText()))
			} else if ff.HereDoc() != nil {
				out = append(out, y.VisitHereDoc(ff.HereDoc()))
			}
		default:
			return utils.Errorf("chained file filter: unsupported param form")
		}
		return nil
	}
	switch ap := actualParam.(type) {
	case *sf.AllParamContext:
		if ap.SingleParam() != nil {
			if err := collect(ap.SingleParam()); err != nil {
				return nil, err
			}
		}
	case *sf.EveryParamContext:
		for _, p := range ap.AllActualParamFilter() {
			if pf, ok := p.(*sf.ActualParamFilterContext); ok && pf.SingleParam() != nil {
				if err := collect(pf.SingleParam()); err != nil {
					return nil, err
				}
			}
		}
		if ap.SingleParam() != nil {
			if err := collect(ap.SingleParam()); err != nil {
				return nil, err
			}
		}
	default:
		return nil, utils.Errorf("chained file filter: unsupported actualParam form")
	}
	return out, nil
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
		// Chained context search: $a.regexp(/x/) — a field call whose name is a
		// file-filter method is deferred; if the next filterItem is a
		// FunctionCallFilter it becomes a chained FileFilter op, otherwise it
		// falls back to a plain member access (flushed at expression end).
		if isFileFilterMethod(i.NameFilter().GetText()) {
			y.pendingFileFilterField = i.NameFilter()
			return nil
		}
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
	if !haveQuestion {
		switch ret := i.(type) {
		case *sf.AllParamContext:
			statement := y.EmitEnterStatement()
			if sp, ok := ret.SingleParam().(*sf.SingleParamContext); ok && sp != nil && sp.FilterStatement() != nil {
				y.EmitPushCallArgs(0, true)
				y.VisitFilterStatement(sp.FilterStatement())
			}
			y.EmitExitStatement(statement)
			return nil
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
				if sp, ok := single.(*sf.SingleParamContext); ok && sp != nil && sp.FilterStatement() != nil {
					y.EmitPushCallArgs(i, false)
					y.VisitFilterStatement(sp.FilterStatement())
				}
				y.EmitExitStatement(statement)
			}
			if ret.SingleParam() != nil {
				statement := y.EmitEnterStatement()
				if sp, ok := ret.SingleParam().(*sf.SingleParamContext); ok && sp != nil && sp.FilterStatement() != nil {
					y.EmitPushCallArgs(len(ret.AllActualParamFilter()), true)
					y.VisitFilterStatement(sp.FilterStatement())
				}
				y.EmitExitStatement(statement)
			}
			return nil
		default:
			return utils.Errorf("BUG: ActualParamFilter type error: %s", reflect.TypeOf(ret))
		}
	}

	var visitCallArgConditionExpression func(expr sf.IConditionExpressionContext, argStart int, containOther bool) error
	visitCallArgConditionExpression = func(expr sf.IConditionExpressionContext, argStart int, containOther bool) error {
		if y == nil || expr == nil {
			return nil
		}
		switch c := expr.(type) {
		case *sf.FilterExpressionAndContext:
			conds := c.AllConditionExpression()
			for idx, exp := range conds {
				y.EmitAnchorScopeStart()
				if err := visitCallArgConditionExpression(exp, argStart, containOther); err != nil {
					return err
				}
				y.EmitAnchorScopeEnd()
				if idx > 0 {
					y.EmitOperator("&&")
				}
			}
			return nil
		case *sf.FilterExpressionOrContext:
			conds := c.AllConditionExpression()
			for idx, exp := range conds {
				y.EmitAnchorScopeStart()
				if err := visitCallArgConditionExpression(exp, argStart, containOther); err != nil {
					return err
				}
				y.EmitAnchorScopeEnd()
				if idx > 0 {
					y.EmitOperator("||")
				}
			}
			return nil
		case *sf.NotConditionContext:
			y.EmitAnchorScopeStart()
			if err := visitCallArgConditionExpression(c.ConditionExpression(), argStart, containOther); err != nil {
				return err
			}
			y.EmitAnchorScopeEnd()
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
			y.EmitAnchorScopeStart()
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
			y.EmitAnchorScopeEnd()
			y.EmitFilter()
			return nil
		case *sf.OpcodeTypeConditionContext:
			opcodes := c.AllOpcodesCondition()
			ops := make([]string, 0, len(opcodes))
			for _, opcode := range opcodes {
				ops = append(ops, opcode.GetText())
			}
			y.EmitPushCallArgs(argStart, containOther)
			y.EmitAnchorScopeStart()
			y.EmitCompareOpcode(ops)
			y.EmitCondition()
			y.EmitAnchorScopeEnd()
			y.EmitFilter()
			return nil
		case *sf.StringContainAnyConditionContext:
			res := y.VisitStringLiteralWithoutStarGroup(c.StringLiteralWithoutStarGroup())
			y.EmitPushCallArgs(argStart, containOther)
			y.EmitAnchorScopeStart()
			y.EmitCompareString(res, MatchHaveAny)
			y.EmitCondition()
			y.EmitAnchorScopeEnd()
			y.EmitFilter()
			return nil
		case *sf.StringContainHaveConditionContext:
			res := y.VisitStringLiteralWithoutStarGroup(c.StringLiteralWithoutStarGroup())
			y.EmitPushCallArgs(argStart, containOther)
			y.EmitAnchorScopeStart()
			y.EmitCompareString(res, MatchHave)
			y.EmitCondition()
			y.EmitAnchorScopeEnd()
			y.EmitFilter()
			return nil
		default:
			// Fallback: treat as an arg-level condition expression, then lift to call-level via OpFilter.
			y.EmitPushCallArgs(argStart, containOther)
			y.EmitAnchorScopeStart()
			y.VisitConditionExpression(expr)
			y.EmitCondition()
			y.EmitAnchorScopeEnd()
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
