package profile

import "github.com/yaklang/yaklang/common/yak/ssa"

// FuncInfo describes one function in the current compilation unit.
type FuncInfo struct {
	Name       string
	SSAID      int64
	BlockCount int
	InstCount  int
	IsExtern   bool
	IsEntry    bool
}

// Inventory is the full set of functions available for profile-driven
// obfuscation selection.
type Inventory struct {
	Funcs  []FuncInfo
	byName map[string]*FuncInfo
}

// Lookup returns the FuncInfo for the given function name, or nil.
func (inv *Inventory) Lookup(name string) *FuncInfo {
	if inv == nil {
		return nil
	}
	return inv.byName[name]
}

// NewInventory constructs an Inventory from a pre-built slice.
func NewInventory(funcs []FuncInfo) *Inventory {
	inv := &Inventory{
		Funcs:  funcs,
		byName: make(map[string]*FuncInfo, len(funcs)),
	}
	for i := range inv.Funcs {
		inv.byName[inv.Funcs[i].Name] = &inv.Funcs[i]
	}
	return inv
}

// BuildInventoryFromSSA enumerates functions in the SSA program and returns an
// inventory for profile selection.
func BuildInventoryFromSSA(program *ssa.Program, entryName string) *Inventory {
	if program == nil {
		return NewInventory(nil)
	}
	var funcs []FuncInfo
	program.EachFunction(func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		info := FuncInfo{
			Name:     fn.GetName(),
			SSAID:    fn.GetId(),
			IsExtern: fn.IsExtern(),
			IsEntry:  fn.GetName() == entryName,
		}
		if !info.IsExtern {
			info.BlockCount = len(fn.Blocks)
			for _, blockID := range fn.Blocks {
				block, ok := fn.GetBasicBlockByID(blockID)
				if ok && block != nil {
					info.InstCount += len(block.Insts)
				}
			}
		}
		funcs = append(funcs, info)
	})
	return NewInventory(funcs)
}
