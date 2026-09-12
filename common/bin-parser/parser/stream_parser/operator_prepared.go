package stream_parser

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/rules"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

// A bounded cache of immutable instruction templates. Invocations still own
// their engine, library, root scope, callbacks and all runtime containers.
// In particular an operator cannot retain another invocation's node bindings.
var preparedOperators = utils.NewLRUCache[*preparedOperator](512)

var embeddedOperatorSources = sync.OnceValue(func() map[string]bool {
	sources := make(map[string]bool)
	var visit func(any)
	visit = func(v any) {
		switch v := v.(type) {
		case yaml.MapSlice:
			for _, item := range v {
				if item.Key == "operator" {
					if source, ok := item.Value.(string); ok {
						sources[source] = true
					}
				}
				visit(item.Value)
			}
		case []any:
			for _, item := range v {
				visit(item)
			}
		}
	}
	_ = fs.WalkDir(rules.RuleFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		data, err := rules.RuleFS.ReadFile(path)
		if err != nil {
			return nil
		}
		var document yaml.MapSlice
		if yaml.Unmarshal(data, &document) == nil {
			visit(document)
		}
		return nil
	})
	return sources
})

type preparedOperator struct {
	symbols *yakvm.SymbolTable
	codes   []*yakvm.Code
}

func loadPreparedOperator(source string) (*preparedOperator, error) {
	if len(source) > operatorProgramSourceLimit {
		return nil, nil
	}
	return preparedOperators.GetOrLoad(source, func() (prepared *preparedOperator, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("prepare operator: %v", recovered)
			}
		}()
		artifact, err := loadOperatorProgram(source)
		if err != nil {
			return nil, err
		}
		decoder := yakvm.NewCodesMarshaller()
		decoder.SetSourceCode(source)
		symbols, codes, err := decoder.Unmarshal(artifact)
		if err != nil {
			return nil, err
		}
		if !isPreparedOperator(codes) {
			return nil, nil
		}
		bindRuleProgramSource(codes, &source)
		return &preparedOperator{symbols: symbols, codes: codes}, nil
	})
}

// Reject dynamic execution, user functions (including callbacks), retained
// mutable instruction operands and context/config lookups that could inject
// a callable. This is deliberately conservative: unsupported programs use
// the existing fresh decoder and full goroutine-aware VM.
func isPreparedOperator(codes []*yakvm.Code) bool {
	// A single immediately invoked child visitor cannot publish its anonymous
	// Yak function: it has no binding, and ForEachChild invokes it synchronously
	// and discards the return. Nested functions and callback/config lookups in
	// the visitor remain excluded. OpPush copies the function's defining scope.
	if len(codes) == 6 && codes[0].Opcode == yakvm.OpPushId && codes[0].Op1.String() == "this" &&
		codes[1].Opcode == yakvm.OpPush && codes[1].Op1.String() == "ForEachChild" &&
		codes[2].Opcode == yakvm.OpMemberCall && codes[3].Opcode == yakvm.OpPush &&
		codes[4].Opcode == yakvm.OpCall && codes[4].Unary == 1 && codes[5].Opcode == yakvm.OpPop {
		if visitor, ok := codes[3].Op1.Value.(*yakvm.Function); ok {
			return isPreparedScalarOperator(visitor.GetCodes(), true)
		}
	}
	return isPreparedScalarOperator(codes, false)
}

func isPreparedScalarOperator(codes []*yakvm.Code, childVisitor bool) bool {
	for index, code := range codes {
		switch code.Opcode {
		case yakvm.OpAsyncCall, yakvm.OpInclude, yakvm.OpDefer:
			return false
		case yakvm.OpPushId:
			switch code.Op1.String() {
			case "this", "getNode", "getNodeResult", "getCurrentPosition", "panic", "debug", "len":
			default:
				return false
			}
		}
		for _, operand := range []*yakvm.Value{code.Op1, code.Op2} {
			if operand == nil {
				continue
			}
			if operand.ExtraInfo != nil || operand.CallerRef != nil || operand.CalleeRef != nil {
				return false
			}
			switch v := operand.Value.(type) {
			case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
			case string:
				// A value supplied by the caller may contain a Yak function. Do
				// not let such a function enter the synchronous private engine.
				if v == "GetCfg" || v == "GetInfo" || v == "AddInfo" && !childVisitor && !isPreparedLiteralMetadata(codes, index) {
					return false
				}
			case reflect.Type:
			default:
				return false
			}
		}
	}
	return true
}

// A discarded this.AddInfo(literalKey, literalScalar) call cannot publish a
// function, node binding or mutable instruction operand. Keep dynamic values,
// method aliases and collections on the original VM path.
func isPreparedLiteralMetadata(codes []*yakvm.Code, index int) bool {
	if index < 1 || index+5 >= len(codes) {
		return false
	}
	receiver, member, key, value, call, pop := codes[index-1], codes[index+1], codes[index+2], codes[index+3], codes[index+4], codes[index+5]
	if receiver.Opcode != yakvm.OpPushId || receiver.Op1.String() != "this" || member.Opcode != yakvm.OpMemberCall ||
		key.Opcode != yakvm.OpPush || key.Op1 == nil || value.Opcode != yakvm.OpPush || value.Op1 == nil ||
		call.Opcode != yakvm.OpCall || call.Unary != 2 || pop.Opcode != yakvm.OpPop {
		return false
	}
	if _, ok := key.Op1.Value.(string); !ok {
		return false
	}
	switch value.Op1.Value.(type) {
	case nil, bool, string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	default:
		return false
	}
}

func execPreparedOperator(node *base.Node, source string, operator func(*base.Node) (func(bool), error), modes []string) (handled bool, err error) {
	if len(modes) == 0 || modes[0] != ParserMode {
		return false, nil
	}
	// The compact method representation is private to existing embedded rule
	// programs. Custom scripts may inspect or replace public YakNode fields.
	if !embeddedOperatorSources()[source] {
		return false, nil
	}
	if node != nil && node.Ctx != nil && node.Ctx.GetBool("preparedOperatorLegacy") {
		return false, nil
	}
	p, prepareErr := loadEmbeddedPreparedOperator(source)
	if prepareErr != nil || p == nil {
		return false, nil
	}
	handled = true
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	engine := antlr4yak.New()
	engine.GetVM().GetConfig().SetSuppressPanicDebugStack(true)
	engine.GetVM().GetConfig().SetSynchronousExecution(true)
	engine.ImportLibs(preparedOperatorLibrary(node, operator))
	engine.GetVM().SetSymboltable(p.symbols)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return true, engine.GetVM().ExecYakCode(ctx, source, p.codes, yakvm.None)
}

func preparedOperatorLibrary(node *base.Node, process func(*base.Node) (func(bool), error)) map[string]any {
	var this *operatorNode
	if node != nil {
		this = convertOperatorNode(node, process)
	}
	return map[string]any{
		"this": this,
		"len":  func(v any) int { return reflect.ValueOf(v).Len() },
		"getNodeResult": func(key string) any {
			n := getNodeByPath(node, key)
			if n == nil {
				panic("node not found")
			}
			value, err := n.Result()
			if err != nil {
				panic(err)
			}
			return value
		},
		"getNode":            func(key string) any { return convertOperatorNode(getNodeByPath(node, key), process) },
		"getCurrentPosition": func() int { return len(node.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes()) },
		"debug":              log.Debugf,
	}
}
