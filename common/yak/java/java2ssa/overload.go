package java2ssa

import (
	"fmt"
	"sort"
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type javaParamSignature struct {
	key string
	typ ssa.Type
}

type javaCallableCandidate struct {
	function     *ssa.Function
	paramTypeKey []string
	paramType    []ssa.Type
	variadic     bool
	order        int
}

func (y *singleFileBuilder) nextJavaStableTemp(prefix string) string {
	y.stableTempCounter++
	if prefix == "" {
		prefix = "tmp"
	}
	return fmt.Sprintf("%s_%d", prefix, y.stableTempCounter)
}

func (y *singleFileBuilder) nextJavaAnonymousClassName(parentName string) string {
	y.stableAnonClassSeq++
	base := sanitizeStableNamePart(parentName)
	if base == "" {
		base = "anonymous_class"
	}
	return fmt.Sprintf("%s$anonymous_%d", base, y.stableAnonClassSeq)
}

func (y *singleFileBuilder) javaStableCallableName(
	kind string,
	class *ssa.Blueprint,
	methodName string,
	params []javaParamSignature,
	variadic bool,
	raw antlr.ParserRuleContext,
) string {
	ownerName := "unknown_owner"
	if class != nil && class.Name != "" {
		ownerName = class.Name
	}
	paramKey := make([]string, 0, len(params))
	for _, param := range params {
		paramKey = append(paramKey, param.key)
	}
	signature := strings.Join(paramKey, ",")
	if variadic {
		signature += "|variadic"
	}
	seed := strings.Join([]string{
		kind,
		ownerName,
		methodName,
		signature,
		javaContextPosKey(raw),
	}, "|")
	hash := utils.CalcSha1(seed)
	if len(hash) > 12 {
		hash = hash[:12]
	}
	base := fmt.Sprintf(
		"%s_%s_%s",
		sanitizeStableNamePart(ownerName),
		sanitizeStableNamePart(methodName),
		hash,
	)
	suffix := y.stableNameCollision[base]
	y.stableNameCollision[base] = suffix + 1
	if suffix > 0 {
		return fmt.Sprintf("%s_%d", base, suffix)
	}
	return base
}

func sanitizeStableNamePart(raw string) string {
	if raw == "" {
		return "unnamed"
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	text := strings.Trim(b.String(), "_")
	if text == "" {
		return "unnamed"
	}
	return text
}

func javaContextPosKey(raw antlr.ParserRuleContext) string {
	if raw == nil || raw.GetStart() == nil {
		return "0:0"
	}
	return fmt.Sprintf("%d:%d", raw.GetStart().GetLine(), raw.GetStart().GetColumn())
}

func (y *singleFileBuilder) collectJavaFormalParameterSignatures(raw javaparser.IFormalParametersContext) ([]javaParamSignature, bool) {
	params := make([]javaParamSignature, 0)
	if y == nil || raw == nil {
		return params, false
	}
	i, _ := raw.(*javaparser.FormalParametersContext)
	if i == nil {
		return params, false
	}

	if i.FormalParameterList() == nil {
		return params, false
	}
	list, _ := i.FormalParameterList().(*javaparser.FormalParameterListContext)
	if list == nil {
		return params, false
	}

	for _, formal := range list.AllFormalParameter() {
		params = append(params, y.collectJavaFormalParameterSignature(formal))
	}
	variadic := false
	if last := list.LastFormalParameter(); last != nil {
		lastParam, isVariadic := y.collectJavaLastFormalParameterSignature(last)
		params = append(params, lastParam)
		variadic = isVariadic
	}
	return params, variadic
}

func (y *singleFileBuilder) collectJavaFormalParameterSignature(raw javaparser.IFormalParameterContext) javaParamSignature {
	sign := javaParamSignature{
		key: "any",
		typ: ssa.CreateAnyType(),
	}
	if y == nil || raw == nil {
		return sign
	}
	i, _ := raw.(*javaparser.FormalParameterContext)
	if i == nil {
		return sign
	}

	var (
		typCtx       antlr.ParserRuleContext
		sourceType   ssa.Type
		sourceTypeTx string
	)
	if typeType := i.TypeType(); typeType != nil {
		sourceType = y.VisitTypeType(typeType)
		sourceTypeTx = typeType.GetText()
		if p, ok := typeType.(antlr.ParserRuleContext); ok {
			typCtx = p
		}
	}
	bracketLevel := 0
	if id := i.VariableDeclaratorId(); id != nil {
		if p, ok := id.(*javaparser.VariableDeclaratorIdContext); ok {
			bracketLevel = len(p.AllLBRACK())
			if typCtx == nil {
				typCtx = p
			}
		}
	}
	sourceType = TypeAddBracketLevel(sourceType, bracketLevel)
	sign.typ = sourceType
	sign.key = y.buildJavaTypeSignatureKey(sourceTypeTx, sourceType, typCtx)
	return sign
}

func (y *singleFileBuilder) collectJavaLastFormalParameterSignature(raw javaparser.ILastFormalParameterContext) (javaParamSignature, bool) {
	sign := javaParamSignature{
		key: "any",
		typ: ssa.CreateAnyType(),
	}
	if y == nil || raw == nil {
		return sign, false
	}
	i, _ := raw.(*javaparser.LastFormalParameterContext)
	if i == nil {
		return sign, false
	}

	var (
		typCtx       antlr.ParserRuleContext
		sourceType   ssa.Type
		sourceTypeTx string
	)
	if typeType := i.TypeType(); typeType != nil {
		sourceType = y.VisitTypeType(typeType)
		sourceTypeTx = typeType.GetText()
		if p, ok := typeType.(antlr.ParserRuleContext); ok {
			typCtx = p
		}
	}
	bracketLevel := 0
	if id := i.VariableDeclaratorId(); id != nil {
		if p, ok := id.(*javaparser.VariableDeclaratorIdContext); ok {
			bracketLevel = len(p.AllLBRACK())
			if typCtx == nil {
				typCtx = p
			}
		}
	}
	sourceType = TypeAddBracketLevel(sourceType, bracketLevel)
	sign.typ = sourceType
	sign.key = y.buildJavaTypeSignatureKey(sourceTypeTx, sourceType, typCtx)
	return sign, i.ELLIPSIS() != nil
}

func (y *singleFileBuilder) buildJavaTypeSignatureKey(sourceTypeText string, typ ssa.Type, ctx antlr.ParserRuleContext) string {
	if typ != nil {
		fullNames := append([]string(nil), typ.GetFullTypeNames()...)
		if len(fullNames) > 0 {
			sort.Strings(fullNames)
			return fullNames[0]
		}
		if kind := typ.GetTypeKind(); kind != ssa.AnyTypeKind {
			if name := strings.TrimSpace(typ.String()); name != "" {
				return name
			}
		}
	}
	sourceTypeText = strings.TrimSpace(sourceTypeText)
	if sourceTypeText != "" {
		return sourceTypeText
	}
	return "unknown@" + javaContextPosKey(ctx)
}

func (y *singleFileBuilder) registerJavaMethodOverload(
	class *ssa.Blueprint,
	methodName string,
	function *ssa.Function,
	params []javaParamSignature,
	variadic bool,
) {
	if y == nil || class == nil || function == nil {
		return
	}
	if y.methodOverloads[class] == nil {
		y.methodOverloads[class] = make(map[string][]*javaCallableCandidate)
	}
	candidate := &javaCallableCandidate{
		function:     function,
		paramTypeKey: make([]string, 0, len(params)),
		paramType:    make([]ssa.Type, 0, len(params)),
		variadic:     variadic,
		order:        y.callableDeclOrder,
	}
	y.callableDeclOrder++
	for _, param := range params {
		candidate.paramTypeKey = append(candidate.paramTypeKey, param.key)
		candidate.paramType = append(candidate.paramType, param.typ)
	}
	y.methodOverloads[class][methodName] = append(y.methodOverloads[class][methodName], candidate)
}

func (y *singleFileBuilder) registerJavaConstructorOverload(
	class *ssa.Blueprint,
	function *ssa.Function,
	params []javaParamSignature,
	variadic bool,
) {
	if y == nil || class == nil || function == nil {
		return
	}
	candidate := &javaCallableCandidate{
		function:     function,
		paramTypeKey: make([]string, 0, len(params)),
		paramType:    make([]ssa.Type, 0, len(params)),
		variadic:     variadic,
		order:        y.callableDeclOrder,
	}
	y.callableDeclOrder++
	for _, param := range params {
		candidate.paramTypeKey = append(candidate.paramTypeKey, param.key)
		candidate.paramType = append(candidate.paramType, param.typ)
	}
	y.constructorOverloads[class] = append(y.constructorOverloads[class], candidate)
}

func (y *singleFileBuilder) resolveJavaMethodOverload(class *ssa.Blueprint, methodName string, args []ssa.Value) *ssa.Function {
	if y == nil || class == nil || methodName == "" {
		return nil
	}

	candidates := make([]*javaCallableCandidate, 0)
	visited := make(map[*ssa.Blueprint]struct{})
	queue := []*ssa.Blueprint{class}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == nil {
			continue
		}
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}
		if methods := y.methodOverloads[current]; methods != nil {
			candidates = append(candidates, methods[methodName]...)
		}
		queue = append(queue, current.GetParentBlueprint()...)
	}

	best, ambiguous := y.selectBestJavaCallable(candidates, args)
	if ambiguous {
		log.Warnf("java overload ambiguous: %s.%s with %d args", class.Name, methodName, len(args))
	}
	if best == nil {
		return nil
	}
	best.function.Build()
	return best.function
}

func (y *singleFileBuilder) resolveJavaConstructorOverload(class *ssa.Blueprint, args []ssa.Value) *ssa.Function {
	if y == nil || class == nil {
		return nil
	}
	best, ambiguous := y.selectBestJavaCallable(y.constructorOverloads[class], args)
	if ambiguous {
		log.Warnf("java constructor overload ambiguous: %s with %d args", class.Name, len(args))
	}
	if best == nil {
		return nil
	}
	best.function.Build()
	return best.function
}

func (y *singleFileBuilder) selectBestJavaCallable(
	candidates []*javaCallableCandidate,
	args []ssa.Value,
) (*javaCallableCandidate, bool) {
	var (
		best          *javaCallableCandidate
		bestScore     = -1
		hasAmbiguous  = false
		bestArityDiff = -1
	)
	for _, candidate := range candidates {
		score, arityDiff, ok := y.scoreJavaCallableCandidate(candidate, args)
		if !ok {
			continue
		}
		if best == nil ||
			score > bestScore ||
			(score == bestScore && (bestArityDiff == -1 || arityDiff < bestArityDiff)) ||
			(score == bestScore && arityDiff == bestArityDiff && candidate.order < best.order) {
			best = candidate
			bestScore = score
			bestArityDiff = arityDiff
			hasAmbiguous = false
			continue
		}
		if score == bestScore && arityDiff == bestArityDiff {
			hasAmbiguous = true
		}
	}
	return best, hasAmbiguous
}

func (y *singleFileBuilder) scoreJavaCallableCandidate(candidate *javaCallableCandidate, args []ssa.Value) (int, int, bool) {
	if candidate == nil {
		return 0, 0, false
	}
	paramCount := len(candidate.paramTypeKey)
	argCount := len(args)

	if candidate.variadic {
		if argCount < paramCount-1 {
			return 0, 0, false
		}
	} else if argCount != paramCount {
		return 0, 0, false
	}

	score := 0
	arityDiff := 0
	if candidate.variadic && argCount > paramCount {
		arityDiff = argCount - paramCount
	}

	for idx := 0; idx < argCount; idx++ {
		paramIndex := idx
		if candidate.variadic && paramCount > 0 && paramIndex >= paramCount-1 {
			paramIndex = paramCount - 1
		}

		arg := args[idx]
		argType := ssa.Type(nil)
		if !utils.IsNil(arg) {
			argType = arg.GetType()
		}

		partScore, ok := y.matchJavaTypeForOverload(argType, candidate.paramType[paramIndex], candidate.paramTypeKey[paramIndex])
		if !ok {
			return 0, 0, false
		}
		score += partScore
	}
	if candidate.variadic {
		score--
	}
	return score, arityDiff, true
}

func (y *singleFileBuilder) matchJavaTypeForOverload(argType ssa.Type, paramType ssa.Type, paramKey string) (int, bool) {
	if argType == nil || argType.GetTypeKind() == ssa.AnyTypeKind {
		return 1, true
	}
	if paramType == nil || paramType.GetTypeKind() == ssa.AnyTypeKind {
		return 1, true
	}
	if argType.GetTypeKind() == ssa.NullTypeKind {
		switch paramType.GetTypeKind() {
		case ssa.BooleanTypeKind, ssa.NumberTypeKind:
			return 0, false
		default:
			return 2, true
		}
	}
	if ssa.TypeEqual(argType, paramType) {
		return 8, true
	}
	if ssa.TypeCompare(argType, paramType) || ssa.TypeCompare(paramType, argType) {
		return 6, true
	}

	argKey := y.buildJavaTypeSignatureKey("", argType, nil)
	if argKey == paramKey {
		return 5, true
	}
	if javaSimpleTypeName(argKey) == javaSimpleTypeName(paramKey) {
		return 3, true
	}
	return 0, false
}

func javaSimpleTypeName(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 && idx < len(name)-1 {
		return name[idx+1:]
	}
	return name
}

func (y *singleFileBuilder) callJavaConstructorWithOverload(
	class *ssa.Blueprint,
	args []ssa.Value,
	withDeferDestructor bool,
) ssa.Value {
	if y == nil || class == nil {
		return nil
	}
	if len(args) == 0 {
		return y.ClassConstructor(class, args)
	}
	constructor := y.resolveJavaConstructorOverload(class, args[1:])
	if constructor == nil {
		if withDeferDestructor {
			return y.ClassConstructor(class, args)
		}
		method := class.GetMagicMethod(ssa.Constructor, y.FunctionBuilder)
		call := y.NewCall(method, args)
		y.EmitCall(call)
		return call
	}
	call := y.NewCall(constructor, args)
	y.EmitCall(call)
	call.SetType(class)
	if withDeferDestructor {
		destructor := class.GetMagicMethod(ssa.Destructor, y.FunctionBuilder)
		deferCall := y.NewCall(destructor, []ssa.Value{call})
		y.EmitDefer(deferCall)
	}
	return call
}

func (y *singleFileBuilder) extractJavaBlueprintFromValue(value ssa.Value) *ssa.Blueprint {
	if y == nil || utils.IsNil(value) {
		return nil
	}
	if class, ok := ssa.ToClassBluePrintType(value.GetType()); ok {
		return class
	}
	if un, ok := ssa.ToUndefined(value); ok {
		return y.GetBluePrint(un.GetName())
	}
	return nil
}
