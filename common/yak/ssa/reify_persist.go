package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
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
	p.MarkAllInstructionsDirty()
	return p.Cache.SaveToDatabase()
}

// RegisterInstructionName publishes name in the program's variable index and
// name pool without changing the instruction's verbose name. A producer that
// creates functions directly, instead of through a language builder, needs it
// so a syntaxflow name search can find those function values.
func (p *Program) RegisterInstructionName(name string, instruction Instruction) {
	if p == nil || p.Cache == nil || name == "" || utils.IsNil(instruction) {
		return
	}
	p.Cache.AddVariable(name, instruction)
}

// MarkAllInstructionsDirty republishes every live instruction of a program.
// It is the exported form of the persistence hook that instruction setters use
// after a direct field mutation.
func (p *Program) MarkAllInstructionsDirty() {
	if p == nil || p.Cache == nil || p.Funcs == nil {
		return
	}
	p.Funcs.ForEach(func(_ string, function *Function) bool {
		if function == nil {
			return true
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
