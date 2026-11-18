package ssa

import (
	"fmt"
)

// SaveToDatabase persists a program that was built on a DB-backed cache.
//
// Source compilers persist through the compile pipeline. A producer that
// builds SSA from another representation, such as the reify binary lifter,
// needs the same final sequence without a source file: flush the pending
// instruction mutations, write the program metadata, then close the cache
// stores so every instruction reaches ssadb.
//
// The program must be created with ProgramCacheDBWrite. After this call the
// cache is closed; open the program again with ProgramCacheDBRead (for
// example ssaapi.FromDatabase) to query it.
func (p *Program) SaveToDatabase() error {
	if p == nil || p.Cache == nil {
		return fmt.Errorf("ssa: nil program cache")
	}
	if !p.Cache.HaveDatabaseBackend() {
		return fmt.Errorf("ssa: program %q uses an in-memory cache", p.Name)
	}
	if p.DatabaseKind != ProgramCacheDBWrite {
		return fmt.Errorf("ssa: program %q is not on the database write cache", p.Name)
	}
	// The binary lifter finishes every recovered function during conversion.
	// Calling Finish here would run the source-compiler lazy build, which looks
	// for a language-level entry function that a lifted binary does not have.
	// A producer that needs deferred builds must run them before this call.
	//
	// Program metadata is a separate table (ir_programs) from instruction rows
	// (ir_codes). Write it before the instruction stores flush.
	p.UpdateToDatabase()()
	// SSA passes mutate exported instruction fields directly (for example
	// call.Args). Republish the live instructions so the final flush writes the
	// post-pass state instead of the construction-time snapshot.
	p.markAllInstructionsDirty()
	return p.Cache.SaveToDatabase()
}

// markAllInstructionsDirty republishes every live instruction and registers
// the recovered function names before the final flush.
//
// A producer that creates functions without a language builder leaves no entry
// in the variable index, so a syntaxflow name search cannot find them. The
// index is only useful when the program is persisted, which is why the
// registration lives on this save path instead of in the converter.
func (p *Program) markAllInstructionsDirty() {
	if p == nil || p.Cache == nil || p.Funcs == nil {
		return
	}
	p.Funcs.ForEach(func(_ string, function *Function) bool {
		if function == nil {
			return true
		}
		if name := function.GetName(); name != "" {
			p.Cache.AddVariable(name, function)
		}
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			if !ok || block == nil {
				continue
			}
			ids := make([]int64, 0, len(block.Phis)+len(block.Insts))
			ids = append(ids, block.Phis...)
			ids = append(ids, block.Insts...)
			for _, id := range ids {
				instruction, ok := function.GetInstructionById(id)
				if !ok || instruction == nil {
					continue
				}
				p.Cache.MarkInstructionDirty(instruction)
			}
		}
		return true
	})
}
