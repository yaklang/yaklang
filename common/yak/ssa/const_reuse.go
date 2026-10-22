package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// Synthetic keys emitted repeatedly while resolving one source occurrence
// have the same owner, source position and immutable value. Source literals
// keep independent identities; a mutated synthetic key leaves this pool.
type placeholderConstantKey struct {
	block    int64
	editor   *memedit.MemEditor
	position variableRangePosition
	value    any
}

type placeholderConstant struct {
	value  *ConstInst
	typeID int64
	kind   TypeKind
}

func placeholderKey(b *FunctionBuilder, value any) placeholderConstantKey {
	k := placeholderConstantKey{block: b.CurrentBlock.GetId(), value: value}
	if r := b.CurrentRange; r != nil {
		k.editor = r.GetEditor()
		k.position = variableRangePosition{r.GetStartOffset(), r.GetEndOffset(), r.GetStart().GetLine(), r.GetStart().GetColumn(), r.GetEnd().GetLine(), r.GetEnd().GetColumn()}
	}
	return k
}

func reusablePlaceholder(entry placeholderConstant, key placeholderConstantKey) bool {
	c := entry.value
	if c.typId != entry.typeID {
		return false
	}
	if c.ConstType != ConstTypePlaceholder || c.Unary != 0 || c.Origin != 0 || c.IsExtern() || c.IsObject() || c.IsMember() || c.reference != 0 || len(c.pointer) != 0 {
		return false
	}
	if variables := c.getVariablesMap(); variables != nil && variables.Len() != 0 {
		return false
	}
	if typ, ok := c.GetType().(*BasicType); !ok || typ.Kind != entry.kind || typ.methods.Load() != nil || len(typ.GetFullTypeNames()) != 0 {
		return false
	}
	if r := c.GetRange(); r != nil {
		return r.GetEditor() == key.editor &&
			r.GetStartOffset() == key.position.start && r.GetEndOffset() == key.position.end &&
			r.GetStart().GetLine() == key.position.startLine && r.GetStart().GetColumn() == key.position.startColumn &&
			r.GetEnd().GetLine() == key.position.endLine && r.GetEnd().GetColumn() == key.position.endColumn
	}
	return key.editor == nil
}

func (b *FunctionBuilder) emitSharedPlaceholder(value any) *ConstInst {
	key := placeholderKey(b, value)
	if b.placeholderConstants == nil {
		b.placeholderConstants = utils.NewSafeMapWithKey[placeholderConstantKey, placeholderConstant]()
	}
	if c, exists := b.placeholderConstants.Get(key); exists && reusablePlaceholder(c, key) {
		return c.value
	}
	c := b.emitFreshConst(value, true)
	if b.CurrentRange != nil {
		c.SetRange(b.CurrentRange.Clone())
	}
	b.placeholderConstants.Set(key, placeholderConstant{c, c.typId, c.GetTypeKind()})
	return c
}
