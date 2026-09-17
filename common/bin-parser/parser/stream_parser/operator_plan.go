package stream_parser

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

// These are closed, declarative operations, not a second Yak interpreter.
// Compilation recognizes the COMPLETE operator before running any operation.
// Plans own only immutable source, field names and lookup tables; nodes,
// transactions, results and callbacks always belong to the invocation.
type operatorPlan struct {
	kind        string
	source      string
	codes       []*yakvm.Code
	run         func(*planExecution) bool
	locations   sync.Map // memoized diagnostics only; never packet values
	diagnostics sync.Map
	ports       *portDispatchPlan
}

var operatorPlans = utils.NewLRUCache[*operatorPlan](512)

type embeddedPlanEntry struct {
	once         sync.Once
	plan         *operatorPlan
	err          error
	preparedOnce sync.Once
	prepared     *preparedOperator
	preparedErr  error
}

// The embedded source set is finite and immutable. Pin both admitted plans
// and negative decisions for this build; a mixed corpus must not evict and
// re-lex unsupported rules on every pass through a bounded general LRU.
var embeddedPlans sync.Map

func embeddedOperatorEntry(source string) *embeddedPlanEntry {
	entry, exists := embeddedPlans.Load(source)
	if !exists {
		entry, _ = embeddedPlans.LoadOrStore(source, &embeddedPlanEntry{})
	}
	return entry.(*embeddedPlanEntry)
}

func loadEmbeddedOperatorPlan(source string) (*operatorPlan, error) {
	p := embeddedOperatorEntry(source)
	p.once.Do(func() { p.plan, p.err = loadOperatorPlan(source) })
	return p.plan, p.err
}

func loadEmbeddedPreparedOperator(source string) (*preparedOperator, error) {
	p := embeddedOperatorEntry(source)
	p.preparedOnce.Do(func() { p.prepared, p.preparedErr = loadPreparedOperator(source) })
	return p.prepared, p.preparedErr
}

type planExecution struct {
	plan       *operatorPlan
	this       *operatorNode
	location   string
	occurrence int
}

func (e *planExecution) at(expression string) { e.atNth(expression, 0) }
func (e *planExecution) atNth(expression string, occurrence int) {
	e.location, e.occurrence = expression, occurrence
}

type planLocation struct {
	expression string
	occurrence int
}
type planDiagnosticSite struct{ code *yakvm.Code }

func (e *planExecution) diagnosticSite() *yakvm.Code {
	key := planLocation{e.location, e.occurrence}
	if cached, ok := e.plan.locations.Load(key); ok {
		return cached.(planDiagnosticSite).code
	}
	var site *yakvm.Code
	wanted := planTokens(e.location)
	lines := strings.Split(e.plan.source, "\n")
	occurrence := 0
	for _, code := range e.plan.codes {
		if code.Opcode != yakvm.OpCall && code.Opcode != yakvm.OpPanic || code.StartLineNumber < 1 || code.EndLineNumber > len(lines) {
			continue
		}
		line := []rune(lines[code.EndLineNumber-1])
		if code.EndColumnNumber >= len(line) {
			continue
		}
		text := strings.Join(lines[:code.EndLineNumber-1], "\n") + "\n" + string(line[:code.EndColumnNumber+1])
		if strings.HasSuffix(planTokens(text), wanted) {
			if occurrence == e.occurrence {
				site = code
				break
			}
			occurrence++
		}
	}
	e.plan.locations.Store(key, planDiagnosticSite{site})
	return site
}

type planDiagnosticTemplate struct {
	once   sync.Once
	prefix string
}

// The source frame is static. Obtain its exact Yak formatting once with an
// empty panic value, then attach invocation-owned error text. Even rejecting
// a candidate needs no VM after warmup. A pre-existing Yak panic may carry
// additional frames, so that uncommon case still uses the original formatter.
func (e *planExecution) diagnostic(value any) error {
	switch value.(type) {
	case *yakvm.VMPanic, *yakvm.VMPanicSignal:
		return e.vmDiagnostic(value)
	}
	key := planLocation{e.location, e.occurrence}
	entry, exists := e.plan.diagnostics.Load(key)
	if !exists {
		entry, _ = e.plan.diagnostics.LoadOrStore(key, &planDiagnosticTemplate{})
	}
	template := entry.(*planDiagnosticTemplate)
	template.once.Do(func() {
		if err := e.vmDiagnostic(""); err != nil && strings.HasSuffix(err.Error(), "YakVM Panic: ") {
			template.prefix = err.Error()
		}
	})
	if template.prefix == "" {
		return e.vmDiagnostic(value)
	}
	if err, ok := value.(error); ok {
		value = err.Error()
	}
	return fmt.Errorf("%s%v", template.prefix, value)
}

// Only a completed call's panic is delivered. No parser callback or
// transaction is ever executed a second time to construct a diagnostic.
func (e *planExecution) vmDiagnostic(value any) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	site := e.diagnosticSite()
	if site == nil {
		return fmt.Errorf("operator plan diagnostic location %q: %v", e.location, value)
	}
	call := *site
	call.Opcode = yakvm.OpCall
	call.Unary = 0
	engine := antlr4yak.New()
	engine.GetVM().GetConfig().SetSuppressPanicDebugStack(true)
	engine.GetVM().GetConfig().SetSynchronousExecution(true)
	engine.ImportLibs(map[string]any{"planFailure": func() { panic(value) }})
	return engine.GetVM().ExecYakCode(context.Background(), e.plan.source, []*yakvm.Code{
		{Opcode: yakvm.OpPushId, Op1: yakvm.NewStringValue("planFailure")}, &call,
	}, yakvm.None)
}

// Use the language lexer: comments/whitespace cannot confuse braces, quoted
// protocol names or a string containing text resembling a call. The compiler
// separately checks syntax before admitting a plan to the cache.
func planTokens(source string) string {
	return strings.Join(planLex(source), " ")
}

func planLex(source string) []string {
	lexer := yak.NewYaklangLexer(antlr.NewInputStream(source))
	lexer.RemoveErrorListeners()
	var parts []string
	for {
		token := lexer.NextToken()
		if token.GetTokenType() == antlr.TokenEOF {
			break
		}
		if token.GetText() == ";" {
			continue
		}
		switch token.GetTokenType() {
		case yak.YaklangLexerWS, yak.YaklangLexerCOMMENT, yak.YaklangLexerLINE_COMMENT, yak.YaklangLexerLF, yak.YaklangLexerEOS:
			continue
		}
		parts = append(parts, token.GetText())
	}
	return parts
}

func loadOperatorPlan(source string) (*operatorPlan, error) {
	if len(source) > operatorProgramSourceLimit {
		return nil, nil
	}
	return operatorPlans.GetOrLoad(source, func() (plan *operatorPlan, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("compile operator plan: %v", recovered)
			}
		}()
		tokens := planTokens(source)
		plan = compileSequencePlan(tokens)
		if plan == nil {
			plan = compilePortDispatchPlan(tokens)
		}
		if plan == nil {
			plan = compileByteLoopPlan(tokens)
		}
		if plan == nil {
			plan = compileFieldDispatchPlan(tokens)
		}
		if plan == nil {
			plan = compileBoundedDispatchPlan(tokens)
		}
		if plan == nil {
			plan = compileOptionsLoopPlan(tokens)
		}
		if plan == nil {
			plan = compileNativeCallPlan(tokens)
		}
		if plan == nil {
			return nil, nil
		}
		// Plans do not execute serialized programs. In particular a large port
		// table must not be rejected by the VM artifact cache's byte limit.
		// Keep source locations from one ordinary syntax/semantic compilation.
		compiler := yakast.NewYakCompiler()
		compiler.Compiler(source)
		if errs := compiler.GetErrors(); len(errs) > 0 {
			return nil, errs
		}
		codes := compiler.GetOpcodes()
		bindRuleProgramSource(codes, &source)
		plan.source, plan.codes = source, codes
		return plan, nil
	})
}

func execOperatorPlan(node *base.Node, source string, process func(*base.Node) (func(bool), error), modes []string) (handled bool, err error) {
	if len(modes) == 0 || modes[0] != ParserMode || node == nil || !embeddedOperatorSources()[source] {
		return false, nil
	}
	if node.Ctx.GetBool("operatorPlanLegacy") || node.Ctx.GetBool("preparedOperatorLegacy") {
		return false, nil
	}
	plan, err := loadEmbeddedOperatorPlan(source)
	if err != nil || plan == nil {
		return false, nil
	}
	if plan.kind == "native-call" && node.Ctx.GetBool("nativeBridgeLegacy") {
		return false, nil
	}
	e := &planExecution{plan: plan, this: convertOperatorNode(node, process)}
	handled = true
	defer func() {
		if recovered := recover(); recovered != nil {
			err = e.diagnostic(recovered)
		}
	}()
	return plan.run(e), nil
}

type sequenceStep struct {
	method, name, expression string
	occurrence               int
}

func compileSequencePlan(tokens string) *operatorPlan {
	var steps []sequenceStep
	occurrences := make(map[string]int)
	rest := tokens
	for strings.HasPrefix(rest, "this . ") {
		method, next, ok := strings.Cut(strings.TrimPrefix(rest, "this . "), " ( ")
		if !ok || method != "ProcessSubNode" && method != "ProcessByType" {
			break
		}
		quoted, next, ok := strings.Cut(next, " )")
		name, err := strconv.Unquote(quoted)
		if !ok || err != nil {
			return nil
		}
		expression := "this." + method + "(" + quoted + ")"
		steps = append(steps, sequenceStep{method, name, expression, occurrences[expression]})
		occurrences[expression]++
		rest = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(next), ";"))
	}
	if len(steps) == 0 {
		return nil
	}
	tail := rest == planTokens(frameTrailerPlanSource)
	if rest != "" && !tail {
		return nil
	}
	return &operatorPlan{kind: "sequence", run: func(e *planExecution) bool {
		for _, step := range steps {
			e.atNth(step.expression, step.occurrence)
			if step.method == "ProcessSubNode" {
				e.this.processSubNodeDiscard(step.name)
			} else {
				e.this.processByTypeDiscard(step.name)
			}
		}
		if tail {
			e.at("this.HasMaxLength()")
			if !e.this.HasMaxLength() {
				return true
			}
			e.at("this.Length()")
			length := e.this.Length()
			e.at("this.GetMaxLength()")
			if length >= e.this.GetMaxLength() {
				return true
			}
			e.at(`this.NewUnknownNode("Frame Trailer")`)
			n := e.this.NewUnknownNode("Frame Trailer")
			e.at("this.GetMaxLength()")
			maximum := e.this.GetMaxLength()
			e.at("this.Length()")
			length = e.this.Length()
			e.at("tail.SetMaxLength(this.GetMaxLength()-this.Length())")
			n.SetMaxLength(maximum - length)
			e.at("tail.Process()")
			n.processDiscard()
		}
		return true
	}}
}

const frameTrailerPlanSource = `if this.HasMaxLength() && this.Length() < this.GetMaxLength() {
 tail = this.NewUnknownNode("Frame Trailer")
 tail.SetMaxLength(this.GetMaxLength()-this.Length())
 tail.Process()
}`

const byteLoopPlanSource = `for {
 res, op = this.TryProcessByType("PeekByte")
 if !op.OK { op.Recovery(); break }
 op.Recovery()
 ele = this.NewElement()
 ele.Process()
}`

func compileByteLoopPlan(tokens string) *operatorPlan {
	// Semicolons and newlines are equivalent here, but semicolons remain tokens
	// elsewhere (notably inside for clauses).
	canonical := func(s string) string { return strings.ReplaceAll(planTokens(s), " ;", "") }
	if strings.ReplaceAll(tokens, " ;", "") != canonical(byteLoopPlanSource) {
		return nil
	}
	return &operatorPlan{kind: "byte-loop", run: func(e *planExecution) bool {
		for {
			e.at(`this.TryProcessByType("PeekByte")`)
			op := e.this.tryProcessByTypeDiscard("PeekByte")
			if op.ok {
				e.atNth("op.Recovery()", 1)
			} else {
				e.at("op.Recovery()")
			}
			op.recovery()
			if !op.ok {
				break
			}
			e.at("this.NewElement()")
			ele := e.this.NewElement()
			e.at("ele.Process()")
			ele.processDiscard()
		}
		return true
	}}
}
