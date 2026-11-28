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
	p.FlushInstruction()
	return p.Cache.SaveToDatabase()
}

// FlushInstruction publishes the live program state to the instruction cache.
//
// It registers every recovered function name in the variable index and marks
// every instruction of its blocks dirty. A producer that creates functions
// without a language builder leaves no index entry, so a syntaxflow name search
// cannot find them. An in-memory program calls this before a query; a persisted
// program gets the same work from SaveToDatabase before the final cache flush.
func (p *Program) FlushInstruction() {
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

// FlushFunctions publishes one batch of newly converted/transformed functions
// to the instruction cache before their compile unit is flushed and evicted.
func (p *Program) FlushFunctions(functions []*Function) {
	if p == nil || p.Cache == nil {
		return
	}
	for _, function := range functions {
		if function == nil {
			continue
		}
		if name := function.GetName(); name != "" {
			p.Cache.AddVariable(name, function)
		}
		p.Cache.MarkInstructionDirty(function)
		for _, paramID := range function.Params {
			if param, ok := function.GetInstructionById(paramID); ok && param != nil {
				p.Cache.MarkInstructionDirty(param)
			}
		}
		for _, blockID := range function.Blocks {
			block, ok := function.GetBasicBlockByID(blockID)
			if !ok || block == nil {
				continue
			}
			p.Cache.MarkInstructionDirty(block)
			for _, id := range block.Phis {
				if inst, ok := function.GetInstructionById(id); ok && inst != nil {
					p.Cache.MarkInstructionDirty(inst)
				}
			}
			for _, id := range block.Insts {
				if inst, ok := function.GetInstructionById(id); ok && inst != nil {
					p.Cache.MarkInstructionDirty(inst)
				}
			}
		}
	}
}

// SaveFlushedToDatabase persists a DB-backed program whose finished functions
// were already published and flushed per compile unit via FlushFunctions and
// FlushCompileUnit. Only extern stubs still need final variable/instruction
// registration before updating program metadata and closing the caches.
func (p *Program) SaveFlushedToDatabase() error {
	if p == nil || p.Cache == nil {
		return fmt.Errorf("ssa: nil program cache")
	}
	if !p.Cache.HaveDatabaseBackend() {
		return fmt.Errorf("ssa: program %q uses an in-memory cache", p.Name)
	}
	if p.DatabaseKind != ProgramCacheDBWrite {
		return fmt.Errorf("ssa: program %q is not on the database write cache", p.Name)
	}
	if p.Funcs != nil {
		p.Funcs.ForEach(func(_ string, function *Function) bool {
			if function == nil || !function.IsExtern() {
				return true
			}
			if name := function.GetName(); name != "" {
				p.Cache.AddVariable(name, function)
			}
			p.Cache.MarkInstructionDirty(function)
			return true
		})
	}
	p.UpdateToDatabase()()
	return p.Cache.SaveToDatabase()
}
