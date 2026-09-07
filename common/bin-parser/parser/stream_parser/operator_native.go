package stream_parser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// Register complete immutable bridge programs, not textual rewrites or a
// purity inference for arbitrary Yak. Any different source uses the VM. The
// successful program has exactly one native call and no observable locals.
var nativeBridgePrograms = func() map[string]func(*base.Node, func(*base.Node) (func(bool), error)) error {
	programs := make(map[string]func(*base.Node, func(*base.Node) (func(bool), error)) error)
	for _, family := range []struct {
		name     string
		profiles []string
		call     func(*base.Node, func(*base.Node) (func(bool), error), string) error
	}{
		{"parseMemcachedFields", []string{"stats-request", "stats-response", "binary-get-request"}, parseMemcachedFields},
		{"parseCassandraFields", []string{"options4", "supported4", "startup4", "options5-initial", "supported5-initial", "startup5-initial", "internode-initiate-modern"}, parseCassandraFields},
	} {
		for _, p := range family.profiles {
			profile, call := p, family.call
			source := fmt.Sprintf("err = %s(%q)\nif err != nil { panic(err) }\n", family.name, profile)
			programs[source] = func(node *base.Node, process func(*base.Node) (func(bool), error)) error {
				return call(node, process, profile)
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
		result.err = call(node, process)
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
