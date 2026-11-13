// Package ssairtext is a leaf codec for SSAIR text. It imports the ssa package
// to build a Program; ssa cannot import this package back without a cycle.
// Callers are the representation layers (lowir2ssa, ssa2source) and tests.
//
// The dialect mirrors the text that ssa.Function.DisAsm and the decompiler
// renderers print: a function header, a type line, named blocks, and one
// instruction per tab-indented line. It is intentionally smaller than the
// printer output so that fixtures stay readable; unsupported forms fail loudly
// instead of being skipped.
package ssairtext

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Parse builds one program from SSAIR text. An empty input is an error.
func Parse(text string) (*ssa.Program, error) {
	functions, err := splitFunctions(text)
	if err != nil {
		return nil, err
	}
	if len(functions) == 0 {
		return nil, fmt.Errorf("ssairtext: no function found")
	}
	program := ssa.NewProgram(nil, ssa.ProgramCacheMemory, ssa.Application, nil, "", 0)
	for _, source := range functions {
		if err := parseFunction(program, source); err != nil {
			return nil, err
		}
	}
	return program, nil
}

type functionText struct {
	verbose  string
	lines    []string
	externs  []string
	function string
	typeLine string
}

// splitFunctions groups text into function blocks. A function starts at a
// non-indented line that is not a comment, a type line, or an extern entry.
func splitFunctions(text string) ([]functionText, error) {
	var result []functionText
	var current *functionText
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, ";") && !strings.HasPrefix(trimmed, "; boundary:") {
			continue
		}
		if strings.HasPrefix(trimmed, "; boundary:") {
			if current == nil {
				current = &functionText{}
			}
			current.verbose = strings.TrimSpace(strings.TrimPrefix(trimmed, ";"))
			continue
		}
		if strings.HasPrefix(trimmed, "type:") {
			if current == nil {
				return nil, fmt.Errorf("ssairtext: type line before a function header")
			}
			current.typeLine = trimmed
			continue
		}
		if strings.HasPrefix(trimmed, "extern ") {
			if current == nil {
				current = &functionText{}
			}
			current.externs = append(current.externs, strings.TrimSpace(strings.TrimPrefix(trimmed, "extern ")))
			continue
		}
		if !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, " ") {
			if _, isBlock := blockHeader(trimmed); isBlock {
				if current == nil {
					return nil, fmt.Errorf("ssairtext: block %q before a function header", trimmed)
				}
				current.lines = append(current.lines, trimmed)
				continue
			}
			if current != nil && current.function != "" {
				result = append(result, *current)
				current = &functionText{}
			}
			if current == nil {
				current = &functionText{}
			}
			current.function = trimmed
			continue
		}
		if current == nil {
			return nil, fmt.Errorf("ssairtext: instruction before a function header: %q", trimmed)
		}
		current.lines = append(current.lines, strings.TrimLeft(line, "\t "))
	}
	if current != nil && current.function != "" {
		result = append(result, *current)
	}
	return result, nil
}

func parseFunction(program *ssa.Program, source functionText) error {
	info, err := parseHeader(source.function)
	if err != nil {
		return err
	}
	parameterTypes, returnType, err := parseFunctionType(source.typeLine)
	if err != nil {
		return err
	}
	function := program.NewFunction(info.name)
	if source.verbose != "" {
		function.SetVerboseName(source.verbose)
	}
	builder := function.GetOrCreateBuilder()
	entry := builder.CurrentBlock

	blocks := map[string]*ssa.BasicBlock{}
	entryName := entry.GetName()
	blocks[entryName] = entry
	blocks[strings.ReplaceAll(entryName, "-", "_")] = entry

	// Pre-create every block so forward jumps and If targets resolve.
	var lines []string
	for _, line := range source.lines {
		name, isBlock := blockHeader(line)
		if !isBlock {
			lines = append(lines, line)
			continue
		}
		if _, exists := blocks[name]; exists {
			lines = append(lines, "@block:"+name)
			continue
		}
		block := function.NewBasicBlock(name)
		block.SetName(name)
		block.SetScope(ssa.NewScope(function, program.GetProgramName()))
		blocks[name] = block
		blocks[strings.ReplaceAll(block.GetName(), "-", "_")] = block
		lines = append(lines, "@block:"+name)
	}

	for _, name := range source.externs {
		if existing, ok := program.Funcs.Get(name); ok && existing != nil {
			continue
		}
		extern := program.NewFunction(name)
		extern.SetExtern(true)
	}

	// Parameters are created on the entry block.
	resolver := newResolver(program, function, builder, blocks)
	for index, typ := range parameterTypes {
		name := fmt.Sprintf("arg%d", index)
		if index < len(info.params) && info.params[index].name != "" {
			name = info.params[index].name
		}
		parameter := ssa.NewParam(name, false, builder)
		if parameter == nil {
			continue
		}
		parameter.SetType(typ)
		parameter.FormalParameterIndex = index
		function.Params = append(function.Params, parameter.GetId())
		resolver.params[name] = parameter
		resolver.params[fmt.Sprintf("t%d", parameter.GetId())] = parameter
		if index < len(info.params) && info.params[index].id > 0 {
			resolver.params[fmt.Sprintf("t%d", info.params[index].id)] = parameter
		}
	}
	function.SetType(ssa.NewFunctionType("", parameterTypes, returnType, false))
	function.SetCurrentReturnType(returnType)
	function.ParamLength = len(parameterTypes)

	builder.CurrentBlock = entry
	for _, line := range lines {
		if strings.HasPrefix(line, "@block:") {
			if block, ok := blocks[strings.TrimPrefix(line, "@block:")]; ok {
				builder.CurrentBlock = block
			}
			continue
		}
		if err := resolver.emit(builder.CurrentBlock.GetName(), line); err != nil {
			return err
		}
	}
	function.Finish()
	return nil
}

type headerInfo struct {
	name   string
	params []paramInfo
	result string
}

type paramInfo struct {
	name string
	typ  string
	id   int64
}

func parseHeader(line string) (headerInfo, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return headerInfo{}, fmt.Errorf("ssairtext: empty function header")
	}
	info := headerInfo{name: fields[0]}
	rest := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
	for _, part := range splitParams(rest) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Accept "<i64> (1) name" and "i64 name".
		var declaredID int64
		part = strings.ReplaceAll(part, "(", " (")
		fields := strings.Fields(part)
		var typ, name string
		for _, field := range fields {
			if strings.HasPrefix(field, "(") && strings.HasSuffix(field, ")") {
				if id, err := strconv.ParseInt(strings.Trim(field, "()"), 10, 64); err == nil {
					declaredID = id
				}
				continue
			}
			if typ == "" {
				typ = field
				continue
			}
			if name == "" {
				name = field
			}
		}
		if typ == "" || name == "" {
			continue
		}
		info.params = append(info.params, paramInfo{name: name, typ: typ, id: declaredID})
	}
	return info, nil
}

func splitParams(text string) []string {
	if text == "" || text == "(void)" {
		return nil
	}
	var parts []string
	depth := 0
	start := 0
	for index, r := range text {
		switch r {
		case '(', '[', '<':
			depth++
		case ')', ']', '>':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, text[start:index])
				start = index + 1
			}
		}
	}
	parts = append(parts, text[start:])
	return parts
}

// blockHeader reports whether the line names a block.
func blockHeader(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasSuffix(strings.SplitN(line, " ", 2)[0], ":") {
		return "", false
	}
	name := strings.TrimSuffix(strings.SplitN(line, " ", 2)[0], ":")
	if name == "" {
		return "", false
	}
	for _, r := range name {
		if r == '_' || r == '-' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			continue
		}
		return "", false
	}
	return name, true
}

func parseFunctionType(text string) ([]ssa.Type, ssa.Type, error) {
	if strings.TrimSpace(text) == "" {
		return nil, ssa.CreateAnyType(), nil
	}
	line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "type:"))
	arrow := strings.Index(line, "->")
	if arrow < 0 {
		return nil, nil, fmt.Errorf("ssairtext: type line without '->': %q", text)
	}
	paramsText := strings.TrimSpace(line[:arrow])
	paramsText = strings.TrimPrefix(paramsText, "(")
	paramsText = strings.TrimSuffix(paramsText, ")")
	var params []ssa.Type
	for _, part := range strings.Split(paramsText, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		params = append(params, parseType(part))
	}
	returnType := parseType(strings.TrimSpace(line[arrow+2:]))
	return params, returnType, nil
}

func parseType(name string) ssa.Type {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "void", "null":
		return ssa.CreateNullType()
	case "any":
		return ssa.CreateAnyType()
	case "string":
		return ssa.CreateStringType()
	case "bool":
		return ssa.NewBasicType(ssa.BooleanTypeKind, "bool")
	default:
		return ssa.NewBasicType(ssa.NumberTypeKind, strings.TrimSpace(name))
	}
}

type resolver struct {
	program  *ssa.Program
	function *ssa.Function
	builder  *ssa.FunctionBuilder
	blocks   map[string]*ssa.BasicBlock
	values   map[string]ssa.Value
	params   map[string]ssa.Value
}

func newResolver(program *ssa.Program, function *ssa.Function, builder *ssa.FunctionBuilder, blocks map[string]*ssa.BasicBlock) *resolver {
	return &resolver{
		program:  program,
		function: function,
		builder:  builder,
		blocks:   blocks,
		values:   map[string]ssa.Value{},
		params:   map[string]ssa.Value{},
	}
}

func (r *resolver) emit(block string, line string) error {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "type:"):
		return nil
	case strings.HasPrefix(line, "ret"):
		return r.emitReturn(line)
	case strings.HasPrefix(line, "jump"):
		return r.emitJump(block, line)
	case strings.HasPrefix(line, "If ["):
		return r.emitIf(block, line)
	case strings.Contains(line, "= call "):
		return r.emitCall(line)
	case strings.Contains(line, " = "):
		return r.emitAssignment(line)
	default:
		return fmt.Errorf("ssairtext: unsupported line %q", line)
	}
}

func (r *resolver) emitAssignment(line string) error {
	left, right, _ := strings.Cut(line, " = ")
	name := valueName(left)
	if name == "" {
		return fmt.Errorf("ssairtext: assignment without a value name: %q", line)
	}
	declaredType := leftType(left)
	right = strings.TrimSpace(right)
	switch {
	case strings.HasPrefix(right, "type_cast["):
		end := strings.Index(right, "]")
		if end < 0 {
			return fmt.Errorf("ssairtext: bad type cast: %q", line)
		}
		typ := parseType(right[len("type_cast["):end])
		value, err := r.operand(strings.TrimSpace(right[end+1:]))
		if err != nil {
			return err
		}
		value = r.builder.EmitTypeCast(value, typ)
		setValueType(value, declaredType)
		r.set(name, value)
		return nil
	case isBinOpLine(right):
		return r.emitBinOp(name, declaredType, right)
	default:
		value, err := r.operand(right)
		if err != nil {
			return err
		}
		setValueType(value, declaredType)
		r.set(name, value)
		return nil
	}
}

func isBinOpLine(text string) bool {
	fields := strings.Fields(text)
	if len(fields) < 3 {
		return false
	}
	switch fields[2] {
	case ssa.OpAdd, ssa.OpSub, ssa.OpMul, ssa.OpDiv, ssa.OpMod,
		ssa.OpAnd, ssa.OpOr, ssa.OpXor, ssa.OpAndNot, ssa.OpShl, ssa.OpShr,
		ssa.OpEq, ssa.OpNotEq, ssa.OpLt, ssa.OpGt, ssa.OpLtEq, ssa.OpGtEq:
		return true
	default:
		return false
	}
}

func (r *resolver) emitBinOp(name, declaredType, text string) error {
	fields := strings.Fields(text)
	if len(fields) < 5 {
		return fmt.Errorf("ssairtext: incomplete binary operation: %q", text)
	}
	op := fields[2]
	left, err := r.operand(fields[0] + " " + fields[1])
	if err != nil {
		return err
	}
	right, err := r.operand(fields[3] + " " + fields[4])
	if err != nil {
		return err
	}
	value := r.builder.EmitBinOp(ssa.BinaryOpcode(op), left, right)
	setValueType(value, declaredType)
	r.set(name, value)
	return nil
}

func (r *resolver) emitCall(line string) error {
	left, right, _ := strings.Cut(line, " = call ")
	name := valueName(left)
	declaredType := leftType(left)
	if name == "" {
		return fmt.Errorf("ssairtext: call without a value name: %q", line)
	}
	rest := strings.TrimSpace(right)
	// "<() -> any> puts (<string> hello, <i64> 0x1) binding[] member[]"
	open := strings.Index(rest, " (")
	if open < 0 {
		return fmt.Errorf("ssairtext: call without arguments: %q", line)
	}
	targetText := strings.TrimSpace(rest[:open])
	argsEnd := strings.Index(rest[open:], ")")
	if argsEnd < 0 {
		return fmt.Errorf("ssairtext: call without closing parenthesis: %q", line)
	}
	argsText := rest[open+2 : open+argsEnd]
	target, err := r.callTarget(targetText)
	if err != nil {
		return err
	}
	var args []ssa.Value
	for _, part := range splitParams(argsText) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		arg, err := r.operand(part)
		if err != nil {
			return err
		}
		args = append(args, arg)
	}
	call := r.builder.NewCall(target, args)
	r.builder.EmitCall(call)
	setValueType(call, declaredType)
	r.set(name, call)
	return nil
}

func (r *resolver) callTarget(text string) (ssa.Value, error) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return nil, fmt.Errorf("ssairtext: empty call target")
	}
	name := fields[len(fields)-1]
	if value, ok := r.lookup(name); ok {
		return value, nil
	}
	if existing, ok := r.program.Funcs.Get(name); ok && existing != nil {
		return existing, nil
	}
	extern := r.program.NewFunction(name)
	extern.SetExtern(true)
	return extern, nil
}

func (r *resolver) emitReturn(line string) error {
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "ret"))
	if text == "" {
		r.builder.EmitReturn(nil)
		return nil
	}
	var values ssa.Values
	for _, part := range splitParams(text) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		value, err := r.operand(part)
		if err != nil {
			return err
		}
		values = append(values, value)
	}
	r.builder.EmitReturn(values)
	return nil
}

func (r *resolver) emitJump(block, line string) error {
	target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "jump"))
	target = strings.TrimPrefix(target, "->")
	target = strings.TrimSpace(target)
	to, ok := r.blocks[target]
	if !ok {
		return fmt.Errorf("ssairtext: jump to unknown block %q", target)
	}
	r.builder.EmitJump(to)
	return nil
}

func (r *resolver) emitIf(block, line string) error {
	conditionText, rest, ok := strings.Cut(line, "] ")
	if !ok {
		return fmt.Errorf("ssairtext: malformed If: %q", line)
	}
	conditionText = strings.TrimPrefix(conditionText, "If [")
	condition, err := r.operand(conditionText)
	if err != nil {
		return err
	}
	trueText, falseText, ok := strings.Cut(rest, ", false -> ")
	if !ok {
		return fmt.Errorf("ssairtext: malformed If targets: %q", line)
	}
	trueName := strings.TrimSpace(strings.TrimPrefix(trueText, "true ->"))
	falseName := strings.TrimSpace(falseText)
	trueBlock, ok := r.blocks[trueName]
	if !ok {
		return fmt.Errorf("ssairtext: If targets unknown block %q", trueName)
	}
	falseBlock, ok := r.blocks[falseName]
	if !ok {
		return fmt.Errorf("ssairtext: If targets unknown block %q", falseName)
	}
	ifInstruction := r.builder.EmitIf()
	ifInstruction.Cond = condition.GetId()
	ifInstruction.True = trueBlock.GetId()
	ifInstruction.False = falseBlock.GetId()
	current, _ := r.blocks[block]
	if current != nil {
		current.AddSucc(trueBlock)
		current.AddSucc(falseBlock)
	}
	return nil
}

func (r *resolver) operand(text string) (ssa.Value, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("ssairtext: empty operand")
	}
	typeName := ""
	for strings.HasPrefix(text, "<") {
		close := strings.Index(text, ">")
		if close < 0 {
			return nil, fmt.Errorf("ssairtext: unterminated type marker in %q", text)
		}
		typeName = text[1:close]
		text = strings.TrimSpace(text[close+1:])
		if text == "" {
			return nil, fmt.Errorf("ssairtext: type marker without a value")
		}
	}
	if value, ok := r.lookup(text); ok {
		return value, nil
	}
	if strings.HasPrefix(text, "\"") {
		value := r.builder.EmitConstInst(unquote(text))
		r.set(text, value)
		return value, nil
	}
	switch text {
	case "true":
		value := r.builder.EmitConstInst(true)
		r.set(text, value)
		return value, nil
	case "false":
		value := r.builder.EmitConstInst(false)
		r.set(text, value)
		return value, nil
	case "null", "nil":
		value := r.builder.EmitConstInstNil()
		r.set(text, value)
		return value, nil
	}
	if number, err := strconv.ParseInt(text, 0, 64); err == nil {
		value := r.builder.EmitConstInst(number)
		setValueType(value, typeName)
		r.set(text, value)
		return value, nil
	}
	// A bare word after a string marker is a string constant, as printed by
	// Format for constants such as x86.flags.add.
	if strings.ToLower(typeName) == "string" {
		value := r.builder.EmitConstInst(text)
		r.set(text, value)
		return value, nil
	}
	return nil, fmt.Errorf("ssairtext: unknown operand %q", text)
}

func (r *resolver) lookup(name string) (ssa.Value, bool) {
	name = strings.TrimSpace(name)
	if value, ok := r.values[name]; ok {
		return value, true
	}
	if value, ok := r.params[name]; ok {
		return value, true
	}
	return nil, false
}

func (r *resolver) set(name string, value ssa.Value) {
	if value == nil {
		return
	}
	r.values[name] = value
	if value.GetId() > 0 {
		r.values["t"+strconv.FormatInt(value.GetId(), 10)] = value
	}
}

// leftType returns the type marker on the left side of an assignment.
func leftType(left string) string {
	left = strings.TrimSpace(left)
	if !strings.HasPrefix(left, "<") {
		return ""
	}
	close := strings.Index(left, ">")
	if close < 0 {
		return ""
	}
	return left[1:close]
}

func setValueType(value ssa.Value, name string) {
	if value == nil || name == "" {
		return
	}
	value.SetType(parseType(name))
}

func valueName(left string) string {
	left = strings.TrimSpace(left)
	for strings.HasPrefix(left, "<") {
		close := strings.Index(left, ">")
		if close < 0 {
			return ""
		}
		left = strings.TrimSpace(left[close+1:])
	}
	return left
}

func unquote(text string) string {
	text = strings.TrimSpace(text)
	if unquoted, err := strconv.Unquote(text); err == nil {
		return unquoted
	}
	return strings.Trim(text, `"`)
}
