// Package ssa2source renders YakSSA as source-like text.
//
// The output is inspection-oriented pseudocode, not compilable source. It keeps
// SSA values, block labels, implicit calls and effects visible so that an
// analyst can compare the reconstructed program with the original machine code.
package ssa2source

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Render converts a YakSSA program into source-like text.
func Render(ctx context.Context, program *ssa.Program) (source string, err error) {
	if program == nil {
		return "", fmt.Errorf("ssa2source: nil program")
	}
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		// Rendering is diagnostic output. A malformed program must produce an
		// error the caller can print, not a crash of the whole process.
		source, err = "", fmt.Errorf("ssa2source: render panic: %v", recovered)
	}()
	var output strings.Builder
	var renderErr error
	program.Funcs.ForEach(func(_ string, function *ssa.Function) bool {
		if err := ctx.Err(); err != nil {
			renderErr = err
			return false
		}
		if function == nil || function.IsExtern() {
			return true
		}
		if strings.Contains(function.GetVerboseName(), "boundary: plt") {
			return true
		}
		var text strings.Builder
		renderFunction(&text, function)
		cleaned, err := cleanSource(function, text.String())
		if err != nil {
			renderErr = err
			return false
		}
		output.WriteString(renumberSource(function, cleaned))
		output.WriteByte('\n')
		return true
	})
	if renderErr != nil {
		return "", renderErr
	}
	return output.String(), nil
}

var sourceValueID = regexp.MustCompile(`\bv(\d+)\b`)

// renumberSource maps program-global IDs to the same per-function sequence as
// SSAIR text: parameters, then phis and instructions in block order.
func renumberSource(function *ssa.Function, source string) string {
	next := int64(1)
	ids := make(map[int64]int64)
	add := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := ids[id]; ok {
			return
		}
		ids[id] = next
		next++
	}
	for _, id := range function.Params {
		if value, ok := function.GetValueById(id); ok && value != nil && value.HasUsers() {
			add(id)
		}
	}
	for _, blockID := range function.Blocks {
		block, ok := function.GetBasicBlockByID(blockID)
		if !ok || block == nil {
			continue
		}
		for _, id := range block.Phis {
			add(id)
		}
		for _, id := range block.Insts {
			add(id)
		}
	}
	return sourceValueID.ReplaceAllStringFunc(source, func(match string) string {
		raw, err := strconv.ParseInt(match[1:], 10, 64)
		if err != nil {
			return match
		}
		mapped, ok := ids[raw]
		if !ok {
			mapped = next
			next++
			ids[raw] = mapped
		}
		return fmt.Sprintf("v%d", mapped)
	})
}

func renderFunction(text *strings.Builder, function *ssa.Function) {
	params := make([]string, 0, len(function.Params))
	usesFrame := false
	for _, id := range function.Params {
		value, ok := function.GetValueById(id)
		if !ok || value == nil {
			continue
		}
		if value.GetName() == "frame" {
			// The stack pointer is an ABI register, not an argument.
			usesFrame = true
			continue
		}
		if !value.HasUsers() {
			continue
		}
		params = append(params, fmt.Sprintf("%s %s", paramTypeName(value.GetType()), value.GetName()))
	}
	if boundary := function.GetVerboseName(); strings.HasPrefix(boundary, "boundary:") {
		if index := strings.Index(boundary, ";"); index >= 0 {
			boundary = boundary[:index]
		}
		fmt.Fprintf(text, "// boundary: %s\n", strings.TrimSpace(strings.TrimPrefix(boundary, "boundary:")))
	}
	signature := functionSignature(function.GetType())
	if len(params) == 0 {
		fmt.Fprintf(text, "%s %s(void) {\n", signature, function.GetName())
	} else {
		fmt.Fprintf(text, "%s %s(%s) {\n", signature, function.GetName(), strings.Join(params, ", "))
	}
	var body strings.Builder
	newStructurer(function).render(&body)
	if usesFrame || strings.Contains(body.String(), "frame") {
		text.WriteString("    void *frame; /* stack frame base */\n")
	}
	text.WriteString(body.String())
	text.WriteString("}\n")
}

func functionSignature(typ ssa.Type) string {
	if typ == nil {
		return "void"
	}
	functionType, ok := ssa.ToFunctionType(typ)
	if !ok || functionType == nil || functionType.ReturnType == nil {
		return "void"
	}
	name := cTypeName(functionType.ReturnType)
	if name == "" || name == "any" || name == "null" || name == "void" {
		return "void"
	}
	return name
}

func paramTypeName(typ ssa.Type) string {
	if typ == nil {
		return "uint64_t"
	}
	name := cTypeName(typ)
	switch name {
	case "", "any", "void", "null":
		return "uint64_t"
	default:
		return name
	}
}

var sourceAssignment = regexp.MustCompile(`^\s*v(\d+)\s*=\s*(.*);\s*$`)
var sourceDeclaration = regexp.MustCompile(`^\s*[A-Za-z_][A-Za-z0-9_ *]*\s+v(\d+);`)
var sourceValue = regexp.MustCompile(`\bv(\d+)\b`)
var frameBaseAlias = regexp.MustCompile(`^\s*v(\d+)\s*=\s*(entry_rsp|entry_rbp)\s*-\s*0x[0-9a-fA-F]+;\s*$`)

func cleanSource(function *ssa.Function, text string) (string, error) {
	lines := strings.Split(text, "\n")
	lines = inlineFrameBaseAliases(lines)
	if !strings.Contains(function.GetVerboseName(), "frame-kept") {
		lines = pruneDeadAssignments(function, lines)
	}
	lines = pruneUnusedParameters(lines)
	text = strings.Join(lines, "\n")
	definitions := make(map[int64]int)
	for index, line := range lines {
		if match := sourceAssignment.FindStringSubmatch(line); match != nil {
			id, _ := strconv.ParseInt(match[1], 10, 64)
			if _, exists := definitions[id]; !exists {
				definitions[id] = index
			}
		}
	}
	filtered := make([]string, 0, len(lines))
	for index, line := range lines {
		match := sourceAssignment.FindStringSubmatch(line)
		if match != nil {
			id, _ := strconv.ParseInt(match[1], 10, 64)
			value, _ := function.GetValueById(id)
			if value == nil || !value.HasUsers() {
				forward := false
				for _, ref := range sourceValue.FindAllStringSubmatch(match[2], -1) {
					refID, _ := strconv.ParseInt(ref[1], 10, 64)
					if defIndex, ok := definitions[refID]; ok && defIndex > index {
						forward = true
						break
					}
				}
				if forward {
					continue
				}
			}
		}
		filtered = append(filtered, line)
	}
	filteredText := strings.Join(filtered, "\n")
	lines = strings.Split(filteredText, "\n")
	// A value that is referenced before its defining block is declared up front.
	// This keeps the textual source well-formed when an irreducible/loop-heavy
	// CFG cannot be printed in strict definition order.
	definedAt := make(map[int64]int)
	for index, line := range lines {
		if match := sourceAssignment.FindStringSubmatch(line); match != nil {
			id, _ := strconv.ParseInt(match[1], 10, 64)
			if _, exists := definedAt[id]; !exists {
				definedAt[id] = index
			}
		}
	}
	forwardIDs := make(map[int64]bool)
	for index, line := range lines {
		for _, ref := range sourceValue.FindAllStringSubmatch(line, -1) {
			id, _ := strconv.ParseInt(ref[1], 10, 64)
			if defIndex, ok := definedAt[id]; ok && defIndex > index {
				forwardIDs[id] = true
			}
		}
	}
	declaredIDs := make(map[int64]bool)
	insertAt := 0
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, "{") && (strings.HasPrefix(trimmed, "function ") || strings.HasSuffix(trimmed, ") {")) {
			insertAt = index + 1
			break
		}
	}
	for round := 0; round < 4; round++ {
		definitions := make(map[int64]int)
		for index, line := range lines {
			if match := sourceAssignment.FindStringSubmatch(line); match != nil {
				id, _ := strconv.ParseInt(match[1], 10, 64)
				if _, exists := definitions[id]; !exists {
					definitions[id] = index
				}
			}
			if match := sourceDeclaration.FindStringSubmatch(line); match != nil {
				id, _ := strconv.ParseInt(match[1], 10, 64)
				if _, exists := definitions[id]; !exists {
					definitions[id] = index
				}
			}
		}
		missing := make(map[int64]bool)
		for index, line := range lines {
			for _, ref := range sourceValue.FindAllStringSubmatch(line, -1) {
				id, _ := strconv.ParseInt(ref[1], 10, 64)
				if declaredIDs[id] {
					continue
				}
				if defIndex, ok := definitions[id]; ok && defIndex > index {
					missing[id] = true
				}
			}
		}
		if len(missing) == 0 {
			break
		}
		declarations := make([]string, 0, len(missing))
		for id := range missing {
			typ := "uint64_t"
			if value, ok := function.GetValueById(id); ok && value != nil && value.GetType() != nil {
				typ = cTypeName(value.GetType())
				if typ == "any" || typ == "null" || typ == "void" || typ == "" {
					typ = "uint64_t"
				}
			}
			declarations = append(declarations, fmt.Sprintf("    %s v%d; /* forward declaration */", typ, id))
			declaredIDs[id] = true
		}
		sort.Strings(declarations)
		lines = append(lines[:insertAt], append(declarations, lines[insertAt:]...)...)
		insertAt += len(declarations)
		filteredText = strings.Join(lines, "\n")
	}
	lines = strings.Split(filteredText, "\n")
	definitions = make(map[int64]int)
	params := make(map[int64]bool)
	for _, id := range function.Params {
		params[id] = true
	}
	for index, line := range lines {
		if match := sourceAssignment.FindStringSubmatch(line); match != nil {
			id, _ := strconv.ParseInt(match[1], 10, 64)
			if _, exists := definitions[id]; !exists {
				definitions[id] = index
			}
		}
		if match := sourceDeclaration.FindStringSubmatch(line); match != nil {
			id, _ := strconv.ParseInt(match[1], 10, 64)
			if _, exists := definitions[id]; !exists {
				definitions[id] = index
			}
		}
	}
	for index, line := range lines {
		for _, ref := range sourceValue.FindAllStringSubmatch(line, -1) {
			id, _ := strconv.ParseInt(ref[1], 10, 64)
			if params[id] {
				continue
			}
			if match := sourceAssignment.FindStringSubmatch(line); match != nil {
				lhs, _ := strconv.ParseInt(match[1], 10, 64)
				if lhs == id {
					continue
				}
			}
			if declaredIDs[id] {
				continue
			}
			if defIndex, ok := definitions[id]; ok && defIndex > index {
				return "", fmt.Errorf("ssa2source: forward reference in function %s: v%d", function.GetName(), id)
			}
		}
	}
	filteredText = strings.ReplaceAll(filteredText, "entry_rsp - 0x8", "entry_rsp")
	filteredText = strings.ReplaceAll(filteredText, "entry_rbp - 0x8", "entry_rbp")
	return filteredText, nil
}

func inlineFrameBaseAliases(lines []string) []string {
	replacements := make(map[string]string)
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		match := frameBaseAlias.FindStringSubmatch(line)
		if match != nil {
			replacements["v"+match[1]] = match[2]
			continue
		}
		filtered = append(filtered, line)
	}
	if len(replacements) == 0 {
		return lines
	}
	for index, line := range filtered {
		filtered[index] = sourceValue.ReplaceAllStringFunc(line, func(token string) string {
			if to, ok := replacements[token]; ok {
				return to
			}
			return token
		})
	}
	return filtered
}

// pruneUnusedParameters removes the parameters the body never mentions. A phi that
// merges a register the function never wrote can name an entry register, and that
// register is not a parameter of the recovered function.
func pruneUnusedParameters(lines []string) []string {
	headerIndex := -1
	for index, line := range lines {
		if strings.Contains(line, ") {") {
			headerIndex = index
			break
		}
	}
	if headerIndex < 0 {
		return lines
	}
	header := lines[headerIndex]
	closing := strings.LastIndex(header, ") {")
	if closing < 0 {
		return lines
	}
	open := matchingOpenParen(header, closing)
	if open < 0 {
		return lines
	}
	params := strings.Split(header[open+1:closing], ", ")
	if len(params) == 1 && strings.TrimSpace(params[0]) == "void" {
		return lines
	}
	body := strings.Join(lines[headerIndex+1:], "\n")
	usedIdents := collectIdentifiers(body)
	kept := make([]string, 0, len(params))
	for _, param := range params {
		fields := strings.Fields(param)
		if len(fields) == 0 {
			continue
		}
		name := fields[len(fields)-1]
		if usedIdents[name] {
			kept = append(kept, param)
		} else if !isWordString(name) && regexp.MustCompile(`\b`+regexp.QuoteMeta(name)+`\b`).MatchString(body) {
			kept = append(kept, param)
		}
	}
	if len(kept) == 0 {
		kept = []string{"void"}
	}
	lines[headerIndex] = header[:open+1] + strings.Join(kept, ", ") + header[closing:]
	return lines
}

func isWordString(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func collectIdentifiers(text string) map[string]bool {
	found := make(map[string]bool)
	start := -1
	for i := 0; i <= len(text); i++ {
		word := false
		if i < len(text) {
			c := text[i]
			word = (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
		}
		if word {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			found[text[start:i]] = true
			start = -1
		}
	}
	return found
}

// matchingOpenParen returns the '(' that matches the ')' at close. A function
// name may itself contain parentheses (a Go pointer-receiver method is
// main.(*Box[int]).Get), so the parameter list is the last balanced group, not
// the first '(' in the line.
func matchingOpenParen(text string, close int) int {
	depth := 0
	for index := close; index >= 0; index-- {
		switch text[index] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func pruneDeadAssignments(function *ssa.Function, lines []string) []string {
	// A phi that names the first edge still depends on every other edge: dropping
	// them would hide the value the other path supplies.
	kept := phiEdgeValues(function)
	for {
		uses := make(map[int64]int)
		for _, line := range lines {
			match := sourceAssignment.FindStringSubmatch(line)
			lhs := int64(0)
			if match != nil {
				lhs, _ = strconv.ParseInt(match[1], 10, 64)
			}
			for _, ref := range sourceValue.FindAllStringSubmatch(line, -1) {
				id, _ := strconv.ParseInt(ref[1], 10, 64)
				if id != lhs {
					uses[id]++
				}
			}
		}
		filtered := make([]string, 0, len(lines))
		changed := false
		for _, line := range lines {
			match := sourceAssignment.FindStringSubmatch(line)
			if match != nil {
				id, _ := strconv.ParseInt(match[1], 10, 64)
				if uses[id] == 0 && kept[id] == 0 && !assignmentHasSideEffect(function, id) {
					changed = true
					continue
				}
			}
			filtered = append(filtered, line)
		}
		lines = filtered
		if !changed {
			return lines
		}
	}
}

// phiEdgeValues counts the values a phi names on its edges.
func phiEdgeValues(function *ssa.Function) map[int64]int {
	counts := make(map[int64]int)
	if function == nil {
		return counts
	}
	for _, blockID := range function.Blocks {
		block, ok := function.GetBasicBlockByID(blockID)
		if !ok || block == nil {
			continue
		}
		for _, phiID := range block.Phis {
			instruction, ok := function.GetInstructionById(phiID)
			if !ok || instruction == nil {
				continue
			}
			phi, ok := ssa.ToPhi(instruction)
			if !ok || phi == nil {
				continue
			}
			for _, value := range phi.GetValues() {
				if value == nil {
					continue
				}
				counts[value.GetId()]++
			}
		}
	}
	return counts
}

func assignmentHasSideEffect(function *ssa.Function, id int64) bool {
	value, ok := function.GetValueById(id)
	if !ok || value == nil {
		return false
	}
	switch value.(type) {
	case *ssa.Call, *ssa.Panic:
		return true
	default:
		return false
	}
}

func renderInstructionAt(text *strings.Builder, instruction ssa.Instruction, indent string) {
	switch value := instruction.(type) {
	case *ssa.ConstInst:
		// Constants are rendered at their use sites.
	case *ssa.BinOp:
		if value.Op == ssa.OpStore {
			fmt.Fprintf(text, "%s%s;\n", indent, storeExpression(value))
			break
		}
		if !valueUsed(value) {
			break
		}
		fmt.Fprintf(text, "%s%s = %s;\n", indent, assignName(value), binaryExpression(value))
	case *ssa.UnOp:
		if value.Op == ssa.OpLoad || value.Op == ssa.OpSpill {
			// A memory read is an event at a fixed point in the instruction
			// stream, so it keeps a line of its own. Inlining it at the use site
			// would move the read past a later write.
			fmt.Fprintf(text, "%s%s = %s;\n", indent, assignName(value), derefExpression(value))
			break
		}
		if !valueUsed(value) {
			break
		}
		fmt.Fprintf(text, "%s%s = %s%s;\n", indent, assignName(value), unarySymbol(value.Op), valueNameByID(value, value.X))
	case *ssa.Call:
		expression := callExpression(value)
		if isVoidType(value.GetType()) {
			fmt.Fprintf(text, "%s%s;\n", indent, expression)
		} else if valueUsed(value) {
			fmt.Fprintf(text, "%s%s = %s;\n", indent, assignName(value), expression)
		} else {
			fmt.Fprintf(text, "%s%s;\n", indent, expression)
		}
	case *ssa.TypeCast:
		// A cast is inlined at its use sites, so it needs no statement of its own.
	case *ssa.SideEffect:
		// reify models no cross-procedure side effects, so the conversion has no
		// path that emits one. A node here means an invariant broke, so the node
		// is reported and the rest of the function keeps rendering.
		log.Errorf("ssa2source: unexpected side effect %q in %s", value.GetName(), functionOf(value))
		fmt.Fprintf(text, "%s/* unexpected side effect: %s */\n", indent, sanitizeComment(value.GetName()))
	case *ssa.Undefined:
		if !valueUsed(value) {
			break
		}
		fmt.Fprintf(text, "%s%s = undefined; /* undefined source */\n", indent, assignName(value))
	case *ssa.Parameter:
		fmt.Fprintf(text, "%s%s = parameter;\n", indent, assignName(value))
	case *ssa.Panic:
		fmt.Fprintf(text, "%spanic(%s);\n", indent, valueNameByID(value, value.Info))
	case *ssa.Return:
		if values := joinValues(value.GetValues()); values != "" {
			fmt.Fprintf(text, "%sreturn %s;\n", indent, values)
		} else {
			text.WriteString(indent + "return;\n")
		}
	case *ssa.Phi, *ssa.Jump, *ssa.If:
		// Structured rendering handles phis and terminators.
	default:
		fmt.Fprintf(text, "%s%s = %s;\n", indent, assignNameIfValue(instruction), instruction.String())
	}
}

func conditionExpression(instruction ssa.Instruction, id int64) string {
	if instruction == nil || id == 0 {
		return "true"
	}
	value, ok := instruction.GetValueById(id)
	if !ok || value == nil {
		return fmt.Sprintf("v%d", id)
	}
	switch typed := value.(type) {
	case *ssa.BinOp:
		return binaryExpression(typed)
	case *ssa.UnOp:
		if typed.Op == ssa.OpNot {
			if inverted, ok := invertedComparison(typed); ok {
				return inverted
			}
		}
		return unarySymbol(typed.Op) + valueNameByID(typed, typed.X)
	case *ssa.TypeCast:
		return cCast(typed.GetType(), conditionExpression(typed, typed.Value))
	default:
		return valueName(value)
	}
}

func valueUsed(value ssa.Value) bool {
	return valueUsedSeen(value, make(map[int64]bool))
}

func valueUsedSeen(value ssa.Value, seen map[int64]bool) bool {
	if value == nil {
		return false
	}
	if holder, ok := value.(interface {
		GetAllVariables() map[string]*ssa.Variable
	}); ok && len(holder.GetAllVariables()) > 0 {
		return true
	}
	node, ok := value.(ssa.Node)
	if !ok || !node.HasUsers() {
		return false
	}
	if seen[value.GetId()] {
		return false
	}
	seen[value.GetId()] = true
	for _, user := range node.GetUsers() {
		switch typed := user.(type) {
		case *ssa.BinOp:
			if typed.Op == ssa.OpStore {
				return true
			}
			if binaryFoldsAway(typed) {
				continue
			}
			if valueUsedSeen(typed, seen) {
				return true
			}
		case *ssa.UnOp:
			if typed.Op == ssa.OpLoad || typed.Op == ssa.OpSpill {
				return true
			}
			if valueUsedSeen(typed, seen) {
				return true
			}
		case *ssa.TypeCast:
			if valueUsedSeen(typed, seen) {
				return true
			}
		case *ssa.Phi:
			if valueUsedSeen(typed, seen) {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func assignName(value ssa.Value) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprintf("v%d", value.GetId())
}

func assignNameIfValue(instruction ssa.Instruction) string {
	if value, ok := ssa.ToValue(instruction); ok && value != nil {
		return assignName(value)
	}
	return "value"
}

func valueNameByID(instruction ssa.Instruction, id int64) string {
	if instruction == nil || id == 0 {
		return "nil"
	}
	value, ok := instruction.GetValueById(id)
	if !ok || value == nil {
		return fmt.Sprintf("v%d", id)
	}
	return valueName(value)
}

func valueName(value ssa.Value) string {
	if value == nil {
		return "nil"
	}
	if constant, ok := ssa.ToConstInst(value); ok && constant != nil {
		return constText(constant)
	}
	if parameter, ok := ssa.ToParameter(value); ok && parameter != nil {
		return parameter.GetName()
	}
	if cast, ok := ssa.ToTypeCast(value); ok && cast != nil {
		return castExpression(cast)
	}
	if block, ok := ssa.ToBasicBlock(value); ok && block != nil {
		return block.GetName()
	}
	if function, ok := ssa.ToFunction(value); ok && function != nil {
		return function.GetName()
	}
	return assignName(value)
}

func constantSource(instruction ssa.Instruction, id int64) (*ssa.ConstInst, bool) {
	value, ok := instruction.GetValueById(id)
	if !ok || value == nil {
		return nil, false
	}
	constant, ok := ssa.ToConstInst(value)
	if !ok || constant == nil {
		return nil, false
	}
	return constant, true
}

func castConstantText(cast *ssa.TypeCast, constant *ssa.ConstInst) string {
	if constant == nil || constant.Const == nil {
		return "nil"
	}
	if marker, ok := constant.GetRawValue().(string); ok {
		if isBooleanType(cast.GetType()) {
			return "false /* " + sanitizeComment(marker) + " */"
		}
		return "0x0 /* " + sanitizeComment(marker) + " */"
	}
	return constTextAs(constant, cast.GetType())
}

// derefExpression renders a memory read as a C dereference of its address
// operand. The load and spill instructions carry the address themselves, so the
// expression names exactly the value the machine instruction read from.
func derefExpression(read *ssa.UnOp) string {
	if read == nil {
		return "*(void *)(unknown)"
	}
	return fmt.Sprintf("*(%s *)(%s)", cTypeName(read.GetType()), valueNameByID(read, read.X))
}

// storeExpression renders a memory write. Its operands are the address and the
// value written there, in that order.
func storeExpression(store *ssa.BinOp) string {
	if store == nil {
		return "*(void *)(unknown) = unknown"
	}
	return fmt.Sprintf("*(%s *)(%s) = %s", cTypeName(store.GetType()), valueNameByID(store, store.X), valueNameByID(store, store.Y))
}

func binaryExpression(value *ssa.BinOp) string {
	if value == nil {
		return "0x0"
	}
	left := valueNameByID(value, value.X)
	right := valueNameByID(value, value.Y)
	if sameRootValue(value, value.X, value.Y) {
		switch value.Op {
		case ssa.OpXor, ssa.OpSub:
			return "0x0"
		case ssa.OpAnd, ssa.OpOr:
			return left
		}
	}
	switch value.Op {
	case ssa.OpAdd:
		if isIntegerConstant(value, value.Y, 0) {
			return left
		}
		if isIntegerConstant(value, value.X, 0) {
			return right
		}
		if signed, ok := signedIntegerConstant(value, value.Y); ok && signed < 0 {
			return left + " - 0x" + strconv.FormatUint(uint64(-signed), 16)
		}
		if signed, ok := signedIntegerConstant(value, value.X); ok && signed < 0 {
			return right + " - 0x" + strconv.FormatUint(uint64(-signed), 16)
		}
	case ssa.OpMul:
		if isIntegerConstant(value, value.Y, 1) {
			return left
		}
		if isIntegerConstant(value, value.X, 1) {
			return right
		}
	case ssa.OpShl, ssa.OpShr:
		if isIntegerConstant(value, value.Y, 0) {
			return left
		}
	}
	symbol := binarySymbol(value.Op)
	if value.Op == ssa.OpAndNot {
		// The complement binds to the right operand only: a & ~b.
		return left + " " + symbol + " " + right
	}
	return left + " " + symbol + " " + right
}

func binaryFoldsAway(value *ssa.BinOp) bool {
	if value == nil {
		return false
	}
	if sameRootValue(value, value.X, value.Y) && (value.Op == ssa.OpXor || value.Op == ssa.OpSub) {
		return true
	}
	return false
}

func sameRootValue(instruction ssa.Instruction, left, right int64) bool {
	return rootValueID(instruction, left) == rootValueID(instruction, right)
}

func rootValueID(instruction ssa.Instruction, id int64) int64 {
	for depth := 0; depth < 16; depth++ {
		value, ok := instruction.GetValueById(id)
		if !ok || value == nil {
			return id
		}
		cast, ok := ssa.ToTypeCast(value)
		if !ok || cast == nil {
			return id
		}
		id = cast.Value
	}
	return id
}

func signedIntegerConstant(instruction ssa.Instruction, id int64) (int64, bool) {
	value, ok := instruction.GetValueById(id)
	if !ok || value == nil {
		return 0, false
	}
	for depth := 0; depth < 8; depth++ {
		if cast, ok := ssa.ToTypeCast(value); ok && cast != nil {
			next, ok := cast.GetValueById(cast.Value)
			if !ok || next == nil {
				return 0, false
			}
			value = next
			continue
		}
		break
	}
	constant, ok := ssa.ToConstInst(value)
	if !ok || constant == nil {
		return 0, false
	}
	switch raw := constant.GetRawValue().(type) {
	case int:
		return int64(raw), true
	case int64:
		return raw, true
	case uint64:
		return int64(raw), true
	case uint32:
		return int64(int32(raw)), true
	default:
		return 0, false
	}
}

func isIntegerConstant(instruction ssa.Instruction, id int64, want int64) bool {
	value, ok := instruction.GetValueById(id)
	if !ok || value == nil {
		return false
	}
	if cast, ok := ssa.ToTypeCast(value); ok && cast != nil {
		value, ok = cast.GetValueById(cast.Value)
		if !ok || value == nil {
			return false
		}
	}
	constant, ok := ssa.ToConstInst(value)
	if !ok || constant == nil {
		return false
	}
	switch raw := constant.GetRawValue().(type) {
	case int:
		return int64(raw) == want
	case int64:
		return raw == want
	case uint64:
		return raw == uint64(want)
	case uint32:
		return uint64(raw) == uint64(want)
	default:
		return false
	}
}

func castExpression(cast *ssa.TypeCast) string {
	if cast == nil {
		return "0x0"
	}
	if constant, ok := constantSource(cast, cast.Value); ok {
		return castConstantText(cast, constant)
	}
	innerValue, ok := cast.GetValueById(cast.Value)
	if ok && innerValue != nil {
		if inner, ok := ssa.ToTypeCast(innerValue); ok && inner != nil {
			if sameType(cast.GetType(), inner.GetType()) {
				return valueName(inner)
			}
			sourceValue, sourceOK := inner.GetValueById(inner.Value)
			if sourceOK && sourceValue != nil && sameType(cast.GetType(), sourceValue.GetType()) {
				return valueName(sourceValue)
			}
			if sourceOK && sourceValue != nil {
				sourceBits := typeBits(sourceValue.GetType())
				innerBits := typeBits(inner.GetType())
				outerBits := typeBits(cast.GetType())
				// Widening can only drop the intermediate cast when it preserves the
				// source's signedness: (uint64_t)(int32_t)x sign-extends, while
				// (uint64_t)x on a uint32_t does not.
				if sourceBits > 0 && innerBits >= sourceBits && outerBits >= innerBits &&
					sameIntegerSignedness(inner.GetType(), sourceValue.GetType()) {
					return cCast(cast.GetType(), valueName(sourceValue))
				}
			}
		}
	}
	return cCast(cast.GetType(), valueNameByID(cast, cast.Value))
}

func sameType(left, right ssa.Type) bool {
	if left == nil || right == nil {
		return false
	}
	return left.String() == right.String()
}

// sameIntegerSignedness reports whether two types carry the same sign. A
// non-integer type counts as matching, so the caller keeps the previous
// reduction for casts that do not convert an integer.
func sameIntegerSignedness(left, right ssa.Type) bool {
	leftSigned, leftOK := integerTypeSignedness(left)
	rightSigned, rightOK := integerTypeSignedness(right)
	if !leftOK || !rightOK {
		return true
	}
	return leftSigned == rightSigned
}

func integerTypeSignedness(typ ssa.Type) (bool, bool) {
	if typ == nil {
		return false, false
	}
	name := typ.String()
	switch {
	case strings.HasPrefix(name, "int"):
		return true, true
	case strings.HasPrefix(name, "uint"):
		return false, true
	case len(name) > 1 && name[0] == 'i' && name[1] >= '0' && name[1] <= '9':
		return false, true
	}
	return false, false
}

func typeBits(typ ssa.Type) int {
	if typ == nil {
		return 0
	}
	name := typ.String()
	index := strings.LastIndexAny(name, "0123456789")
	end := len(name)
	for index >= 0 && index < end {
		if name[index] < '0' || name[index] > '9' {
			break
		}
		index--
	}
	if index+1 >= end {
		return 0
	}
	value := 0
	for _, digit := range name[index+1:] {
		value = value*10 + int(digit-'0')
	}
	return value
}

func constText(constant *ssa.ConstInst) string {
	if constant == nil || constant.Const == nil {
		return "nil"
	}
	return constTextAs(constant, constant.GetType())
}

func constTextAs(constant *ssa.ConstInst, typ ssa.Type) string {
	if constant == nil || constant.Const == nil {
		return "nil"
	}
	raw := constant.GetRawValue()
	if isPointerType(typ) && zeroConstant(raw) {
		return "NULL"
	}
	var text string
	switch value := raw.(type) {
	case string:
		text = strconv.Quote(value)
	case []byte:
		text = fmt.Sprintf("%#v", value)
	case nil:
		text = "nil"
	case bool:
		text = strconv.FormatBool(value)
	case int64:
		text = formatInteger(value, typ)
	case uint64:
		text = formatUnsigned(value, typ)
	case int:
		text = formatInteger(int64(value), typ)
	case uint32:
		text = formatUnsigned(uint64(value), typ)
	case float32, float64:
		text = fmt.Sprintf("%v", value)
	default:
		text = fmt.Sprintf("%v", value)
	}
	if comment := constant.GetVerboseName(); strings.HasPrefix(comment, "/*") {
		text += " " + comment
	}
	return text
}

func formatInteger(value int64, typ ssa.Type) string {
	if isBooleanType(typ) {
		return strconv.FormatBool(value != 0)
	}
	return fmt.Sprintf("0x%x", uint64(value))
}

func formatUnsigned(value uint64, typ ssa.Type) string {
	if isBooleanType(typ) {
		return strconv.FormatBool(value != 0)
	}
	return fmt.Sprintf("0x%x", value)
}

func isVoidType(typ ssa.Type) bool {
	if typ == nil {
		return false
	}
	name := typ.String()
	return name == "null" || name == "void"
}

func isPointerType(typ ssa.Type) bool {
	if typ == nil {
		return false
	}
	name := typ.String()
	return strings.Contains(name, "ptr") || strings.Contains(name, "*")
}

func zeroConstant(raw any) bool {
	switch value := raw.(type) {
	case int:
		return value == 0
	case int64:
		return value == 0
	case uint64:
		return value == 0
	case uint32:
		return value == 0
	default:
		return false
	}
}

func isBooleanType(typ ssa.Type) bool {
	if typ == nil {
		return false
	}
	name := typ.String()
	return name == "i1" || name == "boolean" || name == "bool"
}

func callExpression(call *ssa.Call) string {
	if call == nil {
		return "call()"
	}
	target := valueNameByID(call, call.Method)
	args := call.GetValuesByIDs(call.Args)
	if target == "select" && len(args) == 3 {
		// A conditional move carries no function: print it as the conditional
		// expression it stands for.
		return fmt.Sprintf("(%s ? %s : %s)", valueName(args[0]), valueName(args[1]), valueName(args[2]))
	}
	return fmt.Sprintf("%s(%s)", target, joinValues(args))
}

func joinValues(values ssa.Values) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, valueName(value))
	}
	return strings.Join(parts, ", ")
}

func stringConstant(instruction ssa.Instruction, id int64) (string, bool) {
	value, ok := instruction.GetValueById(id)
	if !ok || value == nil {
		return "", false
	}
	constant, ok := ssa.ToConstInst(value)
	if !ok || constant == nil {
		return "", false
	}
	text, ok := constant.GetRawValue().(string)
	if !ok || text == "" {
		return "", false
	}
	return text, true
}

func sanitizeComment(text string) string {
	return strings.ReplaceAll(text, "*/", "* /")
}

func cCast(typ ssa.Type, expression string) string {
	name := cTypeName(typ)
	suffix := ""
	if index := strings.Index(expression, " /*"); index >= 0 {
		suffix = expression[index:]
		expression = expression[:index]
	}
	if cNeedsParens(expression) {
		return fmt.Sprintf("(%s)(%s)%s", name, expression, suffix)
	}
	return fmt.Sprintf("(%s)%s%s", name, expression, suffix)
}

func cNeedsParens(expression string) bool {
	if expression == "" {
		return false
	}
	for _, r := range expression {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == '$' {
			continue
		}
		return true
	}
	return false
}

func cTypeName(typ ssa.Type) string {
	if typ == nil {
		return "void"
	}
	switch typ.String() {
	case "null", "void":
		return "void"
	case "i1":
		return "bool"
	case "i8":
		return "uint8_t"
	case "i16":
		return "uint16_t"
	case "i32":
		return "uint32_t"
	case "i64":
		return "uint64_t"
	case "int8":
		return "int8_t"
	case "int16":
		return "int16_t"
	case "int32":
		return "int32_t"
	case "int64":
		return "int64_t"
	case "uint8":
		return "uint8_t"
	case "uint16":
		return "uint16_t"
	case "uint32":
		return "uint32_t"
	case "uint64":
		return "uint64_t"
	case "size_t":
		return "size_t"
	case "char *":
		return "char *"
	case "string":
		return "const char *"
	case "i128":
		return "unsigned __int128"
	case "ptr64", "ptr32":
		return "void *"
	case "f32":
		return "float"
	case "f64":
		return "double"
	default:
		return typ.String()
	}
}

func binarySymbol(op ssa.BinaryOpcode) string {
	switch op {
	case ssa.OpAdd:
		return "+"
	case ssa.OpSub:
		return "-"
	case ssa.OpMul:
		return "*"
	case ssa.OpDiv:
		return "/"
	case ssa.OpMod:
		return "%"
	case ssa.OpAnd:
		return "&"
	case ssa.OpAndNot:
		return "& ~"
	case ssa.OpOr:
		return "|"
	case ssa.OpXor:
		return "^"
	case ssa.OpShl:
		return "<<"
	case ssa.OpShr:
		return ">>"
	case ssa.OpEq:
		return "=="
	case ssa.OpNotEq:
		return "!="
	case ssa.OpLt:
		return "<"
	case ssa.OpLtEq:
		return "<="
	case ssa.OpGt:
		return ">"
	case ssa.OpGtEq:
		return ">="
	default:
		return string(op)
	}
}

func unarySymbol(op ssa.UnaryOpcode) string {
	switch op {
	case ssa.OpNeg:
		return "-"
	case ssa.OpNot:
		return "!"
	case ssa.OpBitwiseNot:
		return "~"
	default:
		return string(op) + " "
	}
}

// invertedComparison turns the negation of a comparison into the comparison that
// says the same thing. A loop condition is often the negated test, and
// !(i >= 3) reads worse than i < 3.
func invertedComparison(value *ssa.UnOp) (string, bool) {
	if value == nil {
		return "", false
	}
	inner, ok := value.GetValueById(value.X)
	if !ok || inner == nil {
		return "", false
	}
	comparison, ok := inner.(*ssa.BinOp)
	if !ok || comparison == nil {
		return "", false
	}
	op, ok := oppositeComparison(comparison.Op)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%s %s %s", valueNameByID(comparison, comparison.X), op, valueNameByID(comparison, comparison.Y)), true
}

// oppositeComparison returns the comparison that holds when the given one does not.
func oppositeComparison(op ssa.BinaryOpcode) (string, bool) {
	switch op {
	case ssa.OpGt:
		return "<=", true
	case ssa.OpGtEq:
		return "<", true
	case ssa.OpLt:
		return ">=", true
	case ssa.OpLtEq:
		return ">", true
	case ssa.OpEq:
		return "!=", true
	case ssa.OpNotEq:
		return "==", true
	}
	return "", false
}

// negatedCondition renders the condition of the arm that the block does not take.
// A comparison inverts into the opposite comparison, which reads better than
// wrapping it in a logical not.
func negatedCondition(instruction ssa.Instruction, id int64) string {
	if instruction != nil {
		if value, ok := instruction.GetValueById(id); ok && value != nil {
			if inverted, ok := invertValue(value); ok {
				return inverted
			}
		}
	}
	return "!(" + conditionExpression(instruction, id) + ")"
}

func invertValue(value ssa.Value) (string, bool) {
	switch typed := value.(type) {
	case *ssa.BinOp:
		op, ok := oppositeComparison(typed.Op)
		if !ok {
			return "", false
		}
		return fmt.Sprintf("%s %s %s", valueNameByID(typed, typed.X), op, valueNameByID(typed, typed.Y)), true
	case *ssa.TypeCast:
		inner, ok := typed.GetValueById(typed.Value)
		if !ok || inner == nil {
			return "", false
		}
		return invertValue(inner)
	}
	return "", false
}

// functionOf names the function an instruction belongs to, so a diagnostic can
// point at the function that produced it.
func functionOf(instruction ssa.Instruction) string {
	function := instruction.GetFunc()
	if function == nil {
		return "<unknown>"
	}
	if name := function.GetVerboseName(); name != "" {
		return name
	}
	return function.GetName()
}
