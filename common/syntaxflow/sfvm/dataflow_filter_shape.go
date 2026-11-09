package sfvm

import (
	"strings"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
)

// DataflowFilterShapeKind identifies dataflow include/exclude sub-rules that are
// equivalent to a membership test on one already-bound symbol. Callers can
// evaluate these shapes from an id set instead of compiling and running a
// sub-query for every enumerated path.
type DataflowFilterShapeKind uint8

const (
	DataflowFilterShapeUnknown DataflowFilterShapeKind = iota
	// DataflowFilterShapeAny is `* & $sym`: the path contains a value bound to
	// $sym. Repeated `& $sym` items are idempotent and fold into this shape.
	DataflowFilterShapeAny
	// DataflowFilterShapeSelf is `<self> & $sym`: the path center is bound to
	// $sym.
	DataflowFilterShapeSelf
	// DataflowFilterShapeCallee is `*<getCallee> & $sym`: a call on the path has
	// a callee bound to $sym.
	DataflowFilterShapeCallee
)

// DataflowFilterShape is a parsed simple dataflow filter.
type DataflowFilterShape struct {
	Kind   DataflowFilterShapeKind
	Symbol string
}

// maxDataflowFilterShapeBytes caps parse work for pathological sub-rules; the
// simple shapes are a few dozen bytes.
const maxDataflowFilterShapeBytes = 1024

// ParseDataflowFilterShape parses a dataflow include/exclude sub-rule and
// reports whether it is one of the simple shapes above.
//
// Only shapes provably equivalent to a single symbol membership test are
// accepted; everything else returns ok=false so the caller keeps the full
// sub-query authoritative. Rejected examples: intersections of different
// symbols, `?{...}` filters, `<self>` combined with `<getCallee>`, and
// multi-statement sub-rules.
//
// A trailing `as $alias` is ignored: it only names the sub-rule result and does
// not change the include/exclude boolean, which is evaluated on whether the
// result is non-empty.
func ParseDataflowFilterShape(text string) (shape DataflowFilterShape, ok bool) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > maxDataflowFilterShapeBytes {
		return shape, false
	}
	defer func() {
		if recover() != nil {
			shape, ok = DataflowFilterShape{}, false
		}
	}()

	lexer := sf.NewSyntaxFlowLexer(antlr.NewInputStream(text))
	lexer.RemoveErrorListeners()
	errCount := &dataflowFilterShapeErrorCounter{DefaultErrorListener: &antlr.DefaultErrorListener{}}
	lexer.AddErrorListener(errCount)
	parser := sf.NewSyntaxFlowParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errCount)
	flow := parser.Flow()
	if flow == nil || errCount.count > 0 {
		return shape, false
	}
	expr := singlePureFilterExpr(flow)
	if expr == nil {
		return shape, false
	}
	return classifyDataflowFilterExpr(expr)
}

type dataflowFilterShapeErrorCounter struct {
	*antlr.DefaultErrorListener
	count int
}

func (c *dataflowFilterShapeErrorCounter) SyntaxError(antlr.Recognizer, interface{}, int, int, string, antlr.RecognitionException) {
	c.count++
}

// singlePureFilterExpr returns the filter expression of a rule that consists of
// exactly one `filterExpr (as $alias)?` statement, and nil for anything else.
// Anchoring on the full statement avoids classifying a nested filter inside a
// larger sub-rule.
func singlePureFilterExpr(flow sf.IFlowContext) sf.IFilterExprContext {
	if flow == nil || flow.Statements() == nil {
		return nil
	}
	statements := flow.Statements().AllStatement()
	if len(statements) != 1 {
		return nil
	}
	stmt, ok := statements[0].(*sf.FilterContext)
	if !ok || stmt.Eos() != nil {
		return nil
	}
	pure, ok := stmt.FilterStatement().(*sf.PureFilterExprContext)
	if !ok {
		return nil
	}
	return pure.FilterExpr()
}

func classifyDataflowFilterExpr(expr sf.IFilterExprContext) (DataflowFilterShape, bool) {
	var shape DataflowFilterShape
	kind, ok := dataflowFilterSubject(expr.FilterItemFirst())
	if !ok {
		return shape, false
	}
	shape.Kind = kind
	items := expr.AllFilterItem()
	if len(items) == 0 {
		return shape, false
	}
	for _, item := range items {
		switch it := item.(type) {
		case *sf.IntersectionRefFilterContext:
			sym := intersectionRefSymbol(it)
			if sym == "" {
				return shape, false
			}
			if shape.Symbol == "" {
				shape.Symbol = sym
			} else if shape.Symbol != sym {
				return shape, false
			}
		case *sf.FirstContext:
			// `*<getCallee>` parses as the `*` subject plus a chained item.
			if shape.Kind != DataflowFilterShapeAny || shape.Symbol != "" || !isGetCalleeFilterItem(it) {
				return shape, false
			}
			shape.Kind = DataflowFilterShapeCallee
		default:
			return shape, false
		}
	}
	if shape.Symbol == "" {
		return shape, false
	}
	return shape, true
}

func dataflowFilterSubject(first sf.IFilterItemFirstContext) (DataflowFilterShapeKind, bool) {
	switch it := first.(type) {
	case *sf.NamedFilterContext:
		if it.NameFilter() != nil && it.NameFilter().Star() != nil {
			return DataflowFilterShapeAny, true
		}
	case *sf.NativeCallFilterContext:
		if isNativeCallNamed(it.NativeCall(), "self") {
			return DataflowFilterShapeSelf, true
		}
	}
	return DataflowFilterShapeUnknown, false
}

func isGetCalleeFilterItem(item *sf.FirstContext) bool {
	first, ok := item.FilterItemFirst().(*sf.NativeCallFilterContext)
	if !ok {
		return false
	}
	return isNativeCallNamed(first.NativeCall(), "getCallee")
}

func isNativeCallNamed(call sf.INativeCallContext, name string) bool {
	if call == nil || call.UseNativeCall() == nil {
		return false
	}
	return call.UseNativeCall().GetText() == name
}

func intersectionRefSymbol(item *sf.IntersectionRefFilterContext) string {
	if item == nil || item.RefVariable() == nil || item.RefVariable().Identifier() == nil {
		return ""
	}
	return item.RefVariable().Identifier().GetText()
}
