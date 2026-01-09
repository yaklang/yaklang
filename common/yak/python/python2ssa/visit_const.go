package python2ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// AssignConst assigns a constant value to a name.
// Returns true if the assignment was successful, false if the constant was already defined.
func (b *singleFileBuilder) AssignConst(name string, value ssa.Value) bool {
	if _, ok := b.constMap[name]; ok {
		// Constant already defined, warn but don't fail
		return false
	}
	b.constMap[name] = value
	return true
}

// ReadConst reads a constant value by name.
// Returns the value and true if found, nil and false otherwise.
func (b *singleFileBuilder) ReadConst(name string) (ssa.Value, bool) {
	v, ok := b.constMap[name]
	return v, ok
}

