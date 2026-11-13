package ast

import (
	"reflect"
	"testing"

	"github.com/yaklang/yaklang/common/yak/typescript/frontend/core"
)

func TestConcreteNodeSharesNodeStorage(t *testing.T) {
	factory := NewNodeFactory(NodeFactoryHooks{})
	node := factory.NewIdentifier("value")
	identifier := node.AsIdentifier()
	if identifier.AsNode() != node {
		t.Fatal("AsNode must return the original node, not a copy")
	}

	// Mutations through either view must reach the same fields, including the
	// atomic ID and the back-reference used by Node's typed accessors.
	identifier.Loc = core.NewTextRange(3, 11)
	identifier.Flags = NodeFlagsSynthesized
	identifier.id.Store(42)
	if node.Pos() != 3 || node.End() != 11 || node.Flags != NodeFlagsSynthesized || node.id.Load() != 42 {
		t.Fatal("concrete node writes did not update the Node view")
	}
	parent := factory.NewIdentifier("parent")
	node.Parent = parent
	node.Loc = core.NewTextRange(5, 13)
	node.id.Store(43)
	if identifier.Parent != parent || identifier.Pos() != 5 || identifier.End() != 13 || identifier.id.Load() != 43 {
		t.Fatal("Node writes did not update the concrete node")
	}
	if node.TryAsIdentifier() != identifier || identifier.AsNode().AsIdentifier() != identifier {
		t.Fatal("typed accessors must preserve the concrete node identity")
	}
	if node.TryAsStringLiteral() != nil {
		t.Fatal("checked accessors must still reject a different node kind")
	}
}

func TestConcreteNodeDoesNotPromoteNodeAccessors(t *testing.T) {
	// The executable uses dynamic reflection. Promoting Node's full API onto
	// each concrete node makes the linker retain those wrappers even when no
	// caller uses them. Keep the API on *Node and reach it through AsNode.
	for _, value := range []any{&Token{}, &Identifier{}, &QualifiedName{}, &ForStatement{}, &SourceFile{}} {
		typ := reflect.TypeOf(value)
		for _, name := range []string{"AsIdentifier", "TryAsIdentifier", "AsSourceFile", "AsBlock", "ModifierFlags"} {
			if _, ok := typ.MethodByName(name); ok {
				t.Errorf("%v unexpectedly promotes %s", typ, name)
			}
		}
		for _, name := range []string{"AsNode", "Pos", "End"} {
			if _, ok := typ.MethodByName(name); !ok {
				t.Errorf("%v is missing %s", typ, name)
			}
		}
	}
}
