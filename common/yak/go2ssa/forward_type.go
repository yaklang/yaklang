package go2ssa

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// forwardNamedType is a use of a named type whose declaration has not been
// seen yet. It is replaced with that declaration before any function body
// runs, so later files do not freeze the field as any or as number 0.
type forwardNamedType struct {
	ssa.Type
	name string
	lib  *ssa.Program
}

func (f *forwardNamedType) String() string {
	if f == nil || f.name == "" {
		return "any"
	}
	return f.name
}

func (f *forwardNamedType) RawString() string { return f.String() }

func (f *forwardNamedType) PkgPathString() string { return f.String() }

type embeddedForward struct {
	parent *ssa.ObjectType
	name   string
	inner  *forwardNamedType
}

type forwardState struct {
	mu       sync.Mutex
	items    []*forwardNamedType
	embedded []embeddedForward
	armed    bool
}

var forwardStates sync.Map

// withForwardState locks the library's forward set. A resolver may drop the set
// once it is empty; the caller rechecks so it cannot append to a deleted one.
func withForwardState(lib *ssa.Program, fn func(*forwardState)) {
	for {
		actual, _ := forwardStates.LoadOrStore(lib, &forwardState{})
		state := actual.(*forwardState)
		state.mu.Lock()
		if current, ok := forwardStates.Load(lib); ok && current == state {
			fn(state)
			state.mu.Unlock()
			return
		}
		state.mu.Unlock()
	}
}

func dropForwardStateIfIdle(lib *ssa.Program) {
	withForwardState(lib, func(state *forwardState) {
		if len(state.items) == 0 && len(state.embedded) == 0 && !state.armed {
			forwardStates.CompareAndDelete(lib, state)
		}
	})
}

func (b *astbuilder) forwardNamedType(name string) ssa.Type {
	lib := b.GetProgram()
	if lib == nil || name == "" || !b.PreHandler() {
		return ssa.CreateAnyType()
	}
	forward := &forwardNamedType{Type: ssa.CreateAnyType(), name: name, lib: lib}
	withForwardState(lib, func(state *forwardState) {
		state.items = append(state.items, forward)
		if !state.armed {
			state.armed = true
			lib.RegisterDeferredBuild(ssa.DeferredBuildKindBinding, "go-forward-type:"+lib.Name, func() {
				resolveForwardTypes(lib)
			})
		}
	})
	return forward
}

func (b *astbuilder) noteEmbeddedForward(parent *ssa.ObjectType, name string, inner *forwardNamedType) {
	lib := b.GetProgram()
	if lib == nil || parent == nil || inner == nil || name == "" {
		return
	}
	withForwardState(lib, func(state *forwardState) {
		state.embedded = append(state.embedded, embeddedForward{parent: parent, name: name, inner: inner})
	})
}

func resolveForwardTypes(lib *ssa.Program) {
	if lib == nil {
		return
	}
	var items []*forwardNamedType
	var embedded []embeddedForward
	withForwardState(lib, func(state *forwardState) {
		items = state.items
		embedded = state.embedded
		state.items = nil
		state.embedded = nil
		state.armed = false
	})
	if len(items) == 0 && len(embedded) == 0 {
		dropForwardStateIfIdle(lib)
		return
	}
	repl := make(map[ssa.Type]ssa.Type, len(items))
	for _, item := range items {
		real, ok := lib.GetExportType(item.name)
		if !ok || utils.IsNil(real) {
			real = ssa.CreateAnyType()
		}
		repl[item] = real
	}
	walk := &forwardWalk{repl: repl, seen: make(map[ssa.Type]struct{})}
	if lib.ExportType != nil {
		lib.ExportType.ForEach(func(_ string, typ ssa.Type) bool {
			walk.rewrite(typ)
			return true
		})
	}
	if lib.Funcs != nil {
		lib.Funcs.ForEach(func(_ string, fun *ssa.Function) bool {
			if fun != nil && fun.Type != nil {
				walk.rewrite(fun.Type)
			}
			return true
		})
	}
	for _, emb := range embedded {
		if emb.parent == nil {
			continue
		}
		if obj := goSelectorObjectType(repl[emb.inner]); obj != nil {
			if emb.parent.AnonymousField == nil {
				emb.parent.AnonymousField = map[string]*ssa.ObjectType{}
			}
			emb.parent.AnonymousField[emb.name] = obj
		}
	}
	dropForwardStateIfIdle(lib)
}

type forwardWalk struct {
	repl map[ssa.Type]ssa.Type
	seen map[ssa.Type]struct{}
}

func (w *forwardWalk) rewrite(typ ssa.Type) ssa.Type {
	if utils.IsNil(typ) {
		return typ
	}
	if next, ok := w.repl[typ]; ok {
		typ = next
		if utils.IsNil(typ) {
			return typ
		}
	}
	if _, ok := w.seen[typ]; ok {
		return typ
	}
	w.seen[typ] = struct{}{}
	switch typed := typ.(type) {
	case *ssa.ObjectType:
		for i, field := range typed.FieldTypes {
			typed.FieldTypes[i] = w.rewrite(field)
		}
		typed.FieldType = w.rewrite(typed.FieldType)
		typed.KeyTyp = w.rewrite(typed.KeyTyp)
		for name, embedded := range typed.AnonymousField {
			if embedded == nil {
				continue
			}
			if got := w.rewrite(embedded); got != embedded {
				if obj, ok := got.(*ssa.ObjectType); ok {
					typed.AnonymousField[name] = obj
				}
			}
		}
	case *ssa.AliasType:
		typed.Retarget(w.rewrite(typed.GetType()))
	case *ssa.FunctionType:
		changed := false
		for i, param := range typed.Parameter {
			next := w.rewrite(param)
			if next != param {
				typed.Parameter[i] = next
				changed = true
			}
		}
		if next := w.rewrite(typed.ReturnType); next != typed.ReturnType {
			typed.ReturnType = next
			changed = true
		}
		if !utils.IsNil(typed.ObjectType) {
			if next := w.rewrite(typed.ObjectType); next != typed.ObjectType {
				typed.ReplaceBoundReceiver(next)
			}
		}
		if changed {
			typed.ForgetStringCache()
		}
	case *ssa.ChanType:
		typed.Elem = w.rewrite(typed.Elem)
	case *ssa.InterfaceType:
		for _, obj := range typed.GetStructure() {
			w.rewrite(obj)
		}
	}
	return typ
}
