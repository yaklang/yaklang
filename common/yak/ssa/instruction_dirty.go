package ssa

// Mutations publish the concrete instruction, rather than looking it up by ID:
// a completed unit may already have released its clean resident cache entry.
func (a *anInstruction) bindPersistenceInstruction(inst Instruction) {
	// Reuse the construction-only pending-type slot for the concrete owner.
	// Binding happens before publishing the instruction in the cache.
	if _, bound := a.persistState.(Instruction); !bound {
		a.persistState = inst
	}
}

func (a *anInstruction) markPersistenceDirty() {
	if a == nil || a.prog == nil || a.prog.Cache == nil {
		return
	}
	self, bound := a.persistState.(Instruction)
	if !bound {
		return
	}
	store := a.prog.Cache.instructions
	if store != nil && store.residentCache != nil {
		store.residentCache.Set(a.id, self)
	}
}

func bindInstructionPersistence(inst Instruction) {
	if owner, ok := inst.(interface{ bindPersistenceInstruction(Instruction) }); ok {
		owner.bindPersistenceInstruction(inst)
	}
}
