package csharp2ssa

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// csharpConstructorRegistry keeps overload metadata outside the generic SSA
// Blueprint API. Blueprint.Constructor intentionally exposes only one magic
// method, while C# needs the complete overload set for new/base/this calls.
type csharpConstructorRegistry struct {
	mu         sync.RWMutex
	entries    map[*ssa.Blueprint][]csharpConstructor
	declared   map[*ssa.Blueprint]struct{}
	methods    map[csharpMethodKey][]csharpMethod
	methodKeys map[*ssa.Function]csharpMethodKey
	overrides  map[csharpMethodEdge]struct{}
}

type csharpConstructor struct {
	function   *ssa.Function
	parameters []csharpConstructorParameter
	required   int
	variadic   bool
}

type csharpConstructorParameter struct {
	name     string
	typeName string
	modifier string
	optional bool
	params   bool
}

// csharpArgumentBinding maps each formal parameter to indexes in the already
// evaluated source argument list. Only a params formal may own multiple source
// indexes. Evaluation order therefore stays independent from Call.Args order.
type csharpArgumentBinding struct {
	formal         [][]int
	expandedParams bool
}

type csharpConstructorSelection struct {
	constructor csharpConstructor
	binding     csharpArgumentBinding
}

type csharpMethodKey struct {
	class  *ssa.Blueprint
	name   string
	static bool
}

type csharpMethod struct {
	function   *ssa.Function
	parameters []csharpConstructorParameter
	key        csharpMethodKey
	override   bool
	hides      bool
	partial    bool
	hasBody    bool
}

type csharpMethodEdge struct {
	base    *ssa.Function
	derived *ssa.Function
}

type csharpMethodSelection struct {
	method    csharpMethod
	binding   csharpArgumentBinding
	ambiguous bool
	best      []csharpMethodAlternative
}

type csharpMethodAlternative struct {
	method  csharpMethod
	binding csharpArgumentBinding
}

func (r *csharpConstructorRegistry) reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.entries = make(map[*ssa.Blueprint][]csharpConstructor)
	r.declared = make(map[*ssa.Blueprint]struct{})
	r.methods = make(map[csharpMethodKey][]csharpMethod)
	r.methodKeys = make(map[*ssa.Function]csharpMethodKey)
	r.overrides = make(map[csharpMethodEdge]struct{})
	r.mu.Unlock()
}

// markDeclared records the exact blueprint created for a C# source type.  A
// name/export lookup is insufficient here because unresolved external types
// are also represented by exported blueprints, and may share a short name
// with a source type in another namespace.
func (r *csharpConstructorRegistry) markDeclared(class *ssa.Blueprint) {
	if r == nil || class == nil {
		return
	}
	r.mu.Lock()
	if r.declared == nil {
		r.declared = make(map[*ssa.Blueprint]struct{})
	}
	r.declared[class] = struct{}{}
	r.mu.Unlock()
}

func (r *csharpConstructorRegistry) isDeclared(class *ssa.Blueprint) bool {
	if r == nil || class == nil {
		return false
	}
	r.mu.RLock()
	_, ok := r.declared[class]
	r.mu.RUnlock()
	return ok
}

func (r *csharpConstructorRegistry) register(class *ssa.Blueprint, function *ssa.Function, raw csharpparser.IParameter_listContext) {
	if r == nil || class == nil || function == nil {
		return
	}
	entry := csharpConstructor{function: function}
	list, _ := raw.(*csharpparser.Parameter_listContext)
	if list != nil {
		if fixed, _ := list.Fixed_parameters().(*csharpparser.Fixed_parametersContext); fixed != nil {
			for _, rawParameter := range fixed.AllFixed_parameter() {
				parameter, _ := rawParameter.(*csharpparser.Fixed_parameterContext)
				if parameter == nil {
					continue
				}
				item := csharpConstructorParameter{
					name:     identText(parameter.Identifier()),
					modifier: csharpFixedParameterModifier(parameter),
					optional: parameter.Default_argument() != nil,
				}
				if parameter.Type_() != nil {
					item.typeName = parameter.Type_().GetText()
				}
				entry.parameters = append(entry.parameters, item)
				if !item.optional {
					entry.required++
				}
			}
		}
		if parameterArray, _ := list.Parameter_array().(*csharpparser.Parameter_arrayContext); parameterArray != nil {
			item := csharpConstructorParameter{
				name:   identText(parameterArray.Identifier()),
				params: true,
			}
			if parameterArray.Array_type() != nil {
				item.typeName = parameterArray.Array_type().GetText()
			}
			entry.parameters = append(entry.parameters, item)
			entry.variadic = true
		}
	}
	r.mu.Lock()
	if r.entries == nil {
		r.entries = make(map[*ssa.Blueprint][]csharpConstructor)
	}
	r.entries[class] = append(r.entries[class], entry)
	r.mu.Unlock()
}

func csharpParameterMetadata(raw csharpparser.IParameter_listContext) []csharpConstructorParameter {
	list, _ := raw.(*csharpparser.Parameter_listContext)
	if list == nil {
		return nil
	}
	var parameters []csharpConstructorParameter
	if fixed, _ := list.Fixed_parameters().(*csharpparser.Fixed_parametersContext); fixed != nil {
		for _, rawParameter := range fixed.AllFixed_parameter() {
			parameter, _ := rawParameter.(*csharpparser.Fixed_parameterContext)
			if parameter == nil {
				continue
			}
			item := csharpConstructorParameter{
				name:     identText(parameter.Identifier()),
				modifier: csharpFixedParameterModifier(parameter),
				optional: parameter.Default_argument() != nil,
			}
			if parameter.Type_() != nil {
				item.typeName = parameter.Type_().GetText()
			}
			parameters = append(parameters, item)
		}
	}
	if parameterArray, _ := list.Parameter_array().(*csharpparser.Parameter_arrayContext); parameterArray != nil {
		item := csharpConstructorParameter{
			name:   identText(parameterArray.Identifier()),
			params: true,
		}
		if parameterArray.Array_type() != nil {
			item.typeName = parameterArray.Array_type().GetText()
		}
		parameters = append(parameters, item)
	}
	return parameters
}

func csharpFixedParameterModifier(parameter *csharpparser.Fixed_parameterContext) string {
	if parameter == nil {
		return ""
	}
	modifier, _ := parameter.Parameter_modifier().(*csharpparser.Parameter_modifierContext)
	if modifier == nil {
		return ""
	}
	mode, _ := modifier.Parameter_mode_modifier().(*csharpparser.Parameter_mode_modifierContext)
	if mode == nil {
		return ""
	}
	switch {
	case mode.KW_REF() != nil:
		return "ref"
	case mode.KW_OUT() != nil:
		return "out"
	case mode.KW_IN() != nil:
		return "in"
	default:
		return ""
	}
}

// registerMethod records one source-declared method overload. It returns true
// for the first overload in this exact C# method group. Only that first method
// is installed in Blueprint's single method slot; later overloads stay in this
// registry so Blueprint's virtual-dispatch pointer chain is not polluted by
// same-class overload relationships.
func (r *csharpConstructorRegistry) registerMethod(class *ssa.Blueprint, name string, static bool, function *ssa.Function, raw csharpparser.IParameter_listContext, override, hides bool, partialBody ...bool) bool {
	return r.registerMethodMetadata(class, name, static, function, csharpParameterMetadata(raw), override, hides, partialBody...)
}

func (r *csharpConstructorRegistry) registerMethodMetadata(class *ssa.Blueprint, name string, static bool, function *ssa.Function, parameters []csharpConstructorParameter, override, hides bool, partialBody ...bool) bool {
	if r == nil || class == nil || name == "" || function == nil {
		return false
	}
	key := csharpMethodKey{class: class, name: name, static: static}
	method := csharpMethod{
		function:   function,
		parameters: append([]csharpConstructorParameter(nil), parameters...),
		key:        key,
		override:   override,
		hides:      hides,
	}
	if len(partialBody) != 0 {
		method.partial = partialBody[0]
	}
	if len(partialBody) > 1 {
		method.hasBody = partialBody[1]
	}
	r.mu.Lock()
	if r.methods == nil {
		r.methods = make(map[csharpMethodKey][]csharpMethod)
	}
	if r.methodKeys == nil {
		r.methodKeys = make(map[*ssa.Function]csharpMethodKey)
	}
	first := len(r.methods[key]) == 0
	if method.partial {
		signature := csharpMethodSignature(method)
		for index, existing := range r.methods[key] {
			if !existing.partial || csharpMethodSignature(existing) != signature {
				continue
			}
			// A partial definition and implementation are one callable, not an
			// overload pair. Prefer the body-bearing declaration regardless of
			// source/compilation-unit order, while keeping both shell functions
			// mapped to the same group for calls built from the first slot.
			if method.hasBody && !existing.hasBody {
				r.methods[key][index] = method
			}
			r.methodKeys[function] = key
			r.mu.Unlock()
			return false
		}
	}
	r.methods[key] = append(r.methods[key], method)
	r.methodKeys[function] = key
	r.mu.Unlock()
	return first
}

func (r *csharpConstructorRegistry) hasMethod(class *ssa.Blueprint, name string, static bool) bool {
	if r == nil || class == nil || name == "" {
		return false
	}
	r.mu.RLock()
	_, ok := r.methods[csharpMethodKey{class: class, name: name, static: static}]
	r.mu.RUnlock()
	return ok
}

func (r *csharpConstructorRegistry) selectMethod(function *ssa.Function, args []csharpEvaluatedArgument) *csharpMethodSelection {
	if r == nil || function == nil {
		return nil
	}
	r.mu.RLock()
	key, ok := r.methodKeys[function]
	if !ok {
		r.mu.RUnlock()
		return nil
	}
	candidates := append([]csharpMethod(nil), r.methods[key]...)
	r.mu.RUnlock()
	selection := selectCSharpMethodCandidate(candidates, args)
	if selection != nil {
		for _, candidate := range selection.best {
			r.linkVirtualOverrides(candidate.method)
		}
	}
	return selection
}

// selectBareMethod handles an unqualified invocation inside a type. C# lets a
// bare call in an instance context choose between static and instance overloads
// with the same name. Search the nearest declaring type in the hierarchy, then
// rank that combined candidate set using the already-evaluated arguments.
func (r *csharpConstructorRegistry) selectBareMethod(class *ssa.Blueprint, name string, allowInstance bool, args []csharpEvaluatedArgument) *csharpMethodSelection {
	if r == nil || class == nil || name == "" {
		return nil
	}
	seen := make(map[*ssa.Blueprint]struct{})
	for current := class; current != nil; current = current.GetSuperBlueprint() {
		if _, exists := seen[current]; exists {
			break
		}
		seen[current] = struct{}{}
		current.Build()
		r.mu.RLock()
		candidates := append([]csharpMethod(nil), r.methods[csharpMethodKey{class: current, name: name, static: true}]...)
		if allowInstance {
			candidates = append(candidates, r.methods[csharpMethodKey{class: current, name: name, static: false}]...)
		}
		r.mu.RUnlock()
		if len(candidates) != 0 {
			selection := selectCSharpMethodCandidate(candidates, args)
			if selection != nil {
				for _, candidate := range selection.best {
					r.linkVirtualOverrides(candidate.method)
				}
			}
			return selection
		}
	}
	return nil
}

func selectCSharpMethodCandidate(candidates []csharpMethod, args []csharpEvaluatedArgument) *csharpMethodSelection {
	bestScore := -1 << 30
	var best *csharpMethodSelection
	for _, candidate := range candidates {
		if candidate.function == nil {
			continue
		}
		binding, ok := bindCSharpArguments(candidate.parameters, args)
		if !ok {
			continue
		}
		score := matchCSharpArguments(candidate.parameters, args, binding)
		if score > bestScore {
			bestScore = score
			alternative := csharpMethodAlternative{method: candidate, binding: binding}
			best = &csharpMethodSelection{method: candidate, binding: binding, best: []csharpMethodAlternative{alternative}}
		} else if score == bestScore && best != nil {
			best.ambiguous = true
			best.best = append(best.best, csharpMethodAlternative{method: candidate, binding: binding})
		}
	}
	return best
}

func csharpMethodSignature(method csharpMethod) string {
	parts := make([]string, 0, len(method.parameters))
	for _, parameter := range method.parameters {
		typeName := canonicalCSharpSignatureType(parameter.typeName)
		parts = append(parts, parameter.modifier+":"+typeName)
	}
	return strings.Join(parts, ",")
}

var csharpSignatureTypeAliases = map[string]string{
	"bool": "System.Boolean", "Boolean": "System.Boolean", "System.Boolean": "System.Boolean",
	"byte": "System.Byte", "Byte": "System.Byte", "System.Byte": "System.Byte",
	"sbyte": "System.SByte", "SByte": "System.SByte", "System.SByte": "System.SByte",
	"char": "System.Char", "Char": "System.Char", "System.Char": "System.Char",
	"decimal": "System.Decimal", "Decimal": "System.Decimal", "System.Decimal": "System.Decimal",
	"double": "System.Double", "Double": "System.Double", "System.Double": "System.Double",
	"float": "System.Single", "Single": "System.Single", "System.Single": "System.Single",
	"int": "System.Int32", "Int32": "System.Int32", "System.Int32": "System.Int32",
	"uint": "System.UInt32", "UInt32": "System.UInt32", "System.UInt32": "System.UInt32",
	"long": "System.Int64", "Int64": "System.Int64", "System.Int64": "System.Int64",
	"ulong": "System.UInt64", "UInt64": "System.UInt64", "System.UInt64": "System.UInt64",
	"short": "System.Int16", "Int16": "System.Int16", "System.Int16": "System.Int16",
	"ushort": "System.UInt16", "UInt16": "System.UInt16", "System.UInt16": "System.UInt16",
	"nint": "System.IntPtr", "IntPtr": "System.IntPtr", "System.IntPtr": "System.IntPtr",
	"nuint": "System.UIntPtr", "UIntPtr": "System.UIntPtr", "System.UIntPtr": "System.UIntPtr",
	"object": "System.Object", "Object": "System.Object", "System.Object": "System.Object",
	"dynamic": "System.Object",
	"string":  "System.String", "String": "System.String", "System.String": "System.String",
	"void": "System.Void", "Void": "System.Void", "System.Void": "System.Void",
}

// canonicalCSharpSignatureType normalizes aliases without discarding shape.
// Array ranks, nullable markers, generic punctuation, pointer markers, and
// parameter modifiers therefore remain part of the overload/override key.
func canonicalCSharpSignatureType(raw string) string {
	raw = strings.ReplaceAll(raw, "global::", "")
	for _, whitespace := range []string{" ", "\t", "\r", "\n"} {
		raw = strings.ReplaceAll(raw, whitespace, "")
	}
	var canonical strings.Builder
	for index := 0; index < len(raw); {
		if !isCSharpSignatureIdentifierByte(raw[index]) {
			canonical.WriteByte(raw[index])
			index++
			continue
		}
		end := index + 1
		for end < len(raw) && isCSharpSignatureIdentifierByte(raw[end]) {
			end++
		}
		token := raw[index:end]
		if alias, ok := csharpSignatureTypeAliases[token]; ok {
			canonical.WriteString(alias)
		} else {
			canonical.WriteString(token)
		}
		index = end
	}
	return canonical.String()
}

func isCSharpSignatureIdentifierByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '_' || value == '@' || value == '.'
}

func csharpBlueprintDerivesFrom(class, base *ssa.Blueprint) bool {
	if class == nil || base == nil || class == base {
		return false
	}
	seen := make(map[*ssa.Blueprint]struct{})
	queue := append([]*ssa.Blueprint(nil), class.GetParentBlueprint()...)
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if current == nil {
			continue
		}
		if current == base {
			return true
		}
		if _, exists := seen[current]; exists {
			continue
		}
		seen[current] = struct{}{}
		current.Build()
		queue = append(queue, current.GetParentBlueprint()...)
	}
	return false
}

// csharpBlueprintDeclaresInterface reports whether class's own base list names
// target (possibly through another interface). It intentionally does not walk
// the class's base class: a derived class that merely inherits an interface
// also inherits its existing interface map. Only listing the interface again
// starts a new lookup at the derived class and lets a `new` method replace the
// inherited implementation.
func csharpBlueprintDeclaresInterface(class, target *ssa.Blueprint) bool {
	if class == nil || target == nil {
		return false
	}
	seen := make(map[*ssa.Blueprint]struct{})
	queue := append([]*ssa.Blueprint(nil), class.GetInterfaceBlueprint()...)
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if current == nil {
			continue
		}
		if current == target {
			return true
		}
		if _, exists := seen[current]; exists {
			continue
		}
		seen[current] = struct{}{}
		current.Build()
		queue = append(queue, current.GetInterfaceBlueprint()...)
	}
	return false
}

func csharpInterfaceMappingClass(concrete, target *ssa.Blueprint) *ssa.Blueprint {
	seen := make(map[*ssa.Blueprint]struct{})
	for current := concrete; current != nil; current = current.GetSuperBlueprint() {
		if _, exists := seen[current]; exists {
			return nil
		}
		seen[current] = struct{}{}
		current.Build()
		if csharpBlueprintDeclaresInterface(current, target) {
			return current
		}
	}
	return nil
}

func csharpMethodWithSignature(methods map[*ssa.Blueprint][]csharpMethod, class *ssa.Blueprint, signature string, requireOverride bool) (csharpMethod, bool) {
	for _, method := range methods[class] {
		if method.function == nil || csharpMethodSignature(method) != signature {
			continue
		}
		if requireOverride && (!method.override || method.hides) {
			continue
		}
		return method, true
	}
	return csharpMethod{}, false
}

// csharpInterfaceImplementation resolves the effective implementation for one
// concrete runtime type. Interface mapping starts at the nearest class that
// explicitly lists the interface. A method declared there (including `new`)
// wins over an inherited method. Below that mapping class, only an actual class
// override may change dispatch; an unrelated hiding method must not silently
// remap an inherited interface.
func csharpInterfaceImplementation(concrete, target *ssa.Blueprint, signature string, methods map[*ssa.Blueprint][]csharpMethod) (csharpMethod, bool) {
	mappingClass := csharpInterfaceMappingClass(concrete, target)
	if mappingClass == nil {
		return csharpMethod{}, false
	}

	seen := make(map[*ssa.Blueprint]struct{})
	var mapped csharpMethod
	var owner *ssa.Blueprint
	for current := mappingClass; current != nil; current = current.GetSuperBlueprint() {
		if _, exists := seen[current]; exists {
			return csharpMethod{}, false
		}
		seen[current] = struct{}{}
		current.Build()
		if method, ok := csharpMethodWithSignature(methods, current, signature, false); ok {
			mapped, owner = method, current
			break
		}
	}
	if owner == nil {
		return csharpMethod{}, false
	}

	// The concrete type may be below the class that established the interface
	// map. Preserve ordinary virtual dispatch by selecting its nearest explicit
	// override, while ignoring new/implicit hiding declarations.
	seen = make(map[*ssa.Blueprint]struct{})
	for current := concrete; current != nil && current != owner; current = current.GetSuperBlueprint() {
		if _, exists := seen[current]; exists {
			break
		}
		seen[current] = struct{}{}
		current.Build()
		if method, ok := csharpMethodWithSignature(methods, current, signature, true); ok {
			return method, true
		}
	}
	return mapped, true
}

// linkVirtualOverrides adds only cross-class, signature-equal override edges.
// Same-class overloads never enter this graph. Function top-def traversal only
// examines direct pointer children, so each selected ancestor points at every
// matching descendant override; Point also records the semantic base reference
// on the overriding function.
func (r *csharpConstructorRegistry) linkVirtualOverrides(base csharpMethod) {
	if r == nil || base.function == nil || base.key.class == nil || base.key.static {
		return
	}
	base.key.class.Build()
	r.mu.RLock()
	var candidates []csharpMethod
	methodsByClass := make(map[*ssa.Blueprint][]csharpMethod)
	for key, group := range r.methods {
		if key.static || key.name != base.key.name {
			continue
		}
		copied := append([]csharpMethod(nil), group...)
		methodsByClass[key.class] = copied
		if key.class != base.key.class {
			candidates = append(candidates, copied...)
		}
	}
	declared := make([]*ssa.Blueprint, 0, len(r.declared))
	for class := range r.declared {
		declared = append(declared, class)
	}
	r.mu.RUnlock()
	baseSignature := csharpMethodSignature(base)
	if base.key.class.IsInterface() {
		for _, concrete := range declared {
			if concrete == nil || concrete.IsInterface() {
				continue
			}
			concrete.Build()
			implementation, ok := csharpInterfaceImplementation(concrete, base.key.class, baseSignature, methodsByClass)
			if !ok {
				continue
			}
			r.linkMethodEdge(base.function, implementation.function)
		}
		return
	}
	for _, derived := range candidates {
		if derived.hides || derived.function == nil || csharpMethodSignature(derived) != baseSignature {
			continue
		}
		derived.key.class.Build()
		if !derived.override || !csharpBlueprintDerivesFrom(derived.key.class, base.key.class) {
			continue
		}
		r.linkMethodEdge(base.function, derived.function)
	}
}

func (r *csharpConstructorRegistry) linkMethodEdge(base, derived *ssa.Function) {
	if r == nil || base == nil || derived == nil || base == derived {
		return
	}
	edge := csharpMethodEdge{base: base, derived: derived}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.overrides == nil {
		r.overrides = make(map[csharpMethodEdge]struct{})
	}
	if _, exists := r.overrides[edge]; exists {
		return
	}
	r.overrides[edge] = struct{}{}
	ssa.Point(derived, base)
}

func (r *csharpConstructorRegistry) candidates(class *ssa.Blueprint) []csharpConstructor {
	if r == nil || class == nil {
		return nil
	}
	r.mu.RLock()
	ret := append([]csharpConstructor(nil), r.entries[class]...)
	r.mu.RUnlock()
	return ret
}

func (r *csharpConstructorRegistry) selectConstructors(class *ssa.Blueprint, args []csharpEvaluatedArgument, exclude *ssa.Function) []csharpConstructorSelection {
	candidates := r.candidates(class)
	if len(candidates) == 0 {
		return nil
	}
	bestScore := -1 << 30
	var best []csharpConstructorSelection
	for _, candidate := range candidates {
		if candidate.function == nil || candidate.function == exclude {
			continue
		}
		binding, ok := bindCSharpArguments(candidate.parameters, args)
		if !ok {
			continue
		}
		score := candidate.matchScore(args, binding)
		if score > bestScore {
			bestScore = score
			best = []csharpConstructorSelection{{constructor: candidate, binding: binding}}
		} else if score == bestScore {
			best = append(best, csharpConstructorSelection{constructor: candidate, binding: binding})
		}
	}
	return best
}

func bindCSharpArguments(parameters []csharpConstructorParameter, args []csharpEvaluatedArgument) (csharpArgumentBinding, bool) {
	binding := csharpArgumentBinding{formal: make([][]int, len(parameters))}
	names := make(map[string]int, len(parameters))
	paramsIndex := -1
	for index, parameter := range parameters {
		if parameter.name != "" {
			names[parameter.name] = index
		}
		if parameter.params {
			paramsIndex = index
		}
	}

	positional := 0
	paramsNamed := false
	for sourceIndex, argument := range args {
		if argument.name != "" {
			formalIndex, ok := names[argument.name]
			if !ok || len(binding.formal[formalIndex]) != 0 {
				return csharpArgumentBinding{}, false
			}
			binding.formal[formalIndex] = append(binding.formal[formalIndex], sourceIndex)
			if formalIndex == paramsIndex {
				paramsNamed = true
			}
			continue
		}

		for positional < len(parameters) && !parameters[positional].params && len(binding.formal[positional]) != 0 {
			positional++
		}
		if positional >= len(parameters) || (positional == paramsIndex && paramsNamed) {
			return csharpArgumentBinding{}, false
		}
		binding.formal[positional] = append(binding.formal[positional], sourceIndex)
		if positional != paramsIndex {
			positional++
		}
	}

	for index, parameter := range parameters {
		if !parameter.optional && !parameter.params && len(binding.formal[index]) == 0 {
			return csharpArgumentBinding{}, false
		}
		for _, sourceIndex := range binding.formal[index] {
			if !csharpArgumentModifierCompatible(parameter.modifier, args[sourceIndex].modifier) {
				return csharpArgumentBinding{}, false
			}
		}
	}
	if paramsIndex >= 0 {
		bound := binding.formal[paramsIndex]
		switch {
		case len(bound) == 0:
			binding.expandedParams = true
		case paramsNamed || len(bound) > 1:
			binding.expandedParams = !paramsNamed
		default:
			argument := args[bound[0]].value
			binding.expandedParams = utils.IsNil(argument) || argument.GetType() == nil || argument.GetType().GetTypeKind() != ssa.SliceTypeKind
		}
	}
	return binding, true
}

func csharpArgumentModifierCompatible(parameter, argument string) bool {
	switch parameter {
	case "ref", "out":
		return argument == parameter
	case "byref":
		// Function-type metadata loaded from SSA/DB preserves pointer side
		// effects but not the source spelling of ref versus out. Either explicit
		// by-reference argument is compatible; a plain value is not.
		return argument == "ref" || argument == "out"
	case "in":
		return argument == "" || argument == "in"
	default:
		return argument == ""
	}
}

func (c csharpConstructor) matchScore(args []csharpEvaluatedArgument, binding csharpArgumentBinding) int {
	return matchCSharpArguments(c.parameters, args, binding)
}

func matchCSharpArguments(parameters []csharpConstructorParameter, args []csharpEvaluatedArgument, binding csharpArgumentBinding) int {
	score := 0
	exact := len(args) == len(parameters) && !binding.expandedParams
	if exact {
		for _, indexes := range binding.formal {
			if len(indexes) != 1 {
				exact = false
				break
			}
		}
	}
	if exact {
		// Prefer an exact-arity overload to one reached through defaults/params.
		score += 32
	}
	for parameterIndex, indexes := range binding.formal {
		parameter := parameters[parameterIndex]
		typeName := parameter.typeName
		if parameter.params && binding.expandedParams {
			typeName = strings.TrimSuffix(typeName, "[]")
		}
		for _, sourceIndex := range indexes {
			score += csharpConstructorTypeScore(typeName, args[sourceIndex].value)
		}
	}
	if binding.expandedParams {
		// Prefer an otherwise equivalent fixed-arity overload over expanded params.
		score -= 8
	}
	return score
}

func csharpConstructorTypeScore(raw string, arg ssa.Value) int {
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "global::"))
	nullable := strings.HasSuffix(raw, "?")
	raw = strings.TrimSuffix(raw, "?")
	if raw == "" || arg == nil || arg.GetType() == nil {
		return 0
	}
	short := lastDotSegment(stripGenericSuffix(raw))
	typ := arg.GetType()
	if typ.GetTypeKind() == ssa.NullTypeKind {
		switch {
		case short == "object" || short == "Object" || short == "dynamic":
			return 1
		case nullable, short == "string", short == "String", strings.HasSuffix(raw, "[]"):
			return 10
		case isCSharpNonNullableValueType(short):
			return -8
		default:
			// A named class/interface reference is more specific than object. If
			// several unrelated reference types remain, registration order is a
			// stable approximation of C#'s compile-time ambiguity.
			return 8
		}
	}
	if typ.GetTypeKind() == ssa.AnyTypeKind {
		// Unknown external return types provide no evidence for overload
		// ranking. In particular, do not let object win merely because it is a
		// universal conversion; an equal best score triggers a sound fallback.
		return 0
	}
	if short == "object" || short == "Object" || short == "dynamic" {
		return 1
	}
	switch typ.GetTypeKind() {
	case ssa.StringTypeKind:
		if literalKind := csharpQuotedLiteralKind(arg); literalKind != "" {
			switch {
			case literalKind == "char" && (short == "char" || short == "Char"):
				return 20
			case literalKind == "string" && (short == "string" || short == "String"):
				return 20
			case short == "string" || short == "String" || short == "char" || short == "Char":
				return -4
			}
		}
		if short == "string" || short == "String" {
			return 16
		}
		if short == "char" || short == "Char" {
			return 8
		}
		return -4
	case ssa.BooleanTypeKind:
		if short == "bool" || short == "Boolean" {
			return 16
		}
		return -4
	case ssa.ByteTypeKind:
		if short == "byte" || short == "sbyte" || short == "Byte" || short == "SByte" {
			return 18
		}
		if isCSharpNumericType(short) {
			return 8
		}
		return -4
	case ssa.NumberTypeKind:
		if literalScore, ok := csharpNumericLiteralTypeScore(short, arg); ok {
			return literalScore
		}
		if isCSharpNumericType(short) {
			return 12
		}
		return -4
	case ssa.SliceTypeKind:
		if strings.HasSuffix(raw, "[]") || csharpSliceLikeTypes[short] {
			return 12
		}
		return -4
	case ssa.MapTypeKind:
		if csharpMapLikeTypes[short] {
			return 12
		}
		return -4
	case ssa.ClassBluePrintTypeKind:
		if blueprint, ok := ssa.ToBluePrintType(typ); ok && blueprint != nil {
			if raw == blueprint.Name || short == blueprint.Name {
				return 20
			}
			for _, fullName := range blueprint.GetFullTypeNames() {
				if raw == fullName {
					return 24
				}
			}
			if distance, fullName := csharpBlueprintConversionDistance(blueprint, raw, short); distance > 0 {
				if fullName {
					return 20 - min(distance, 8)
				}
				return 18 - min(distance, 8)
			}
		}
		return -2
	case ssa.AnyTypeKind, ssa.NullTypeKind:
		return 0
	default:
		return 0
	}
}

func csharpBlueprintConversionDistance(blueprint *ssa.Blueprint, raw, short string) (int, bool) {
	type pendingBlueprint struct {
		blueprint *ssa.Blueprint
		distance  int
	}
	seen := map[*ssa.Blueprint]struct{}{blueprint: {}}
	queue := []pendingBlueprint{{blueprint: blueprint, distance: 0}}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		current.blueprint.Build()
		if current.distance > 0 {
			if raw == current.blueprint.Name || short == current.blueprint.Name {
				return current.distance, false
			}
			for _, fullName := range current.blueprint.GetFullTypeNames() {
				if raw == fullName {
					return current.distance, true
				}
			}
		}
		next := append([]*ssa.Blueprint(nil), current.blueprint.GetParentBlueprint()...)
		next = append(next, current.blueprint.GetInterfaceBlueprint()...)
		for _, related := range next {
			if related == nil {
				continue
			}
			if _, exists := seen[related]; exists {
				continue
			}
			seen[related] = struct{}{}
			queue = append(queue, pendingBlueprint{blueprint: related, distance: current.distance + 1})
		}
	}
	return 0, false
}

func csharpQuotedLiteralKind(arg ssa.Value) string {
	literal, ok := ssa.ToConstInst(arg)
	if !ok || literal == nil || literal.GetRange() == nil {
		return ""
	}
	text := strings.TrimSpace(literal.GetRange().GetText())
	if strings.HasPrefix(text, "'") {
		return "char"
	}
	if strings.HasPrefix(text, `"`) || strings.HasPrefix(text, `@"`) {
		return "string"
	}
	return ""
}

func isCSharpNonNullableValueType(name string) bool {
	if isCSharpNumericType(name) {
		return true
	}
	switch name {
	case "bool", "Boolean", "byte", "sbyte", "Byte", "SByte", "char", "Char":
		return true
	default:
		return false
	}
}

func csharpNumericLiteralTypeScore(parameterType string, arg ssa.Value) (int, bool) {
	literal, ok := ssa.ToConstInst(arg)
	if !ok || literal == nil {
		return 0, false
	}
	value, ok := literal.GetRawValue().(int64)
	if !ok {
		return 0, false
	}
	text := ""
	if literal.GetRange() != nil {
		text = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(literal.GetRange().GetText()), "_", ""))
	}
	explicitLong := strings.HasSuffix(text, "l") || strings.HasSuffix(text, "ul") || strings.HasSuffix(text, "lu")
	if explicitLong {
		switch parameterType {
		case "long", "Int64":
			return 20, true
		default:
			if isCSharpNumericType(parameterType) {
				return 8, true
			}
			return -4, true
		}
	}
	if value >= -2147483648 && value <= 2147483647 {
		switch parameterType {
		case "int", "Int32":
			return 20, true
		default:
			if isCSharpNumericType(parameterType) {
				return 8, true
			}
			return -4, true
		}
	}
	if parameterType == "long" || parameterType == "Int64" {
		return 20, true
	}
	if isCSharpNumericType(parameterType) {
		return 8, true
	}
	return -4, true
}

func isCSharpNumericType(name string) bool {
	switch name {
	case "short", "ushort", "int", "uint", "long", "ulong", "float", "double", "decimal", "nint", "nuint",
		"Int16", "UInt16", "Int32", "UInt32", "Int64", "UInt64", "Single", "Double", "Decimal":
		return true
	default:
		return false
	}
}

func (b *singleFileBuilder) materializeCSharpArguments(
	parameters []csharpConstructorParameter,
	function *ssa.Function,
	parameterOffset int,
	arguments []csharpEvaluatedArgument,
	binding csharpArgumentBinding,
) ([]ssa.Value, []outArgument) {
	if function != nil {
		function.Build()
	}
	lastFormal := -1
	for index, indexes := range binding.formal {
		if len(indexes) != 0 || parameters[index].params {
			lastFormal = index
		}
	}
	if lastFormal < 0 {
		return nil, nil
	}

	values := make([]ssa.Value, 0, lastFormal+1)
	var outs []outArgument
	formalValue := func(index int) *ssa.Parameter {
		if function == nil || parameterOffset+index >= len(function.Params) {
			return nil
		}
		value, ok := function.GetValueById(function.Params[parameterOffset+index])
		if !ok {
			return nil
		}
		parameter, _ := ssa.ToParameter(value)
		return parameter
	}

	for formalIndex := 0; formalIndex <= lastFormal; formalIndex++ {
		parameter := parameters[formalIndex]
		indexes := binding.formal[formalIndex]
		if parameter.params && binding.expandedParams {
			parameterType := ssa.Type(ssa.NewSliceType(ssa.CreateAnyType()))
			if formal := formalValue(formalIndex); formal != nil && formal.GetType() != nil && formal.GetType().GetTypeKind() == ssa.SliceTypeKind {
				parameterType = formal.GetType()
			}
			size := b.EmitConstInst(len(indexes))
			packed := b.EmitMakeBuildWithType(parameterType, size, size)
			for elementIndex, sourceIndex := range indexes {
				argument := arguments[sourceIndex]
				if utils.IsNil(argument.value) {
					continue
				}
				b.AssignVariable(b.CreateMemberCallVariable(packed, b.EmitConstInst(elementIndex)), argument.value)
			}
			values = append(values, packed)
			continue
		}

		if len(indexes) != 0 {
			argument := arguments[indexes[0]]
			values = append(values, argument.value)
			if argument.outVariable != nil {
				outs = append(outs, outArgument{
					index:            parameterOffset + formalIndex,
					variable:         argument.outVariable,
					calleeSideEffect: parameter.modifier == "ref" || parameter.modifier == "out" || parameter.modifier == "byref",
				})
			}
			continue
		}

		// A named argument may target a later formal. Materialize any omitted
		// optional slots before it so the positional SSA vector remains aligned;
		// trailing optional formals stay omitted and use Parameter.GetDefault().
		var value ssa.Value
		if formal := formalValue(formalIndex); formal != nil {
			value = formal.GetDefault()
		}
		if utils.IsNil(value) {
			value = b.EmitUndefined(parameter.name)
		}
		values = append(values, value)
	}
	return values, outs
}

func (b *singleFileBuilder) projectConstructorReceiverState(call *ssa.Call, receiver ssa.Value) {
	if b == nil || call == nil || utils.IsNil(receiver) {
		return
	}
	for _, pair := range ssa.GetLastWinsMemberPairs(receiver) {
		if utils.IsNil(pair.Key) || utils.IsNil(pair.Member) {
			continue
		}
		// Use an ordinary member-variable assignment, not only an object-pair
		// annotation. Call argument/member binding consults the caller scope by
		// `#callID.key`, and returned class values export those variables as
		// CallMemberCall side effects for factory-style wrappers.
		b.AssignVariable(b.CreateMemberCallVariable(call, pair.Key), pair.Member)
	}
}

func csharpResolvedFunction(value ssa.Value) *ssa.Function {
	seen := make(map[int64]struct{})
	for !utils.IsNil(value) {
		if _, ok := seen[value.GetId()]; ok {
			return nil
		}
		seen[value.GetId()] = struct{}{}
		if function, ok := ssa.ToFunction(value); ok {
			function.Build()
			return function
		}
		value = value.GetReference()
	}
	return nil
}

func csharpCallableParameters(callee ssa.Value) (*ssa.Function, int, []csharpConstructorParameter) {
	function := csharpResolvedFunction(callee)
	if function == nil || function.IsExtern() {
		return nil, 0, nil
	}
	parameterOffset := 0
	functionType, _ := ssa.ToFunctionType(function.GetType())
	if functionType != nil && functionType.IsMethod {
		parameterOffset = 1
	}
	if parameterOffset > len(function.Params) {
		return nil, 0, nil
	}
	pointerParameters := make(map[int]struct{})
	if functionType != nil {
		for _, sideEffect := range functionType.SideEffects {
			if sideEffect == nil || sideEffect.Kind != ssa.PointerSideEffect || sideEffect.MemberCallKind != ssa.ParameterCall {
				continue
			}
			pointerParameters[sideEffect.MemberCallObjectIndex] = struct{}{}
		}
	}
	parameters := make([]csharpConstructorParameter, 0, len(function.Params)-parameterOffset)
	for index := parameterOffset; index < len(function.Params); index++ {
		value, ok := function.GetValueById(function.Params[index])
		if !ok {
			return nil, 0, nil
		}
		parameter, ok := ssa.ToParameter(value)
		if !ok {
			return nil, 0, nil
		}
		modifier := ""
		if _, ok := pointerParameters[index]; ok {
			modifier = "byref"
		}
		parameters = append(parameters, csharpConstructorParameter{
			name:     parameter.GetName(),
			modifier: modifier,
			optional: !utils.IsNil(parameter.GetDefault()),
			params:   functionType != nil && functionType.IsVariadic && index == len(function.Params)-1,
		})
	}
	return function, parameterOffset, parameters
}

func (b *singleFileBuilder) prepareDetailedCall(callee ssa.Value, arguments []csharpEvaluatedArgument) (ssa.Value, []ssa.Value, []outArgument) {
	function, parameterOffset, parameters := csharpCallableParameters(callee)
	if function == nil {
		values, outs := flattenEvaluatedArguments(arguments)
		return callee, values, outs
	}
	if selection := b.constructors.selectMethod(function, arguments); selection != nil {
		if selection.ambiguous {
			return b.prepareAmbiguousMethodCall(callee, nil, function.GetMethodName(), arguments)
		}
		return b.materializeSelectedMethodCall(callee, nil, selection, arguments)
	}
	binding, ok := bindCSharpArguments(parameters, arguments)
	if !ok {
		values, outs := flattenEvaluatedArguments(arguments)
		return callee, values, outs
	}
	values, outs := b.materializeCSharpArguments(parameters, function, parameterOffset, arguments, binding)
	return callee, values, outs
}

func (b *singleFileBuilder) prepareDetailedBareCall(callee ssa.Value, class *ssa.Blueprint, name string, receiver ssa.Value, arguments []csharpEvaluatedArgument) (ssa.Value, []ssa.Value, []outArgument) {
	if selection := b.constructors.selectBareMethod(class, name, !utils.IsNil(receiver), arguments); selection != nil {
		if selection.ambiguous {
			return b.prepareAmbiguousMethodCall(callee, receiver, name, arguments)
		}
		return b.materializeSelectedMethodCall(callee, receiver, selection, arguments)
	}
	return b.prepareDetailedCall(callee, arguments)
}

// prepareAmbiguousMethodCall deliberately drops the first-slot Blueprint
// reference when available SSA types cannot choose a unique overload. Entering
// an arbitrary source body is unsound because its formal order/return may hide
// another candidate's dataflow. An unknown call over every evaluated actual is
// conservative: top-def analysis sees all possible inputs, and evaluation order
// remains exactly the source order.
func (b *singleFileBuilder) prepareAmbiguousMethodCall(callee, receiver ssa.Value, name string, arguments []csharpEvaluatedArgument) (ssa.Value, []ssa.Value, []outArgument) {
	if utils.IsNil(receiver) && !utils.IsNil(callee) && callee.IsMember() {
		receiver = ssa.GetLatestObject(callee)
	}
	values, outs := flattenEvaluatedArguments(arguments)
	if !utils.IsNil(receiver) {
		values = append([]ssa.Value{receiver}, values...)
		for index := range outs {
			outs[index].index++
		}
	}
	unknown := b.EmitUndefined(name)
	unknown.SetType(ssa.NewFunctionTypeDefine(name, nil, []ssa.Type{ssa.CreateAnyType()}, false))
	return unknown, values, outs
}

func (b *singleFileBuilder) materializeSelectedMethodCall(callee, receiver ssa.Value, selection *csharpMethodSelection, arguments []csharpEvaluatedArgument) (ssa.Value, []ssa.Value, []outArgument) {
	selected := selection.method.function
	selected.Build()
	selectedCallee := ssa.Value(selected)
	selectedType, _ := ssa.ToFunctionType(selected.GetType())
	selectedOffset := 0
	if selectedType != nil && selectedType.IsMethod {
		selectedOffset = 1
		switch {
		case !utils.IsNil(receiver):
			// A bare invocation may have started from a static overload because
			// identifier lookup has no arguments yet. Rebind the chosen instance
			// overload to the current `this` without evaluating anything again.
			selectedCallee = b.bindInstanceMethod(receiver, selected.GetMethodName(), selected)
		case !utils.IsNil(callee) && callee.IsMember():
			// Do not repoint the lookup proxy: Point(proxy, first) already put it
			// in the first overload's pointer list, and changing only Reference
			// would leave a stale reverse edge. Bind a second fresh proxy straight
			// to the selected implementation while reusing the evaluated receiver.
			if qualifiedReceiver := ssa.GetLatestObject(callee); !utils.IsNil(qualifiedReceiver) {
				selectedCallee = b.bindInstanceMethod(qualifiedReceiver, selected.GetMethodName(), selected)
			}
		}
	}
	values, outs := b.materializeCSharpArguments(selection.method.parameters, selected, selectedOffset, arguments, selection.binding)
	return selectedCallee, values, outs
}

type csharpMethodStaticWrite struct {
	object ssa.Value
	path   []ssa.Value
	key    ssa.Value
	values []ssa.Value
}

type csharpMethodStateTarget struct {
	write      *csharpMethodStaticWrite
	baseline   ssa.Value
	resultName string
}

type csharpStaticRootedValue struct {
	object ssa.Value
	path   []ssa.Value
}

func csharpMemberTargetSignature(object, key ssa.Value) string {
	if utils.IsNil(object) || utils.IsNil(key) {
		return ""
	}
	name := ssa.GetKeyString(key)
	if name == "" {
		name = key.String()
	}
	return fmt.Sprintf("%d:%s", object.GetId(), name)
}

func csharpStaticWriteSignature(write *csharpMethodStaticWrite) string {
	if write == nil || utils.IsNil(write.object) {
		return ""
	}
	path := csharpConstructorReceiverWriteSignature(write.path, write.key)
	if path == "" {
		return ""
	}
	return fmt.Sprintf("%d/%s", write.object.GetId(), path)
}

func csharpStaticWriteOrder(writes map[string]*csharpMethodStaticWrite) []string {
	order := make([]string, 0, len(writes))
	for signature := range writes {
		order = append(order, signature)
	}
	sort.SliceStable(order, func(left, right int) bool {
		leftWrite, rightWrite := writes[order[left]], writes[order[right]]
		if len(leftWrite.path) != len(rightWrite.path) {
			return len(leftWrite.path) < len(rightWrite.path)
		}
		return order[left] < order[right]
	})
	return order
}

func csharpMethodReceiverMemberKeys(alternatives []csharpMethodAlternative) map[string]ssa.Value {
	keys := make(map[string]ssa.Value)
	add := func(key ssa.Value) {
		if utils.IsNil(key) {
			return
		}
		name := ssa.GetKeyString(key)
		if name == "" {
			name = key.String()
		}
		if name != "" {
			keys[name] = key
		}
	}
	for _, alternative := range alternatives {
		if alternative.method.key.static || alternative.method.function == nil {
			continue
		}
		function := alternative.method.function
		function.Build()
		functionType, _ := ssa.ToFunctionType(function.GetType())
		if functionType == nil {
			continue
		}
		for _, member := range functionType.ParameterMember {
			if member == nil || member.MemberCallKind != ssa.ParameterMemberCall || member.MemberCallObjectIndex != 0 {
				continue
			}
			key, _ := member.GetValueById(member.MemberCallKey)
			add(key)
		}
		for _, sideEffect := range functionType.SideEffects {
			if sideEffect == nil || sideEffect.MemberCallKind != ssa.ParameterMemberCall || sideEffect.MemberCallObjectIndex != 0 {
				continue
			}
			key, _ := function.GetValueById(sideEffect.MemberCallKey)
			add(key)
		}
	}
	return keys
}

func walkCSharpFunctionValues(function *ssa.Function, visit func(ssa.Value)) {
	if function == nil || visit == nil {
		return
	}
	function.Build()
	seen := make(map[int64]struct{})
	inspectValue := func(value ssa.Value) {
		if utils.IsNil(value) {
			return
		}
		if _, exists := seen[value.GetId()]; exists {
			return
		}
		seen[value.GetId()] = struct{}{}
		visit(value)
	}
	for _, parameterID := range function.Params {
		parameter, ok := function.GetValueById(parameterID)
		if ok {
			inspectValue(parameter)
		}
	}
	for _, memberID := range function.ParameterMembers {
		member, ok := function.GetValueById(memberID)
		if ok {
			inspectValue(member)
		}
	}
	for _, blockID := range function.Blocks {
		block, ok := function.GetBasicBlockByID(blockID)
		if !ok || block == nil {
			continue
		}
		instructionIDs := append(append([]int64(nil), block.Insts...), block.Phis...)
		for _, instructionID := range instructionIDs {
			instruction, ok := function.GetInstructionById(instructionID)
			if !ok {
				continue
			}
			value, ok := ssa.ToValue(instruction)
			if ok {
				inspectValue(value)
			}
		}
	}
}

func walkCSharpFunctionAssignments(function *ssa.Function, visit func(ssa.Value, *ssa.Variable)) {
	if visit == nil {
		return
	}
	walkCSharpFunctionValues(function, func(value ssa.Value) {
		variables := value.GetAllVariables()
		names := make([]string, 0, len(variables))
		for name := range variables {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			variable := variables[name]
			visit(value, variable)
		}
	})
}

// csharpFunctionStaticWrites recovers assignments to blueprint containers.
// The generic SSA side-effect model handles parameters, free values, and
// receiver members, but a C# static field is a member of a global blueprint
// container and is not currently exported in FunctionType.SideEffects.
func csharpDirectStaticWrites(function *ssa.Function) map[string]*csharpMethodStaticWrite {
	writes := make(map[string]*csharpMethodStaticWrite)
	rootedValues := make(map[int64][]csharpStaticRootedValue)
	addRootedValue := func(value ssa.Value, root csharpStaticRootedValue) {
		if utils.IsNil(value) || utils.IsNil(root.object) {
			return
		}
		signature := fmt.Sprintf("%d/%s", root.object.GetId(), csharpValuePathSignature(root.path))
		for _, existing := range rootedValues[value.GetId()] {
			existingSignature := fmt.Sprintf("%d/%s", existing.object.GetId(), csharpValuePathSignature(existing.path))
			if existingSignature == signature {
				return
			}
		}
		root.path = append([]ssa.Value(nil), root.path...)
		rootedValues[value.GetId()] = append(rootedValues[value.GetId()], root)
	}
	var seedMembers func(ssa.Value, csharpStaticRootedValue, map[int64]struct{})
	seedMembers = func(value ssa.Value, root csharpStaticRootedValue, visiting map[int64]struct{}) {
		if utils.IsNil(value) {
			return
		}
		if _, exists := visiting[value.GetId()]; exists {
			return
		}
		visiting[value.GetId()] = struct{}{}
		defer delete(visiting, value.GetId())
		addRootedValue(value, root)
		for _, pair := range ssa.GetLastWinsMemberPairs(value) {
			if utils.IsNil(pair.Key) || utils.IsNil(pair.Member) {
				continue
			}
			child := csharpStaticRootedValue{
				object: root.object,
				path:   append(append([]ssa.Value(nil), root.path...), pair.Key),
			}
			seedMembers(pair.Member, child, visiting)
		}
	}
	if program := function.GetProgram(); program != nil && program.Blueprint != nil {
		program.Blueprint.ForEach(func(_ string, blueprint *ssa.Blueprint) bool {
			if blueprint == nil || utils.IsNil(blueprint.Container()) {
				return true
			}
			container := blueprint.Container()
			addRootedValue(container, csharpStaticRootedValue{object: container})
			for _, values := range blueprint.StaticMember {
				for _, memberValue := range values {
					if utils.IsNil(memberValue) {
						continue
					}
					for _, variable := range memberValue.GetAllVariables() {
						if variable == nil || !variable.IsMemberCall() {
							continue
						}
						object, key := variable.GetMemberCall()
						if utils.IsNil(object) || object.GetId() != container.GetId() || utils.IsNil(key) {
							continue
						}
						seedMembers(memberValue, csharpStaticRootedValue{object: container, path: []ssa.Value{key}}, make(map[int64]struct{}))
					}
				}
			}
			return true
		})
	}
	staticPaths := func(object ssa.Value) []csharpStaticRootedValue {
		if utils.IsNil(object) {
			return nil
		}
		if roots := rootedValues[object.GetId()]; len(roots) != 0 {
			return roots
		}
		if cast, ok := ssa.ToTypeCast(object); ok {
			operand, exists := cast.GetValueById(cast.Value)
			if exists {
				return rootedValues[operand.GetId()]
			}
		}
		return nil
	}
	walkCSharpFunctionAssignments(function, func(value ssa.Value, variable *ssa.Variable) {
		if variable == nil || !variable.IsMemberCall() {
			return
		}
		object, key := variable.GetMemberCall()
		for _, root := range staticPaths(object) {
			write := &csharpMethodStaticWrite{object: root.object, path: root.path, key: key}
			signature := csharpStaticWriteSignature(write)
			if signature == "" {
				continue
			}
			if existing := writes[signature]; existing != nil {
				write = existing
			} else {
				writes[signature] = write
			}
			duplicate := false
			for _, existing := range write.values {
				if existing.GetId() == value.GetId() {
					duplicate = true
					break
				}
			}
			if !duplicate {
				write.values = append(write.values, value)
			}
			child := csharpStaticRootedValue{
				object: root.object,
				path:   append(append([]ssa.Value(nil), root.path...), key),
			}
			seedMembers(value, child, make(map[int64]struct{}))
		}
	})
	return writes
}

func csharpFunctionStaticWrites(function *ssa.Function) map[string]*csharpMethodStaticWrite {
	return csharpFunctionStaticWritesRecursive(function, make(map[int64]struct{}))
}

func csharpFunctionStaticWritesRecursive(function *ssa.Function, visiting map[int64]struct{}) map[string]*csharpMethodStaticWrite {
	if function == nil {
		return nil
	}
	if _, exists := visiting[function.GetId()]; exists {
		return nil
	}
	visiting[function.GetId()] = struct{}{}
	defer delete(visiting, function.GetId())
	writes := csharpDirectStaticWrites(function)
	walkCSharpFunctionValues(function, func(value ssa.Value) {
		call, ok := ssa.ToCall(value)
		if !ok || call == nil {
			return
		}
		method, ok := call.GetValueById(call.Method)
		if !ok {
			return
		}
		callee := csharpResolvedFunction(method)
		if callee == nil || callee.IsExtern() {
			return
		}
		for _, nested := range csharpFunctionStaticWritesRecursive(callee, visiting) {
			nested = csharpActualizeStaticWriteForCall(call, nested)
			signature := csharpStaticWriteSignature(nested)
			if signature == "" {
				continue
			}
			write := writes[signature]
			if write == nil {
				write = &csharpMethodStaticWrite{
					object: nested.object,
					path:   append([]ssa.Value(nil), nested.path...),
					key:    nested.key,
				}
				writes[signature] = write
			}
			for _, nestedValue := range nested.values {
				actualValue := csharpMethodCallValue(call, nestedValue)
				if utils.IsNil(actualValue) {
					continue
				}
				duplicate := false
				for _, existing := range write.values {
					if existing.GetId() == actualValue.GetId() {
						duplicate = true
						break
					}
				}
				if !duplicate {
					write.values = append(write.values, actualValue)
				}
			}
		}
	})
	return writes
}

func csharpMethodStaticWrites(method csharpMethod) map[string]*csharpMethodStaticWrite {
	return csharpFunctionStaticWrites(method.function)
}

func csharpMethodCallValue(call *ssa.Call, value ssa.Value) ssa.Value {
	if call == nil || utils.IsNil(value) {
		return value
	}
	if parameter, ok := ssa.ToParameter(value); ok {
		if parameter.IsFreeValue || parameter.FormalParameterIndex < 0 || parameter.FormalParameterIndex >= len(call.Args) {
			return value
		}
		actual, ok := call.GetValueById(call.Args[parameter.FormalParameterIndex])
		if ok && !utils.IsNil(actual) {
			return actual
		}
	}
	if member, ok := ssa.ToParameterMember(value); ok {
		if member.FormalParameterIndex < 0 || member.FormalParameterIndex >= len(call.ArgMember) {
			return value
		}
		actual, ok := call.GetValueById(call.ArgMember[member.FormalParameterIndex])
		if ok && !utils.IsNil(actual) {
			return actual
		}
	}
	return value
}

func csharpActualizeStaticWriteForCall(call *ssa.Call, write *csharpMethodStaticWrite) *csharpMethodStaticWrite {
	if write == nil {
		return nil
	}
	actual := &csharpMethodStaticWrite{
		object: write.object,
		path:   make([]ssa.Value, len(write.path)),
		key:    csharpMethodCallValue(call, write.key),
		values: write.values,
	}
	for index, key := range write.path {
		actual.path[index] = csharpMethodCallValue(call, key)
	}
	return actual
}

func csharpActualizeStaticWritesForBinding(
	writes map[string]*csharpMethodStaticWrite,
	parameterOffset int,
	binding csharpArgumentBinding,
	arguments []csharpEvaluatedArgument,
) map[string]*csharpMethodStaticWrite {
	actualize := func(value ssa.Value) ssa.Value {
		parameter, ok := ssa.ToParameter(value)
		if !ok || parameter.IsFreeValue {
			return value
		}
		formalIndex := parameter.FormalParameterIndex - parameterOffset
		if formalIndex < 0 || formalIndex >= len(binding.formal) || len(binding.formal[formalIndex]) == 0 {
			return value
		}
		sourceIndex := binding.formal[formalIndex][0]
		if sourceIndex < 0 || sourceIndex >= len(arguments) || utils.IsNil(arguments[sourceIndex].value) {
			return value
		}
		return arguments[sourceIndex].value
	}
	actualWrites := make(map[string]*csharpMethodStaticWrite, len(writes))
	for _, signature := range csharpStaticWriteOrder(writes) {
		write := writes[signature]
		actual := &csharpMethodStaticWrite{
			object: write.object,
			path:   make([]ssa.Value, len(write.path)),
			key:    actualize(write.key),
			values: write.values,
		}
		for index, key := range write.path {
			actual.path[index] = actualize(key)
		}
		actualSignature := csharpStaticWriteSignature(actual)
		if actualSignature == "" {
			continue
		}
		if existing := actualWrites[actualSignature]; existing != nil {
			existing.values = append(existing.values, actual.values...)
			continue
		}
		actualWrites[actualSignature] = actual
	}
	return actualWrites
}

func csharpActualizeReceiverWriteForCall(call *ssa.Call, write *csharpConstructorReceiverWrite) *csharpConstructorReceiverWrite {
	if write == nil {
		return nil
	}
	actual := &csharpConstructorReceiverWrite{
		path:   make([]ssa.Value, len(write.path)),
		key:    csharpMethodCallValue(call, write.key),
		values: write.values,
	}
	for index, key := range write.path {
		actual.path[index] = csharpMethodCallValue(call, key)
	}
	return actual
}

func csharpActualizeReceiverWriteForBinding(
	write *csharpConstructorReceiverWrite,
	parameterOffset int,
	binding csharpArgumentBinding,
	arguments []csharpEvaluatedArgument,
) *csharpConstructorReceiverWrite {
	if write == nil {
		return nil
	}
	actualize := func(value ssa.Value) ssa.Value {
		parameter, ok := ssa.ToParameter(value)
		if !ok || parameter.IsFreeValue {
			return value
		}
		formalIndex := parameter.FormalParameterIndex - parameterOffset
		if formalIndex < 0 || formalIndex >= len(binding.formal) || len(binding.formal[formalIndex]) == 0 {
			return value
		}
		sourceIndex := binding.formal[formalIndex][0]
		if sourceIndex < 0 || sourceIndex >= len(arguments) || utils.IsNil(arguments[sourceIndex].value) {
			return value
		}
		return arguments[sourceIndex].value
	}
	actual := &csharpConstructorReceiverWrite{
		path:   make([]ssa.Value, len(write.path)),
		key:    actualize(write.key),
		values: write.values,
	}
	for index, key := range write.path {
		actual.path[index] = actualize(key)
	}
	return actual
}

func csharpActualizeReceiverWritesForBinding(
	writes map[string]*csharpConstructorReceiverWrite,
	parameterOffset int,
	binding csharpArgumentBinding,
	arguments []csharpEvaluatedArgument,
) map[string]*csharpConstructorReceiverWrite {
	actualWrites := make(map[string]*csharpConstructorReceiverWrite, len(writes))
	for _, signature := range csharpReceiverWriteOrder(writes) {
		actual := csharpActualizeReceiverWriteForBinding(writes[signature], parameterOffset, binding, arguments)
		actualSignature := csharpConstructorReceiverWriteSignature(actual.path, actual.key)
		if actualSignature == "" {
			continue
		}
		if existing := actualWrites[actualSignature]; existing != nil {
			existing.values = append(existing.values, actual.values...)
			continue
		}
		actualWrites[actualSignature] = actual
	}
	return actualWrites
}

func (b *singleFileBuilder) projectMethodExplicitWrites(result, callee, receiver ssa.Value) {
	call, ok := ssa.ToCall(result)
	if !ok || call == nil {
		return
	}
	function := csharpResolvedFunction(callee)
	if function == nil {
		return
	}
	staticWrites := csharpFunctionStaticWrites(function)
	for _, signature := range csharpStaticWriteOrder(staticWrites) {
		write := csharpActualizeStaticWriteForCall(call, staticWrites[signature])
		object, key, ok := b.resolveCSharpStaticWrite(write)
		if !ok {
			continue
		}
		merged := b.emitCSharpCallWrite("method.static."+signature, call, write.values)
		if utils.IsNil(merged) {
			continue
		}
		b.AssignVariable(b.CreateMemberCallVariable(object, key), merged)
	}
	functionType, _ := ssa.ToFunctionType(function.GetType())
	if functionType == nil || !functionType.IsMethod {
		return
	}
	if utils.IsNil(receiver) && len(call.Args) != 0 {
		receiver, _ = call.GetValueById(call.Args[0])
	}
	if utils.IsNil(receiver) {
		return
	}
	writes := csharpConstructorReceiverWrites(function)
	for _, signature := range csharpReceiverWriteOrder(writes) {
		write := csharpActualizeReceiverWriteForCall(call, writes[signature])
		object, key, ok := b.resolveConstructorReceiverWrite(receiver, write)
		if !ok {
			continue
		}
		merged := b.emitCSharpCallWrite("method.receiver."+signature, call, write.values)
		if !utils.IsNil(merged) {
			b.AssignVariable(b.CreateMemberCallVariable(object, key), merged)
		}
	}
}

func csharpMethodReturnMemberKeys(alternatives []csharpMethodAlternative) map[string]ssa.Value {
	keys := make(map[string]ssa.Value)
	for _, alternative := range alternatives {
		function := alternative.method.function
		if function == nil {
			continue
		}
		function.Build()
		for _, returnID := range function.Return {
			instruction, ok := function.GetInstructionById(returnID)
			if !ok {
				continue
			}
			returned, ok := ssa.ToReturn(instruction)
			if !ok {
				continue
			}
			for _, resultID := range returned.Results {
				value, ok := function.GetValueById(resultID)
				if !ok || utils.IsNil(value) {
					continue
				}
				for _, pair := range ssa.GetLastWinsMemberPairs(value) {
					if utils.IsNil(pair.Key) {
						continue
					}
					name := pair.KeyString()
					if name == "" {
						name = ssa.GetKeyString(pair.Key)
					}
					if name != "" {
						keys[name] = pair.Key
					}
				}
			}
		}
	}
	return keys
}

// emitAmbiguousMethodCalls lowers equally-ranked overloads into mutually
// exclusive synthetic branches. Branch scopes give every candidate the same
// receiver/global/ref baseline and let the generic SSA merge create phis for
// return values and all side effects at the join. The receiver and evaluated
// actual values are captured before this helper, so no source expression is
// evaluated more than once.
func (b *singleFileBuilder) emitAmbiguousMethodCalls(
	callee, receiver ssa.Value,
	name string,
	arguments []csharpEvaluatedArgument,
	selection *csharpMethodSelection,
	nonVirtual bool,
) ssa.Value {
	if b == nil || selection == nil || len(selection.best) < 2 {
		return nil
	}
	dispatch := b.EmitUndefined("$overload-dispatch")
	resultName := fmt.Sprintf("$overload-result-%d", dispatch.GetId())
	b.AssignVariable(b.CreateVariable(resultName), b.EmitValueOnlyDeclare(resultName))

	actualReceiver := receiver
	if utils.IsNil(actualReceiver) && !utils.IsNil(callee) && callee.IsMember() {
		actualReceiver = ssa.GetLatestObject(callee)
	}
	targets := make(map[string]*csharpMethodStateTarget)
	var targetOrder []string
	addTarget := func(write *csharpMethodStaticWrite) {
		signature := csharpStaticWriteSignature(write)
		if signature == "" {
			return
		}
		if _, exists := targets[signature]; exists {
			return
		}
		targets[signature] = &csharpMethodStateTarget{write: write}
		targetOrder = append(targetOrder, signature)
	}
	receiverTargets := make(map[string]*csharpReceiverStateTarget)
	addReceiverTarget := func(write *csharpConstructorReceiverWrite) {
		if write == nil || utils.IsNil(actualReceiver) {
			return
		}
		signature := csharpConstructorReceiverWriteSignature(write.path, write.key)
		if signature == "" {
			return
		}
		if _, exists := receiverTargets[signature]; !exists {
			receiverTargets[signature] = &csharpReceiverStateTarget{write: write}
		}
	}
	if !utils.IsNil(actualReceiver) {
		receiverKeys := csharpMethodReceiverMemberKeys(selection.best)
		names := make([]string, 0, len(receiverKeys))
		for memberName := range receiverKeys {
			names = append(names, memberName)
		}
		sort.Strings(names)
		for _, memberName := range names {
			addReceiverTarget(&csharpConstructorReceiverWrite{key: receiverKeys[memberName]})
		}
	}
	receiverWrites := make([]map[string]*csharpConstructorReceiverWrite, len(selection.best))
	staticWrites := make([]map[string]*csharpMethodStaticWrite, len(selection.best))
	for index, alternative := range selection.best {
		receiverWrites[index] = csharpConstructorReceiverWrites(alternative.method.function)
		parameterOffset := 0
		if !alternative.method.key.static {
			parameterOffset = 1
		}
		receiverWrites[index] = csharpActualizeReceiverWritesForBinding(
			receiverWrites[index], parameterOffset, alternative.binding, arguments,
		)
		for _, signature := range csharpReceiverWriteOrder(receiverWrites[index]) {
			addReceiverTarget(receiverWrites[index][signature])
		}
		staticWrites[index] = csharpActualizeStaticWritesForBinding(
			csharpMethodStaticWrites(alternative.method), parameterOffset, alternative.binding, arguments,
		)
		for _, signature := range csharpStaticWriteOrder(staticWrites[index]) {
			addTarget(staticWrites[index][signature])
		}
	}
	sort.SliceStable(targetOrder, func(left, right int) bool {
		leftWrite, rightWrite := targets[targetOrder[left]].write, targets[targetOrder[right]].write
		if len(leftWrite.path) != len(rightWrite.path) {
			return len(leftWrite.path) < len(rightWrite.path)
		}
		return targetOrder[left] < targetOrder[right]
	})
	for index, signature := range targetOrder {
		target := targets[signature]
		if object, key, ok := b.resolveCSharpStaticWrite(target.write); ok {
			target.baseline = b.ReadMemberCallValue(object, key)
		}
		if utils.IsNil(target.baseline) {
			target.baseline = b.EmitUndefined(ssa.GetKeyString(target.write.key))
		}
		target.resultName = fmt.Sprintf("%s.state.%d", resultName, index)
		b.AssignVariable(b.CreateVariable(target.resultName), b.EmitValueOnlyDeclare(target.resultName))
	}
	receiverTargetOrder := make([]string, 0, len(receiverTargets))
	for signature := range receiverTargets {
		receiverTargetOrder = append(receiverTargetOrder, signature)
	}
	sort.SliceStable(receiverTargetOrder, func(left, right int) bool {
		leftWrite := receiverTargets[receiverTargetOrder[left]].write
		rightWrite := receiverTargets[receiverTargetOrder[right]].write
		if len(leftWrite.path) != len(rightWrite.path) {
			return len(leftWrite.path) < len(rightWrite.path)
		}
		return receiverTargetOrder[left] < receiverTargetOrder[right]
	})
	for index, signature := range receiverTargetOrder {
		target := receiverTargets[signature]
		if object, key, ok := b.resolveConstructorReceiverWrite(actualReceiver, target.write); ok {
			target.baseline = b.ReadMemberCallValue(object, key)
		}
		if utils.IsNil(target.baseline) {
			target.baseline = b.EmitUndefined(ssa.GetKeyString(target.write.key))
		}
		target.resultName = fmt.Sprintf("%s.receiver-state.%d", resultName, index)
		b.AssignVariable(b.CreateVariable(target.resultName), b.EmitValueOnlyDeclare(target.resultName))
	}
	returnMemberKeys := csharpMethodReturnMemberKeys(selection.best)
	returnMemberOrder := make([]string, 0, len(returnMemberKeys))
	for memberName := range returnMemberKeys {
		returnMemberOrder = append(returnMemberOrder, memberName)
	}
	sort.Strings(returnMemberOrder)
	returnMemberResults := make(map[string]string, len(returnMemberOrder))
	for index, memberName := range returnMemberOrder {
		stateName := fmt.Sprintf("%s.return-state.%d", resultName, index)
		returnMemberResults[memberName] = stateName
		b.AssignVariable(b.CreateVariable(stateName), b.EmitValueOnlyDeclare(stateName))
	}
	emitCandidate := func(index int, alternative csharpMethodAlternative) {
		candidate := &csharpMethodSelection{method: alternative.method, binding: alternative.binding}
		selectedCallee, args, outs := b.materializeSelectedMethodCall(callee, receiver, candidate, arguments)
		var result ssa.Value
		if nonVirtual {
			result = b.emitBaseCall(selectedCallee, args, outs, name)
		} else {
			result = b.emitCall(selectedCallee, args, outs, name)
		}
		if utils.IsNil(result) {
			result = b.EmitUndefined(name)
		}
		if call, ok := ssa.ToCall(result); ok {
			for _, signature := range csharpReceiverWriteOrder(receiverWrites[index]) {
				write := receiverWrites[index][signature]
				object, key, ok := b.resolveConstructorReceiverWrite(actualReceiver, write)
				if !ok {
					continue
				}
				merged := b.emitCSharpCallWrite("ambiguous-method.receiver."+signature, call, write.values)
				if !utils.IsNil(merged) {
					b.AssignVariable(b.CreateMemberCallVariable(object, key), merged)
				}
			}
			for _, signature := range csharpStaticWriteOrder(staticWrites[index]) {
				write := csharpActualizeStaticWriteForCall(call, staticWrites[index][signature])
				object, key, ok := b.resolveCSharpStaticWrite(write)
				if !ok {
					continue
				}
				var sideEffects []ssa.Value
				for _, value := range write.values {
					actualValue := csharpMethodCallValue(call, value)
					if sideEffect := b.EmitSideEffect(ssa.GetKeyString(write.key), call, actualValue); sideEffect != nil {
						sideEffects = append(sideEffects, sideEffect)
					}
				}
				merged := b.mergeConstructorValues("ambiguous-method.static."+ssa.GetKeyString(write.key), sideEffects)
				if !utils.IsNil(merged) {
					b.AssignVariable(b.CreateMemberCallVariable(object, key), merged)
				}
			}
		}
		for _, signature := range targetOrder {
			target := targets[signature]
			var value ssa.Value
			if object, key, ok := b.resolveCSharpStaticWrite(target.write); ok {
				value = b.ReadMemberCallValue(object, key)
			}
			if utils.IsNil(value) {
				value = target.baseline
			}
			b.AssignVariable(b.CreateVariable(target.resultName), value)
		}
		for _, signature := range receiverTargetOrder {
			target := receiverTargets[signature]
			var value ssa.Value
			if object, key, ok := b.resolveConstructorReceiverWrite(actualReceiver, target.write); ok {
				value = b.ReadMemberCallValue(object, key)
			}
			if utils.IsNil(value) {
				value = target.baseline
			}
			b.AssignVariable(b.CreateVariable(target.resultName), value)
		}
		for _, memberName := range returnMemberOrder {
			value := b.ReadMemberCallValue(result, returnMemberKeys[memberName])
			if utils.IsNil(value) {
				value = b.EmitUndefined(memberName)
			}
			b.AssignVariable(b.CreateVariable(returnMemberResults[memberName]), value)
		}
		b.AssignVariable(b.CreateVariable(resultName), result)
	}

	branches := b.CreateIfBuilder()
	last := len(selection.best) - 1
	for index, alternative := range selection.best {
		index, alternative := index, alternative
		body := func() { emitCandidate(index, alternative) }
		if index == last {
			branches.SetElse(body)
			break
		}
		branches.AppendItem(func() ssa.Value {
			return b.EmitBinOp(ssa.OpEq, dispatch, b.EmitConstInst(index))
		}, body)
	}
	branches.Build()
	result := b.ReadValue(resultName)
	for _, signature := range targetOrder {
		target := targets[signature]
		merged := b.ReadValue(target.resultName)
		if utils.IsNil(merged) {
			merged = target.baseline
		}
		if object, key, ok := b.resolveCSharpStaticWrite(target.write); ok {
			b.AssignVariable(b.CreateMemberCallVariable(object, key), merged)
		}
	}
	for _, signature := range receiverTargetOrder {
		target := receiverTargets[signature]
		merged := b.ReadValue(target.resultName)
		if utils.IsNil(merged) {
			merged = target.baseline
		}
		if object, key, ok := b.resolveConstructorReceiverWrite(actualReceiver, target.write); ok {
			b.AssignVariable(b.CreateMemberCallVariable(object, key), merged)
		}
	}
	for _, memberName := range returnMemberOrder {
		merged := b.ReadValue(returnMemberResults[memberName])
		if !utils.IsNil(merged) {
			b.AssignVariable(b.CreateMemberCallVariable(result, returnMemberKeys[memberName]), merged)
		}
	}
	return result
}

func (b *singleFileBuilder) emitAmbiguousDetailedMethodCall(callee ssa.Value, arguments []csharpEvaluatedArgument, name string, nonVirtual bool) (ssa.Value, bool) {
	function := csharpResolvedFunction(callee)
	if function == nil {
		return nil, false
	}
	selection := b.constructors.selectMethod(function, arguments)
	if selection == nil || !selection.ambiguous {
		return nil, false
	}
	return b.emitAmbiguousMethodCalls(callee, nil, name, arguments, selection, nonVirtual), true
}

func (b *singleFileBuilder) emitAmbiguousDetailedBareMethodCall(callee ssa.Value, class *ssa.Blueprint, name string, receiver ssa.Value, arguments []csharpEvaluatedArgument) (ssa.Value, bool) {
	selection := b.constructors.selectBareMethod(class, name, !utils.IsNil(receiver), arguments)
	if selection == nil || !selection.ambiguous {
		return nil, false
	}
	return b.emitAmbiguousMethodCalls(callee, receiver, name, arguments, selection, false), true
}

// projectReturnedConstructorState carries member state through a thin factory
// wrapper. The generic call result has the declared class type, but without
// this projection a subsequent `.Field` read falls back to the blueprint's
// initializer before top-def traversal reaches the returned constructor call.
func (b *singleFileBuilder) projectReturnedConstructorState(call *ssa.Call, callee ssa.Value) {
	if b == nil || call == nil {
		return
	}
	function := csharpResolvedFunction(callee)
	if function == nil {
		return
	}
	for _, returnID := range function.Return {
		instruction, ok := function.GetInstructionById(returnID)
		if !ok {
			continue
		}
		returned, ok := ssa.ToReturn(instruction)
		if !ok {
			continue
		}
		for _, resultID := range returned.Results {
			result, ok := function.GetValueById(resultID)
			if !ok || utils.IsNil(result) {
				continue
			}
			constructorCall, ok := ssa.ToCall(result)
			if !ok {
				continue
			}
			class, ok := ssa.ToBluePrintType(result.GetType())
			if !ok || class == nil {
				continue
			}
			method, ok := constructorCall.GetValueById(constructorCall.Method)
			if !ok {
				continue
			}
			constructor, ok := ssa.ToFunction(method)
			if !ok || !b.isConstructorInHierarchy(class, constructor) {
				continue
			}
			b.projectConstructorReceiverState(call, result)
		}
	}
}

type csharpConstructorReceiverWrite struct {
	path   []ssa.Value
	key    ssa.Value
	values []ssa.Value
}

type csharpReceiverStateTarget struct {
	write      *csharpConstructorReceiverWrite
	baseline   ssa.Value
	resultName string
}

func csharpConstructorParameterMemberPath(function *ssa.Function, member *ssa.ParameterMember, visited map[int64]struct{}) (int, []ssa.Value, bool) {
	if function == nil || member == nil {
		return 0, nil, false
	}
	if _, exists := visited[member.GetId()]; exists {
		return 0, nil, false
	}
	visited[member.GetId()] = struct{}{}
	key, ok := function.GetValueById(member.MemberCallKey)
	if !ok || utils.IsNil(key) {
		return 0, nil, false
	}
	switch member.MemberCallKind {
	case ssa.ParameterMemberCall:
		return member.MemberCallObjectIndex, []ssa.Value{key}, true
	case ssa.MoreParameterMember:
		if member.MemberCallObjectIndex < 0 || member.MemberCallObjectIndex >= len(function.ParameterMembers) {
			return 0, nil, false
		}
		parentValue, ok := function.GetValueById(function.ParameterMembers[member.MemberCallObjectIndex])
		if !ok {
			return 0, nil, false
		}
		parent, ok := ssa.ToParameterMember(parentValue)
		if !ok {
			return 0, nil, false
		}
		root, path, ok := csharpConstructorParameterMemberPath(function, parent, visited)
		if !ok {
			return 0, nil, false
		}
		return root, append(path, key), true
	default:
		return 0, nil, false
	}
}

func csharpValuePathSignature(path []ssa.Value) string {
	var signature strings.Builder
	for _, segment := range path {
		if utils.IsNil(segment) {
			return ""
		}
		name := ssa.GetKeyString(segment)
		if name == "" {
			name = segment.String()
		}
		if name == "" {
			return ""
		}
		fmt.Fprintf(&signature, "%d:%s;", len(name), name)
	}
	return signature.String()
}

func csharpConstructorReceiverWriteSignature(path []ssa.Value, key ssa.Value) string {
	if utils.IsNil(key) {
		return ""
	}
	return csharpValuePathSignature(append(append([]ssa.Value(nil), path...), key))
}

func csharpReceiverWriteOrder(writes map[string]*csharpConstructorReceiverWrite) []string {
	order := make([]string, 0, len(writes))
	for signature := range writes {
		order = append(order, signature)
	}
	sort.SliceStable(order, func(left, right int) bool {
		leftWrite, rightWrite := writes[order[left]], writes[order[right]]
		if len(leftWrite.path) != len(rightWrite.path) {
			return len(leftWrite.path) < len(rightWrite.path)
		}
		return order[left] < order[right]
	})
	return order
}

// csharpConstructorReceiverWrites recovers receiver writes that the generic
// side-effect model cannot export, most importantly this.Cell.Value where the
// assigned variable is rooted at a ParameterMember rather than a Parameter.
func csharpDirectReceiverWrites(function *ssa.Function) map[string]*csharpConstructorReceiverWrite {
	writes := make(map[string]*csharpConstructorReceiverWrite)
	rootedValues := make(map[int64][]ssa.Value)
	var receiverPath func(ssa.Value) ([]ssa.Value, bool)
	receiverPath = func(object ssa.Value) ([]ssa.Value, bool) {
		if utils.IsNil(object) {
			return nil, false
		}
		if parameter, ok := ssa.ToParameter(object); ok {
			return nil, !parameter.IsFreeValue && parameter.FormalParameterIndex == 0
		}
		if member, ok := ssa.ToParameterMember(object); ok {
			root, path, ok := csharpConstructorParameterMemberPath(function, member, make(map[int64]struct{}))
			return path, ok && root == 0
		}
		if path, ok := rootedValues[object.GetId()]; ok {
			return append([]ssa.Value(nil), path...), true
		}
		if cast, ok := ssa.ToTypeCast(object); ok {
			operand, exists := cast.GetValueById(cast.Value)
			if exists {
				return receiverPath(operand)
			}
		}
		return nil, false
	}
	walkCSharpFunctionAssignments(function, func(value ssa.Value, variable *ssa.Variable) {
		if variable == nil || !variable.IsMemberCall() {
			return
		}
		object, key := variable.GetMemberCall()
		path, ok := receiverPath(object)
		if !ok {
			return
		}
		signature := csharpConstructorReceiverWriteSignature(path, key)
		if signature == "" {
			return
		}
		write := writes[signature]
		if write == nil {
			write = &csharpConstructorReceiverWrite{path: append([]ssa.Value(nil), path...), key: key}
			writes[signature] = write
		}
		for _, existing := range write.values {
			if existing.GetId() == value.GetId() {
				return
			}
		}
		write.values = append(write.values, value)
		rootedValues[value.GetId()] = append(append([]ssa.Value(nil), path...), key)
	})
	return writes
}

func csharpConstructorReceiverWrites(function *ssa.Function) map[string]*csharpConstructorReceiverWrite {
	return csharpDirectReceiverWrites(function)
}

// constructorReceiverMemberKeys returns the first member segment of every
// receiver read/write performed by the tied constructors. Restoring those
// members before each speculative call keeps one candidate's side effects from
// becoming another candidate's input. MoreParameterMember chains always have
// their first ParameterMember in FunctionType.ParameterMember, so resetting the
// root also isolates nested member reads.
func constructorReceiverMemberKeys(selections []csharpConstructorSelection) map[string]ssa.Value {
	keys := make(map[string]ssa.Value)
	add := func(key ssa.Value) {
		if utils.IsNil(key) {
			return
		}
		name := ssa.GetKeyString(key)
		if name == "" {
			name = key.String()
		}
		if name != "" {
			keys[name] = key
		}
	}
	for _, selection := range selections {
		function := selection.constructor.function
		if function == nil {
			continue
		}
		function.Build()
		functionType, _ := ssa.ToFunctionType(function.GetType())
		if functionType == nil {
			continue
		}
		for _, member := range functionType.ParameterMember {
			if member == nil || member.MemberCallKind != ssa.ParameterMemberCall || member.MemberCallObjectIndex != 0 {
				continue
			}
			key, _ := member.GetValueById(member.MemberCallKey)
			add(key)
		}
		for _, sideEffect := range functionType.SideEffects {
			if sideEffect == nil {
				continue
			}
			isReceiverMember := sideEffect.MemberCallKind == ssa.ParameterMemberCall && sideEffect.MemberCallObjectIndex == 0
			if !isReceiverMember && sideEffect.MemberCallKind != ssa.CallMemberCall {
				continue
			}
			key, _ := function.GetValueById(sideEffect.MemberCallKey)
			add(key)
		}
	}
	return keys
}

func (b *singleFileBuilder) mergeConstructorValues(name string, values []ssa.Value) ssa.Value {
	filtered := make([]ssa.Value, 0, len(values))
	for _, value := range values {
		if !utils.IsNil(value) {
			filtered = append(filtered, value)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	first := filtered[0]
	allSame := true
	for _, value := range filtered[1:] {
		if value.GetId() != first.GetId() {
			allSame = false
			break
		}
	}
	if allSame {
		return first
	}
	phi := b.EmitPhi(name, filtered)
	if phi == nil {
		return first
	}
	mergedType := first.GetType()
	for _, value := range filtered[1:] {
		if mergedType == nil || value.GetType() == nil || !ssa.TypeCompare(mergedType, value.GetType()) {
			mergedType = ssa.CreateAnyType()
			break
		}
	}
	if mergedType == nil {
		mergedType = ssa.CreateAnyType()
	}
	phi.SetType(mergedType)
	return phi
}

func (b *singleFileBuilder) resolveConstructorReceiverWrite(receiver ssa.Value, write *csharpConstructorReceiverWrite) (ssa.Value, ssa.Value, bool) {
	if b == nil || utils.IsNil(receiver) || write == nil || utils.IsNil(write.key) {
		return nil, nil, false
	}
	object := receiver
	for _, key := range write.path {
		object = b.ReadMemberCallValue(object, key)
		if utils.IsNil(object) {
			return nil, nil, false
		}
	}
	return object, write.key, true
}

func (b *singleFileBuilder) resolveCSharpStaticWrite(write *csharpMethodStaticWrite) (ssa.Value, ssa.Value, bool) {
	if b == nil || write == nil || utils.IsNil(write.object) || utils.IsNil(write.key) {
		return nil, nil, false
	}
	object := write.object
	for _, key := range write.path {
		object = b.ReadMemberCallValue(object, key)
		if utils.IsNil(object) {
			return nil, nil, false
		}
	}
	return object, write.key, true
}

func (b *singleFileBuilder) emitCSharpCallWrite(name string, call *ssa.Call, values []ssa.Value) ssa.Value {
	if b == nil || call == nil {
		return nil
	}
	// Within one basic block, later assignments unconditionally kill earlier
	// assignments to the same target. Keep one final value per block while
	// retaining writes from distinct control-flow branches for soundness.
	effective := make([]ssa.Value, 0, len(values))
	blockIndexes := make(map[int64]int)
	for _, value := range values {
		if utils.IsNil(value) || value.GetBlock() == nil {
			effective = append(effective, value)
			continue
		}
		blockID := value.GetBlock().GetId()
		if index, exists := blockIndexes[blockID]; exists {
			effective[index] = value
			continue
		}
		blockIndexes[blockID] = len(effective)
		effective = append(effective, value)
	}
	sideEffects := make([]ssa.Value, 0, len(effective))
	for _, value := range effective {
		actualValue := csharpMethodCallValue(call, value)
		if utils.IsNil(actualValue) {
			continue
		}
		if sideEffect := b.EmitSideEffect(name, call, actualValue); sideEffect != nil {
			sideEffects = append(sideEffects, sideEffect)
		}
	}
	return b.mergeConstructorValues(name, sideEffects)
}

// projectConstructorExplicitWrites fills the two gaps left by the generic
// call-side-effect model: writes through a receiver ParameterMember and C#
// static fields stored on blueprint containers. It is used for unique calls;
// ambiguous calls perform the same projection inside isolated branches.
func (b *singleFileBuilder) projectConstructorExplicitWrites(call *ssa.Call, receiver ssa.Value, function *ssa.Function) {
	if b == nil || call == nil || function == nil {
		return
	}
	receiverWrites := csharpConstructorReceiverWrites(function)
	receiverOrder := csharpReceiverWriteOrder(receiverWrites)
	for _, signature := range receiverOrder {
		write := csharpActualizeReceiverWriteForCall(call, receiverWrites[signature])
		object, key, ok := b.resolveConstructorReceiverWrite(receiver, write)
		if !ok {
			continue
		}
		merged := b.emitCSharpCallWrite("constructor.receiver."+signature, call, write.values)
		if !utils.IsNil(merged) {
			b.AssignVariable(b.CreateMemberCallVariable(object, key), merged)
		}
	}

	staticWrites := csharpFunctionStaticWrites(function)
	staticOrder := csharpStaticWriteOrder(staticWrites)
	for _, signature := range staticOrder {
		write := csharpActualizeStaticWriteForCall(call, staticWrites[signature])
		object, key, ok := b.resolveCSharpStaticWrite(write)
		if !ok {
			continue
		}
		merged := b.emitCSharpCallWrite("constructor.static."+ssa.GetKeyString(write.key), call, write.values)
		if utils.IsNil(merged) {
			continue
		}
		b.AssignVariable(b.CreateMemberCallVariable(object, key), merged)
	}
}

// emitAmbiguousConstructorCalls preserves every equally-ranked source
// constructor. The argument expressions have already been evaluated, so each
// candidate only rematerializes its own formal vector. Calls are deliberately
// over-approximated and then joined: this lets syntax-flow enter every possible
// constructor body without choosing one by declaration order.
func (b *singleFileBuilder) emitAmbiguousConstructorCalls(
	class *ssa.Blueprint,
	receiver ssa.Value,
	arguments []csharpEvaluatedArgument,
	selections []csharpConstructorSelection,
) ssa.Value {
	if b == nil || class == nil || utils.IsNil(receiver) || len(selections) < 2 {
		return nil
	}
	dispatch := b.EmitUndefined("$constructor-overload-dispatch")
	resultName := fmt.Sprintf("$constructor-overload-result-%d", dispatch.GetId())
	b.AssignVariable(b.CreateVariable(resultName), b.EmitValueOnlyDeclare(resultName))

	targets := make(map[string]*csharpMethodStateTarget)
	var targetOrder []string
	addTarget := func(write *csharpMethodStaticWrite) string {
		signature := csharpStaticWriteSignature(write)
		if signature == "" {
			return ""
		}
		if _, exists := targets[signature]; !exists {
			targets[signature] = &csharpMethodStateTarget{write: write}
			targetOrder = append(targetOrder, signature)
		}
		return signature
	}
	receiverTargets := make(map[string]*csharpReceiverStateTarget)
	addReceiverTarget := func(write *csharpConstructorReceiverWrite) {
		if write == nil {
			return
		}
		signature := csharpConstructorReceiverWriteSignature(write.path, write.key)
		if signature == "" {
			return
		}
		if _, exists := receiverTargets[signature]; !exists {
			receiverTargets[signature] = &csharpReceiverStateTarget{write: write}
		}
	}

	receiverKeys := constructorReceiverMemberKeys(selections)
	receiverKeyNames := make([]string, 0, len(receiverKeys))
	for name := range receiverKeys {
		receiverKeyNames = append(receiverKeyNames, name)
	}
	sort.Strings(receiverKeyNames)
	for _, name := range receiverKeyNames {
		addReceiverTarget(&csharpConstructorReceiverWrite{key: receiverKeys[name]})
	}

	receiverWrites := make([]map[string]*csharpConstructorReceiverWrite, len(selections))
	staticWrites := make([]map[string]*csharpMethodStaticWrite, len(selections))
	for index, selection := range selections {
		receiverWrites[index] = csharpConstructorReceiverWrites(selection.constructor.function)
		receiverWrites[index] = csharpActualizeReceiverWritesForBinding(
			receiverWrites[index], 1, selection.binding, arguments,
		)
		receiverOrder := csharpReceiverWriteOrder(receiverWrites[index])
		for _, signature := range receiverOrder {
			addReceiverTarget(receiverWrites[index][signature])
		}

		staticWrites[index] = csharpActualizeStaticWritesForBinding(
			csharpFunctionStaticWrites(selection.constructor.function), 1, selection.binding, arguments,
		)
		for _, signature := range csharpStaticWriteOrder(staticWrites[index]) {
			addTarget(staticWrites[index][signature])
		}
	}
	sort.SliceStable(targetOrder, func(left, right int) bool {
		leftWrite, rightWrite := targets[targetOrder[left]].write, targets[targetOrder[right]].write
		if len(leftWrite.path) != len(rightWrite.path) {
			return len(leftWrite.path) < len(rightWrite.path)
		}
		return targetOrder[left] < targetOrder[right]
	})
	for index, signature := range targetOrder {
		target := targets[signature]
		if object, key, ok := b.resolveCSharpStaticWrite(target.write); ok {
			target.baseline = b.ReadMemberCallValue(object, key)
		}
		if utils.IsNil(target.baseline) {
			target.baseline = b.EmitUndefined(ssa.GetKeyString(target.write.key))
		}
		target.resultName = fmt.Sprintf("%s.state.%d", resultName, index)
		b.AssignVariable(b.CreateVariable(target.resultName), b.EmitValueOnlyDeclare(target.resultName))
	}
	receiverTargetOrder := make([]string, 0, len(receiverTargets))
	for signature := range receiverTargets {
		receiverTargetOrder = append(receiverTargetOrder, signature)
	}
	sort.SliceStable(receiverTargetOrder, func(left, right int) bool {
		leftWrite := receiverTargets[receiverTargetOrder[left]].write
		rightWrite := receiverTargets[receiverTargetOrder[right]].write
		if len(leftWrite.path) != len(rightWrite.path) {
			return len(leftWrite.path) < len(rightWrite.path)
		}
		return receiverTargetOrder[left] < receiverTargetOrder[right]
	})
	for index, signature := range receiverTargetOrder {
		target := receiverTargets[signature]
		object, key, ok := b.resolveConstructorReceiverWrite(receiver, target.write)
		if ok {
			target.baseline = b.ReadMemberCallValue(object, key)
		}
		if utils.IsNil(target.baseline) {
			target.baseline = b.EmitUndefined(ssa.GetKeyString(target.write.key))
		}
		target.resultName = fmt.Sprintf("%s.receiver-state.%d", resultName, index)
		b.AssignVariable(b.CreateVariable(target.resultName), b.EmitValueOnlyDeclare(target.resultName))
	}

	emitCandidate := func(index int, selection csharpConstructorSelection) {
		candidate := selection.constructor
		args, outs := b.materializeCSharpArguments(candidate.parameters, candidate.function, 1, arguments, selection.binding)
		call := b.EmitCall(b.NewCall(candidate.function, append([]ssa.Value{receiver}, args...)))
		if call == nil {
			for _, signature := range targetOrder {
				target := targets[signature]
				b.AssignVariable(b.CreateVariable(target.resultName), target.baseline)
			}
			for _, signature := range receiverTargetOrder {
				target := receiverTargets[signature]
				b.AssignVariable(b.CreateVariable(target.resultName), target.baseline)
			}
			b.AssignVariable(b.CreateVariable(resultName), b.EmitUndefined("constructor"))
			return
		}
		b.bindOutArguments(call, outs)

		receiverOrder := csharpReceiverWriteOrder(receiverWrites[index])
		for _, signature := range receiverOrder {
			write := receiverWrites[index][signature]
			object, key, ok := b.resolveConstructorReceiverWrite(receiver, write)
			if !ok {
				continue
			}
			merged := b.emitCSharpCallWrite("ambiguous-constructor.receiver."+signature, call, write.values)
			if !utils.IsNil(merged) {
				b.AssignVariable(b.CreateMemberCallVariable(object, key), merged)
			}
		}

		for _, signature := range csharpStaticWriteOrder(staticWrites[index]) {
			write := csharpActualizeStaticWriteForCall(call, staticWrites[index][signature])
			object, key, ok := b.resolveCSharpStaticWrite(write)
			if !ok {
				continue
			}
			merged := b.emitCSharpCallWrite("ambiguous-constructor.static."+ssa.GetKeyString(write.key), call, write.values)
			if !utils.IsNil(merged) {
				b.AssignVariable(b.CreateMemberCallVariable(object, key), merged)
			}
		}

		b.projectConstructorReceiverState(call, receiver)
		for _, signature := range targetOrder {
			target := targets[signature]
			var value ssa.Value
			if object, key, ok := b.resolveCSharpStaticWrite(target.write); ok {
				value = b.ReadMemberCallValue(object, key)
			}
			if utils.IsNil(value) {
				value = target.baseline
			}
			b.AssignVariable(b.CreateVariable(target.resultName), value)
		}
		for _, signature := range receiverTargetOrder {
			target := receiverTargets[signature]
			var value ssa.Value
			if object, key, ok := b.resolveConstructorReceiverWrite(receiver, target.write); ok {
				value = b.ReadMemberCallValue(object, key)
			}
			if utils.IsNil(value) {
				value = target.baseline
			}
			b.AssignVariable(b.CreateVariable(target.resultName), value)
		}
		b.AssignVariable(b.CreateVariable(resultName), call)
	}

	branches := b.CreateIfBuilder()
	last := len(selections) - 1
	for index, selection := range selections {
		index, selection := index, selection
		body := func() { emitCandidate(index, selection) }
		if index == last {
			branches.SetElse(body)
			break
		}
		branches.AppendItem(func() ssa.Value {
			return b.EmitBinOp(ssa.OpEq, dispatch, b.EmitConstInst(index))
		}, body)
	}
	branches.Build()
	result := b.ReadValue(resultName)
	if utils.IsNil(result) {
		return nil
	}
	if receiver.GetType() != nil {
		result.SetType(receiver.GetType())
	} else {
		result.SetType(class)
	}
	for _, signature := range targetOrder {
		target := targets[signature]
		merged := b.ReadValue(target.resultName)
		if utils.IsNil(merged) {
			merged = target.baseline
		}
		if object, key, ok := b.resolveCSharpStaticWrite(target.write); ok {
			b.AssignVariable(b.CreateMemberCallVariable(object, key), merged)
		}
	}
	// Resolve logical receiver paths only after their shallower roots have been
	// merged. A candidate may replace `Cell` before writing `Cell.Value`, so an
	// object captured before the branches is not a valid target for every path.
	for _, signature := range receiverTargetOrder {
		target := receiverTargets[signature]
		merged := b.ReadValue(target.resultName)
		if utils.IsNil(merged) {
			merged = target.baseline
		}
		if object, key, ok := b.resolveConstructorReceiverWrite(receiver, target.write); ok {
			b.AssignVariable(b.CreateMemberCallVariable(object, key), merged)
		}
		if object, key, ok := b.resolveConstructorReceiverWrite(result, target.write); ok {
			b.AssignVariable(b.CreateMemberCallVariable(object, key), merged)
		}
	}
	return result
}

// emitConstructorCall uses the C# overload set when available while preserving
// the existing Blueprint fallback for implicit/default and external types.
func (b *singleFileBuilder) emitConstructorCall(class *ssa.Blueprint, receiver ssa.Value, arguments []csharpEvaluatedArgument, exclude *ssa.Function, allowFallback bool) ssa.Value {
	if b == nil || class == nil || receiver == nil {
		return nil
	}
	// Parent/interface relations are registered lazily on the blueprint. They
	// must be available before resolving an implicit default constructor.
	class.Build()
	candidates := b.constructors.candidates(class)
	selected := b.constructors.selectConstructors(class, arguments, exclude)
	var method ssa.Value
	var args []ssa.Value
	var outs []outArgument
	if len(selected) > 1 {
		return b.emitAmbiguousConstructorCalls(class, receiver, arguments, selected)
	}
	if len(selected) == 1 {
		method = selected[0].constructor.function
		args, outs = b.materializeCSharpArguments(
			selected[0].constructor.parameters,
			selected[0].constructor.function,
			1, // constructor parameter zero is the synthetic $this receiver
			arguments,
			selected[0].binding,
		)
	} else {
		args, outs = flattenEvaluatedArguments(arguments)
	}
	if len(selected) == 0 && allowFallback && len(candidates) == 0 {
		// A source class with no declared constructor has an implicit parameterless
		// constructor. It forwards to the direct base's parameterless/defaulted
		// overload on the same receiver; C# never inherits an arbitrary base ctor.
		if b.constructors.isDeclared(class) {
			if len(arguments) != 0 {
				return nil
			}
			if parent := class.GetSuperBlueprint(); parent != nil {
				return b.emitConstructorCall(parent, receiver, nil, nil, true)
			}
			return receiver
		}
		method = class.GetMagicMethod(ssa.Constructor, b.FunctionBuilder)
	}
	if method == nil || method == exclude {
		return nil
	}
	call := b.EmitCall(b.NewCall(method, append([]ssa.Value{receiver}, args...)))
	b.bindOutArguments(call, outs)
	b.projectConstructorExplicitWrites(call, receiver, csharpResolvedFunction(method))
	b.projectConstructorReceiverState(call, receiver)
	return call
}
