package csharp2ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// 模式匹配：`x is T t`、switch 语句/表达式中的 case 模式。

type patternBinding struct {
	name  string
	value ssa.Value
}

type patternMatchResult struct {
	condition ssa.Value
	bindings  []patternBinding
}

// emitPatternMatch returns the condition for `value matches pattern`. Member
// subpatterns are emitted behind real CFG gates and designations are assigned
// only on the successful edge of the complete pattern.
func (b *singleFileBuilder) emitPatternMatch(value ssa.Value, raw csharpparser.IPatternContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return b.EmitConstInst(true)
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.PatternContext)
	if !ok || i == nil {
		return b.EmitConstInst(true)
	}
	if utils.IsNil(value) {
		value = b.EmitUndefined("subject")
	}
	result := b.emitPatternMatchRaw(value, i)
	b.rememberPatternBindings(result.condition, result.bindings)
	return result.condition
}

func mergePatternBindings(groups ...[]patternBinding) []patternBinding {
	var merged []patternBinding
	positions := make(map[string]int)
	for _, group := range groups {
		for _, binding := range group {
			if binding.name == "" || binding.name == "_" {
				continue
			}
			if index, exists := positions[binding.name]; exists {
				merged[index] = binding
				continue
			}
			positions[binding.name] = len(merged)
			merged = append(merged, binding)
		}
	}
	return merged
}

func (b *singleFileBuilder) rememberPatternBindings(condition ssa.Value, bindings []patternBinding) {
	if b == nil || utils.IsNil(condition) || len(bindings) == 0 {
		return
	}
	if b.patternConditionBindings == nil {
		b.patternConditionBindings = make(map[int64][]patternBinding)
	}
	b.patternConditionBindings[condition.GetId()] = mergePatternBindings(bindings)
}

func (b *singleFileBuilder) patternBindingsFor(condition ssa.Value) []patternBinding {
	if b == nil || utils.IsNil(condition) {
		return nil
	}
	return append([]patternBinding(nil), b.patternConditionBindings[condition.GetId()]...)
}

// emitPatternMatchRaw builds a pattern condition and records its prospective
// bindings without assigning source-level variables. The caller performs those
// assignments only after the complete pattern succeeds.
func (b *singleFileBuilder) emitPatternMatchRaw(value ssa.Value, raw csharpparser.IPatternContext) patternMatchResult {
	i, ok := raw.(*csharpparser.PatternContext)
	if !ok || i == nil {
		return patternMatchResult{condition: b.EmitConstInst(true)}
	}
	if utils.IsNil(value) {
		value = b.EmitUndefined("subject")
	}
	// The grammar's constant-pattern predicate also accepts declaration
	// expressions, so both `is T x` and `case var x` can arrive wrapped in a
	// Constant_patternContext. Normalize that recovered shape before treating it
	// as a value comparison.
	if declaration := declarationExpressionPattern(raw); declaration != nil {
		typ := b.VisitLocalVariableType(declaration.Local_variable_type())
		bound := value
		if typ != nil {
			bound = b.castForPattern(value, typ)
		}
		bindings := namedPatternBinding(identText(declaration.Identifier()), bound)
		if typ == nil {
			return patternMatchResult{condition: b.EmitConstInst(true), bindings: bindings}
		}
		return patternMatchResult{condition: b.emitIsType(value, typ), bindings: bindings}
	}
	switch {
	case i.Constant_pattern() != nil:
		cp, _ := i.Constant_pattern().(*csharpparser.Constant_patternContext)
		if cp == nil || cp.Constant_expression() == nil {
			return patternMatchResult{condition: b.EmitConstInst(true)}
		}
		c := b.VisitExpression(cp.Constant_expression().Expression())
		if utils.IsNil(c) {
			return patternMatchResult{condition: b.EmitConstInst(true)}
		}
		return patternMatchResult{condition: b.EmitBinOp(ssa.OpEq, value, c)}
	case i.Declaration_pattern() != nil:
		dp, _ := i.Declaration_pattern().(*csharpparser.Declaration_patternContext)
		if dp == nil {
			return patternMatchResult{condition: b.EmitConstInst(true)}
		}
		typ := b.VisitType(dp.Type_())
		return patternMatchResult{
			condition: b.emitIsType(value, typ),
			bindings:  b.simpleDesignationBindings(dp.Simple_designation(), b.castForPattern(value, typ)),
		}
	case i.Var_pattern() != nil:
		vp, _ := i.Var_pattern().(*csharpparser.Var_patternContext)
		if vp == nil {
			return patternMatchResult{condition: b.EmitConstInst(true)}
		}
		return patternMatchResult{condition: b.EmitConstInst(true), bindings: b.designationBindings(vp.Designation(), value)}
	case i.Positional_pattern() != nil:
		return b.emitPositionalPatternRaw(value, i.Positional_pattern())
	case i.Property_pattern() != nil:
		return b.emitPropertyPatternRaw(value, i.Property_pattern())
	case i.Discard_pattern() != nil:
		return patternMatchResult{condition: b.EmitConstInst(true)}
	}
	return patternMatchResult{condition: b.EmitConstInst(true)}
}

func (b *singleFileBuilder) castForPattern(value ssa.Value, typ ssa.Type) ssa.Value {
	if typ == nil || typ.GetTypeKind() == ssa.AnyTypeKind {
		return value
	}
	return b.EmitTypeCast(value, typ)
}

func boolConstant(value ssa.Value) (bool, bool) {
	constant, ok := ssa.ToConstInst(value)
	if !ok || !constant.IsBoolean() {
		return false, false
	}
	return constant.Boolean(), true
}

// andPatternCondition emits next only on cur's true edge. The temporary phi is
// also the value returned to an enclosing pattern or switch guard.
func (b *singleFileBuilder) andPatternCondition(cur ssa.Value, next func() ssa.Value) ssa.Value {
	if utils.IsNil(cur) {
		return next()
	}
	if value, ok := boolConstant(cur); ok {
		if !value {
			return cur
		}
		return next()
	}
	return b.binJump(func() ssa.Value { return cur }, next, ssa.AndExpressionVariable, true)
}

func (b *singleFileBuilder) appendPatternResult(result patternMatchResult, next func() patternMatchResult) patternMatchResult {
	if value, ok := boolConstant(result.condition); ok && !value {
		return result
	}
	var following patternMatchResult
	result.condition = b.andPatternCondition(result.condition, func() ssa.Value {
		following = next()
		return following.condition
	})
	result.bindings = append(result.bindings, following.bindings...)
	return result
}

func (b *singleFileBuilder) emitPositionalPatternRaw(value ssa.Value, raw csharpparser.IPositional_patternContext) patternMatchResult {
	i, _ := raw.(*csharpparser.Positional_patternContext)
	if i == nil {
		return patternMatchResult{condition: b.EmitConstInst(true)}
	}
	result := patternMatchResult{condition: b.EmitConstInst(true)}
	subject := value
	if i.Type_() != nil {
		typ := b.VisitType(i.Type_())
		result.condition = b.emitIsType(value, typ)
		subject = b.castForPattern(value, typ)
	}
	if sps, _ := i.Subpatterns().(*csharpparser.SubpatternsContext); sps != nil {
		for idx, sp := range sps.AllSubpattern() {
			spc, _ := sp.(*csharpparser.SubpatternContext)
			if spc == nil {
				continue
			}
			index := idx
			result = b.appendPatternResult(result, func() patternMatchResult {
				var key ssa.Value
				if spc.Identifier() != nil {
					key = b.EmitConstInstPlaceholder(identText(spc.Identifier()))
				} else {
					key = b.EmitConstInst(index)
				}
				element := b.ReadMemberCallValue(subject, key)
				return b.emitPatternMatchRaw(element, spc.Pattern())
			})
		}
	}
	if i.Property_subpattern() != nil {
		result = b.appendPatternResult(result, func() patternMatchResult {
			return b.emitPropertySubpatternRaw(subject, i.Property_subpattern())
		})
	}
	result.bindings = append(result.bindings, b.simpleDesignationBindings(i.Simple_designation(), subject)...)
	return result
}

func (b *singleFileBuilder) emitPropertyPatternRaw(value ssa.Value, raw csharpparser.IProperty_patternContext) patternMatchResult {
	i, _ := raw.(*csharpparser.Property_patternContext)
	if i == nil {
		return patternMatchResult{condition: b.EmitConstInst(true)}
	}
	result := patternMatchResult{condition: b.EmitConstInst(true)}
	subject := value
	if i.Type_() != nil {
		typ := b.VisitType(i.Type_())
		result.condition = b.emitIsType(value, typ)
		subject = b.castForPattern(value, typ)
	} else {
		result.condition = b.EmitBinOp(ssa.OpNotEq, value, b.EmitConstInstNil())
	}
	result = b.appendPatternResult(result, func() patternMatchResult {
		return b.emitPropertySubpatternRaw(subject, i.Property_subpattern())
	})
	result.bindings = append(result.bindings, b.simpleDesignationBindings(i.Simple_designation(), subject)...)
	return result
}

func (b *singleFileBuilder) emitPropertySubpatternRaw(subject ssa.Value, raw csharpparser.IProperty_subpatternContext) patternMatchResult {
	i, _ := raw.(*csharpparser.Property_subpatternContext)
	if i == nil {
		return patternMatchResult{condition: b.EmitConstInst(true)}
	}
	sps, _ := i.Subpatterns().(*csharpparser.SubpatternsContext)
	if sps == nil {
		return patternMatchResult{condition: b.EmitConstInst(true)}
	}
	result := patternMatchResult{condition: b.EmitConstInst(true)}
	for _, sp := range sps.AllSubpattern() {
		spc, _ := sp.(*csharpparser.SubpatternContext)
		if spc == nil {
			continue
		}
		result = b.appendPatternResult(result, func() patternMatchResult {
			var element ssa.Value
			if spc.Identifier() != nil {
				element = b.readMember(subject, identText(spc.Identifier()), false)
			} else {
				element = subject
			}
			return b.emitPatternMatchRaw(element, spc.Pattern())
		})
	}
	return result
}

func namedPatternBinding(name string, value ssa.Value) []patternBinding {
	if name == "" || name == "_" {
		return nil
	}
	return []patternBinding{{name: name, value: value}}
}

func (b *singleFileBuilder) simpleDesignationBindings(raw csharpparser.ISimple_designationContext, value ssa.Value) []patternBinding {
	i, _ := raw.(*csharpparser.Simple_designationContext)
	if i == nil {
		return nil
	}
	sv, _ := i.Single_variable_designation().(*csharpparser.Single_variable_designationContext)
	if sv == nil {
		return nil
	}
	return namedPatternBinding(identText(sv.Identifier()), value)
}

func (b *singleFileBuilder) designationBindings(raw csharpparser.IDesignationContext, value ssa.Value) []patternBinding {
	i, _ := raw.(*csharpparser.DesignationContext)
	if i == nil {
		return nil
	}
	if i.Simple_designation() != nil {
		return b.simpleDesignationBindings(i.Simple_designation(), value)
	}
	td, _ := i.Tuple_designation().(*csharpparser.Tuple_designationContext)
	if td == nil {
		return nil
	}
	ds, _ := td.Designations().(*csharpparser.DesignationsContext)
	if ds == nil {
		return nil
	}
	var bindings []patternBinding
	for idx, d := range ds.AllDesignation() {
		element := b.ReadMemberCallValue(value, b.EmitConstInst(idx))
		bindings = append(bindings, b.designationBindings(d, element)...)
	}
	return bindings
}

func (b *singleFileBuilder) bindPatternBindings(bindings []patternBinding) {
	b.assignPatternBindings(bindings, true)
}

func (b *singleFileBuilder) assignPatternBindings(bindings []patternBinding, createLocal bool) {
	for _, binding := range bindings {
		if binding.name == "" || binding.name == "_" {
			continue
		}
		value := binding.value
		if utils.IsNil(value) {
			value = b.EmitUndefined(binding.name)
		}
		variable := b.CreateVariable(binding.name)
		if createLocal {
			// A designation introduces a new local. CreateVariable treats an
			// assignment from a parameter/free value as a non-local write.
			variable = b.CreateLocalVariable(binding.name)
		}
		b.AssignVariable(variable, value)
	}
}

func (b *singleFileBuilder) bindPatternResultOnSuccess(result patternMatchResult) ssa.Value {
	if len(result.bindings) == 0 {
		return result.condition
	}
	if value, ok := boolConstant(result.condition); ok {
		if value {
			b.bindPatternBindings(result.bindings)
		}
		return result.condition
	}
	// IfBuilder only phi-merges variables that exist in its entry scope. Seed
	// each designation with a value-only declaration, then overwrite it solely
	// on the successful edge. This keeps `is T x && use(x)` connected to x while
	// the false edge retains an unassigned placeholder.
	seen := make(map[string]struct{}, len(result.bindings))
	for _, binding := range result.bindings {
		if binding.name == "" || binding.name == "_" {
			continue
		}
		if _, exists := seen[binding.name]; exists {
			continue
		}
		seen[binding.name] = struct{}{}
		b.AssignVariable(b.CreateLocalVariable(binding.name), b.EmitValueOnlyDeclare(binding.name))
	}
	return b.andPatternCondition(result.condition, func() ssa.Value {
		// The local slots were seeded in the gate's entry scope above. Update
		// those same variables so IfBuilder can merge their successful values.
		b.assignPatternBindings(result.bindings, false)
		return b.EmitConstInst(true)
	})
}

// catchAllPatternBindings handles default-routed `var x` patterns without
// evaluating the pattern a second time in the switch body.
func (b *singleFileBuilder) catchAllPatternBindings(value ssa.Value, raw csharpparser.IPatternContext) []patternBinding {
	if declaration := declarationExpressionPattern(raw); declaration != nil {
		return namedPatternBinding(identText(declaration.Identifier()), value)
	}
	i, _ := raw.(*csharpparser.PatternContext)
	if i == nil {
		return nil
	}
	if vp, _ := i.Var_pattern().(*csharpparser.Var_patternContext); vp != nil {
		return b.designationBindings(vp.Designation(), value)
	}
	return nil
}

// patternDeclaresVariables reports whether the pattern binds any variable name.
func patternDeclaresVariables(raw csharpparser.IPatternContext) bool {
	if declaration := declarationExpressionPattern(raw); declaration != nil {
		name := identText(declaration.Identifier())
		return name != "" && name != "_"
	}
	i, _ := raw.(*csharpparser.PatternContext)
	if i == nil {
		return false
	}
	switch {
	case i.Declaration_pattern() != nil:
		dp, _ := i.Declaration_pattern().(*csharpparser.Declaration_patternContext)
		return dp != nil && simpleDesignationNamed(dp.Simple_designation())
	case i.Var_pattern() != nil:
		return true
	case i.Positional_pattern() != nil:
		pp, _ := i.Positional_pattern().(*csharpparser.Positional_patternContext)
		if pp == nil {
			return false
		}
		if simpleDesignationNamed(pp.Simple_designation()) {
			return true
		}
		if sps, _ := pp.Subpatterns().(*csharpparser.SubpatternsContext); sps != nil {
			for _, sp := range sps.AllSubpattern() {
				if spc, _ := sp.(*csharpparser.SubpatternContext); spc != nil && patternDeclaresVariables(spc.Pattern()) {
					return true
				}
			}
		}
		return propertySubpatternDeclares(pp.Property_subpattern())
	case i.Property_pattern() != nil:
		pp, _ := i.Property_pattern().(*csharpparser.Property_patternContext)
		if pp == nil {
			return false
		}
		return simpleDesignationNamed(pp.Simple_designation()) || propertySubpatternDeclares(pp.Property_subpattern())
	}
	return false
}

func propertySubpatternDeclares(raw csharpparser.IProperty_subpatternContext) bool {
	i, _ := raw.(*csharpparser.Property_subpatternContext)
	if i == nil {
		return false
	}
	sps, _ := i.Subpatterns().(*csharpparser.SubpatternsContext)
	if sps == nil {
		return false
	}
	for _, sp := range sps.AllSubpattern() {
		if spc, _ := sp.(*csharpparser.SubpatternContext); spc != nil && patternDeclaresVariables(spc.Pattern()) {
			return true
		}
	}
	return false
}

func simpleDesignationNamed(raw csharpparser.ISimple_designationContext) bool {
	i, _ := raw.(*csharpparser.Simple_designationContext)
	if i == nil {
		return false
	}
	sv, _ := i.Single_variable_designation().(*csharpparser.Single_variable_designationContext)
	return sv != nil && identText(sv.Identifier()) != "" && identText(sv.Identifier()) != "_"
}

// isConstantPattern reports whether a pattern is a plain constant (usable as a switch case value).
func isConstantPattern(raw csharpparser.IPatternContext) bool {
	i, _ := raw.(*csharpparser.PatternContext)
	return i != nil && i.Constant_pattern() != nil && declarationExpressionPattern(raw) == nil
}

// isCatchAllPattern reports whether the pattern always matches (`_` or `var x`).
func isCatchAllPattern(raw csharpparser.IPatternContext) bool {
	if declaration := declarationExpressionPattern(raw); declaration != nil {
		localType, _ := declaration.Local_variable_type().(*csharpparser.Local_variable_typeContext)
		return localType != nil && localType.KW_VAR() != nil
	}
	i, _ := raw.(*csharpparser.PatternContext)
	if i == nil {
		return false
	}
	// The current grammar can recover a standalone discard as a constant
	// identifier pattern. In switch expressions it is nevertheless the C#
	// catch-all arm, so recognize that recovery shape as well.
	if i.GetText() == "_" {
		return true
	}
	if i.Discard_pattern() != nil {
		return true
	}
	if vp, _ := i.Var_pattern().(*csharpparser.Var_patternContext); vp != nil {
		if d, _ := vp.Designation().(*csharpparser.DesignationContext); d != nil && d.Simple_designation() != nil {
			return true
		}
	}
	return false
}

// declarationExpressionPattern unwraps the parse-recovery shape produced for
// declaration patterns by the current grammar:
//
//	pattern -> constant_pattern -> constant_expression -> expression
//	        -> non_assignment_expression -> declaration_expression
func declarationExpressionPattern(raw csharpparser.IPatternContext) *csharpparser.Declaration_expressionContext {
	i, _ := raw.(*csharpparser.PatternContext)
	if i == nil || i.Constant_pattern() == nil {
		return nil
	}
	cp, _ := i.Constant_pattern().(*csharpparser.Constant_patternContext)
	if cp == nil || cp.Constant_expression() == nil || cp.Constant_expression().Expression() == nil {
		return nil
	}
	nonAssignment := cp.Constant_expression().Expression().Non_assignment_expression()
	if nonAssignment == nil || nonAssignment.Declaration_expression() == nil {
		return nil
	}
	declaration, _ := nonAssignment.Declaration_expression().(*csharpparser.Declaration_expressionContext)
	return declaration
}

// ---------------------------------------------------------------- switch expression

func (b *singleFileBuilder) VisitSwitchExpression(raw csharpparser.ISwitch_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Switch_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.KW_SWITCH() == nil {
		if i.Range_expression() != nil {
			return b.VisitRangeExpression(i.Range_expression())
		}
		return b.VisitSwitchExpression(i.Switch_expression())
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()

	subject := b.VisitSwitchExpression(i.Switch_expression())
	if utils.IsNil(subject) {
		subject = b.EmitUndefined("switch")
	}
	var arms []*csharpparser.Switch_expression_armContext
	if as, _ := i.Switch_expression_arms().(*csharpparser.Switch_expression_armsContext); as != nil {
		for _, a := range as.AllSwitch_expression_arm() {
			if ac, _ := a.(*csharpparser.Switch_expression_armContext); ac != nil {
				arms = append(arms, ac)
			}
		}
	}
	id := ssa.TernaryExpressionVariable
	b.AssignVariable(b.CreateVariable(id), b.EmitValueOnlyDeclare(id))

	var defaultArm *csharpparser.Switch_expression_armContext
	var caseArms []*csharpparser.Switch_expression_armContext
	for _, arm := range arms {
		if defaultArm == nil && arm.Case_guard() == nil && isCatchAllPattern(arm.Pattern()) {
			defaultArm = arm
			continue
		}
		caseArms = append(caseArms, arm)
	}
	armBindings := make(map[*csharpparser.Switch_expression_armContext][]patternBinding, len(caseArms))
	allConst := true
	for _, arm := range caseArms {
		if arm.Case_guard() != nil || !isConstantPattern(arm.Pattern()) {
			allConst = false
			break
		}
	}

	assignResult := func(arm *csharpparser.Switch_expression_armContext) {
		bindings := armBindings[arm]
		if arm == defaultArm && len(bindings) == 0 {
			bindings = b.catchAllPatternBindings(subject, arm.Pattern())
		}
		b.bindPatternBindings(bindings)
		var v ssa.Value
		if ae, _ := arm.Switch_expression_arm_expression().(*csharpparser.Switch_expression_arm_expressionContext); ae != nil {
			v = b.VisitExpression(ae.Expression())
		}
		if utils.IsNil(v) {
			v = b.EmitUndefined(id)
		}
		b.AssignVariable(b.CreateVariable(id), v)
	}

	sw := b.BuildSwitch()
	sw.AutoBreak = true
	if allConst {
		sw.BuildCondition(func() ssa.Value { return subject })
	} else {
		sw.BuildCondition(func() ssa.Value { return b.EmitConstInst(true) })
	}
	sw.BuildCaseSize(len(caseArms))
	sw.SetCase(func(idx int) []ssa.Value {
		arm := caseArms[idx]
		if allConst {
			return []ssa.Value{b.emitPatternConstant(arm.Pattern())}
		}
		condition, bindings := b.emitArmCondition(subject, arm.Pattern(), arm.Case_guard())
		armBindings[arm] = bindings
		return []ssa.Value{condition}
	})
	sw.BuildBody(func(idx int) {
		assignResult(caseArms[idx])
	})
	if defaultArm != nil {
		sw.BuildDefault(func() { assignResult(defaultArm) })
	} else {
		sw.BuildDefault(func() {
			b.AssignVariable(b.CreateVariable(id), b.EmitUndefined(id))
		})
	}
	sw.Finish()
	return b.ReadValue(id)
}

// emitPatternConstant evaluates a constant pattern to its value (for `switch (x) case C:`).
func (b *singleFileBuilder) emitPatternConstant(raw csharpparser.IPatternContext) ssa.Value {
	i, _ := raw.(*csharpparser.PatternContext)
	if i == nil {
		return b.EmitConstInst(true)
	}
	cp, _ := i.Constant_pattern().(*csharpparser.Constant_patternContext)
	if cp == nil || cp.Constant_expression() == nil {
		return b.EmitConstInst(true)
	}
	v := b.VisitExpression(cp.Constant_expression().Expression())
	if utils.IsNil(v) {
		return b.EmitConstInst(true)
	}
	return v
}

// emitArmCondition builds `pattern-match && guard` for a case arm. Pattern
// bindings are established on the match-success edge before a guard is
// evaluated, because the guard may reference those names.
func (b *singleFileBuilder) emitArmCondition(subject ssa.Value, pattern csharpparser.IPatternContext, guard csharpparser.ICase_guardContext) (ssa.Value, []patternBinding) {
	result := b.emitPatternMatchRaw(subject, pattern)
	cond := b.bindPatternResultOnSuccess(result)
	if g, _ := guard.(*csharpparser.Case_guardContext); g != nil && g.Null_coalescing_expression() != nil {
		cond = b.andPatternCondition(cond, func() ssa.Value {
			return b.VisitNullCoalescing(g.Null_coalescing_expression())
		})
	}
	if utils.IsNil(cond) {
		cond = b.EmitConstInst(true)
	}
	return cond, result.bindings
}
