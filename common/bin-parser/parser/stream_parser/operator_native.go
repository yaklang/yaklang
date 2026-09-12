package stream_parser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// Register complete immutable bridge programs, not textual rewrites or a
// purity inference for arbitrary Yak. Any different source uses the VM. The
// successful program has exactly one native call and no observable locals.
type nativeBridgeProgram struct {
	parse  func(*base.Node, func(*base.Node) (func(bool), error)) error
	decode func([]byte, *fieldArena) ([]tlsCertificateField, map[string]any, error)
}

var nativeBridgePrograms = func() map[string]nativeBridgeProgram {
	programs := make(map[string]nativeBridgeProgram)
	for _, family := range []struct {
		name     string
		profiles []string
		call     func(*base.Node, func(*base.Node) (func(bool), error), string) error
		decode   func([]byte, string, *fieldArena) ([]tlsCertificateField, map[string]any, error)
	}{
		{"parseMemcachedFields", []string{"stats-request", "stats-response", "binary-get-request"}, parseMemcachedFields, decodeMemcachedFieldsWithArena},
		{"parseCassandraFields", []string{"options4", "supported4", "startup4", "options5-initial", "supported5-initial", "startup5-initial", "internode-initiate-modern"}, parseCassandraFields, decodeCassandraFieldsWithArena},
	} {
		for _, p := range family.profiles {
			profile, call, decode := p, family.call, family.decode
			source := fmt.Sprintf("err = %s(%q)\nif err != nil { panic(err) }\n", family.name, profile)
			programs[source] = nativeBridgeProgram{
				parse: func(node *base.Node, process func(*base.Node) (func(bool), error)) error {
					return call(node, process, profile)
				},
				decode: func(wire []byte, arena *fieldArena) ([]tlsCertificateField, map[string]any, error) {
					return decode(wire, profile, arena)
				},
			}
		}
	}
	return programs
}()

type bridgeCallResult struct {
	err        error
	panicValue any
	panicked   bool
}

func (r *bridgeCallResult) deliver() error {
	if r.panicked {
		panic(r.panicValue)
	}
	return r.err
}

func execRegisteredNativeBridge(node *base.Node, source string, process func(*base.Node) (func(bool), error), modes []string) (bool, error) {
	if len(modes) == 0 || modes[0] != ParserMode {
		return false, nil
	}
	call, ok := nativeBridgePrograms[source]
	if !ok {
		return false, nil
	}
	if node != nil && node.Ctx != nil && node.Ctx.GetBool("nativeBridgeLegacy") {
		return false, nil
	}
	result := bridgeCallResult{}
	func() {
		completed := false
		defer func() {
			if !completed {
				result.panicked = true
				result.panicValue = recover()
			}
		}()
		result.err = call.parse(node, process)
		completed = true
	}()
	if result.err == nil && !result.panicked {
		return true, nil
	}
	// Preserve the original Yak source, source locations, panic conversion and
	// returned diagnostic. The VM binding replays the completed call's outcome;
	// the parser and its transaction callbacks execute exactly once.
	return true, execBridgeOperatorResult(node, source, process, modes, &result)
}
