package core

import (
	"os"

	"github.com/yaklang/yaklang/common/log"
)

func init() {
	if os.Getenv("JSR_INLINE_DEBUG") != "" {
		jsrInlineDebug = true
	}
	if os.Getenv("JSR_INLINE_OFF") != "" {
		jsrInlineDisabled = true
	}
}

// jsrInlineDisabled is an emergency kill-switch (set via JSR_INLINE_OFF) that reverts to the
// pre-inliner behavior (jsr/ret methods degrade to stubs). Off by default.
var jsrInlineDisabled bool

// JSR/RET subroutine inlining.
//
// Pre-Java-6 javac compiles `try { ... } finally { ... }` using bytecode subroutines: the
// finally body is emitted once, entered with `jsr <finally>` from both the normal-completion
// path and the catch-any exception path, and left with `ret <local>` (the local holds the
// return address pushed by jsr). Java 6+ instead duplicates the finally body inline, which is
// the only form the rest of this decompiler understands. This pass rewrites the older jsr/ret
// form into the modern inlined-duplicate form so downstream CFG/structuring stays unchanged.
//
// Design constraints (this is a security tool: a wrong-but-compilable decompile is worse than a
// clearly-marked stub, and the existing safety net already stubs jsr methods today):
//   - No-op when the method contains no jsr/ret, so the ~99% of modern classes are byte-for-byte
//     unaffected.
//   - Conservative: only the canonical, well-understood javac subroutine shape is transformed.
//     Anything unusual (jsr_w/goto_w, switches, nested/multi-ret subroutines, subroutines that
//     are fallen-through-into or sit inside a try region, return-address reuse, branch targets we
//     cannot remap) makes the pass leave the bytecode untouched, so the method degrades to
//     exactly today's stub.
//   - All validation happens before any mutation, so a bail never corrupts the opcode list.

// jsrSubroutine describes one validated finally subroutine in index space of d.opCodes.
type jsrSubroutine struct {
	entryIdx int // index of the leading `astore <retAddrLocal>`
	retIdx   int // index of the trailing `ret <retAddrLocal>`
	local    int // the return-address local slot
}

// branchTarget records a branch instruction in the rewritten list whose 2-byte relative operand
// must be recomputed once final offsets are assigned. target is the destination opcode pointer,
// which must end up in the rewritten list.
type branchTarget struct {
	op     *OpCode
	target *OpCode
}

// inlineJSRSubroutines rewrites d.opCodes (and d.ExceptionTable) to remove jsr/ret when the
// method uses the canonical javac finally-subroutine pattern. It is a no-op (and never errors)
// otherwise; jsr/ret left in place are rejected downstream exactly as before.
func (d *Decompiler) inlineJSRSubroutines() {
	defer func() {
		// Any unexpected shape that slips past validation must not crash the whole decompile;
		// fall back to the untouched opcode list (-> existing stub).
		_ = recover()
	}()

	if jsrInlineDisabled {
		return
	}
	ops := d.opCodes
	hasJSR := false
	for _, op := range ops {
		switch op.Instr.OpCode {
		case OP_JSR, OP_JSR_W, OP_RET:
			hasJSR = true
		}
	}
	if !hasJSR {
		return
	}
	d.tryInlineJSR(ops)
}

func (d *Decompiler) tryInlineJSR(ops []*OpCode) {
	n := len(ops)
	bail := func(reason string) {
		if jsrInlineDebug {
			log.Infof("jsr-inline bail: %s", reason)
		}
	}

	// Bail on instruction forms whose offsets we cannot safely recompute in index space:
	//   - jsr_w/goto_w use a 4-byte operand that ScanJmp reads with the 2-byte helper anyway.
	//   - tableswitch/lookupswitch store ABSOLUTE targets in SwitchJmpCase, which re-offsetting
	//     would invalidate.
	for _, op := range ops {
		switch op.Instr.OpCode {
		case OP_JSR_W, OP_GOTO_W, OP_TABLESWITCH, OP_LOOKUPSWITCH:
			bail("contains jsr_w/goto_w/switch")
			return
		}
	}

	// origTarget resolves every 2-byte branch (if*/goto) to the opcode it jumps to, using the
	// ORIGINAL offsets (still intact: nothing has been mutated yet).
	origTarget := make(map[*OpCode]*OpCode, n)
	for _, op := range ops {
		if !isTwoByteBranch(op.Instr.OpCode) && op.Instr.OpCode != OP_JSR {
			continue
		}
		dst := op.CurrentOffset + Convert2bytesToInt(op.Data)
		idx, ok := d.offsetToOpcodeIndex[dst]
		if !ok || idx < 0 || idx >= n {
			bail("branch target offset not resolvable")
			return
		}
		origTarget[op] = ops[idx]
	}

	// Collect subroutines: every jsr target must be a canonical subroutine entry (`astore L`)
	// closed by a single `ret L`.
	subsByEntry := map[int]*jsrSubroutine{}
	jsrSub := map[int]*jsrSubroutine{} // jsr opcode index -> its subroutine
	for i, op := range ops {
		if op.Instr.OpCode != OP_JSR {
			continue
		}
		entry := origTarget[op]
		entryIdx := indexOfOpcode(ops, entry)
		if entryIdx < 0 {
			bail("jsr entry index not found")
			return
		}
		sub, ok := subsByEntry[entryIdx]
		if !ok {
			built := buildSubroutine(ops, entryIdx)
			if built == nil {
				bail("subroutine shape not canonical")
				return
			}
			subsByEntry[entryIdx] = built
			sub = built
		}
		jsrSub[i] = sub
	}
	if len(jsrSub) == 0 {
		return
	}

	// Mark every opcode index that belongs to some subroutine body (inclusive of entry & ret),
	// rejecting overlapping subroutines.
	inBody := make([]bool, n)
	for _, sub := range subsByEntry {
		for k := sub.entryIdx; k <= sub.retIdx; k++ {
			if inBody[k] {
				bail("overlapping subroutines")
				return
			}
			inBody[k] = true
		}
	}

	// The instruction before a subroutine entry must not fall through into it (the body must be
	// single-entry, reachable only via jsr). Allow OP_START or an unconditional transfer.
	for _, sub := range subsByEntry {
		prev := sub.entryIdx - 1
		if prev < 0 {
			bail("subroutine entry has no predecessor")
			return
		}
		if ops[prev].Instr.OpCode != OP_START && !isUnconditionalTransfer(ops[prev].Instr.OpCode) {
			bail("fall-through into subroutine entry")
			return
		}
	}

	// Classify every exception entry against the subroutine bodies (those original offsets are
	// deleted, so each entry must be cleanly resolvable):
	//   - fully OUTSIDE every body  -> remapped to the new offset space (the common try/finally
	//     handler, plus outer-try entries whose range merely spans an inner subroutine).
	//   - fully INSIDE one body interior -> a try/catch nested inside the finally; the finally
	//     body is duplicated per jsr call site, so this entry is cloned once per call site too.
	//   - anything straddling a body boundary (or referencing the entry astore) -> bail.
	//
	// "interior" of a subroutine is the offset range strictly between the entry astore and the
	// ret; the end PC is exclusive so it may also equal the ret offset (mapped to the tail goto).
	subContaining := func(pc uint16, allowRet bool) *jsrSubroutine {
		for _, s := range subsByEntry {
			lo := ops[s.entryIdx].CurrentOffset
			hi := ops[s.retIdx].CurrentOffset
			if pc > lo && (pc < hi || (allowRet && pc == hi)) {
				return s
			}
		}
		return nil
	}
	touchesEntry := func(pc uint16) bool {
		for _, s := range subsByEntry {
			if pc == ops[s.entryIdx].CurrentOffset {
				return true
			}
		}
		return false
	}
	outsideExc := make([]*ExceptionTableEntry, 0, len(d.ExceptionTable))
	insideExc := map[*jsrSubroutine][]*ExceptionTableEntry{}
	for _, e := range d.ExceptionTable {
		sS := subContaining(e.StartPc, false)
		sH := subContaining(e.HandlerPc, false)
		sE := subContaining(e.EndPc, true)
		if touchesEntry(e.StartPc) || touchesEntry(e.HandlerPc) {
			bail("exception references subroutine entry")
			return
		}
		if sS == nil && sH == nil && sE == nil {
			outsideExc = append(outsideExc, e)
			continue
		}
		if sS != nil && sS == sH && sS == sE {
			insideExc[sS] = append(insideExc[sS], e)
			continue
		}
		if jsrInlineDebug {
			log.Infof("jsr-inline bail: exc straddles body entry={start=%d end=%d handler=%d type=%d}",
				e.StartPc, e.EndPc, e.HandlerPc, e.CatchType)
		}
		bail("exception straddles subroutine boundary")
		return
	}

	// Build the rewritten opcode list. Survivors reuse their original pointers; subroutine bodies
	// are cloned per jsr call site. Nothing is mutated yet.
	newOps := make([]*OpCode, 0, n)
	branches := make([]branchTarget, 0, n)
	jsrFirstNew := map[uint16]*OpCode{} // original jsr offset -> first opcode emitted in its place
	// excJobs: try/catch entries nested inside a finally, to be re-emitted (one per inline site)
	// with PCs mapped through the call site's cloneMap once final offsets are assigned.
	type excCloneJob struct {
		src      *ExceptionTableEntry
		cloneMap map[*OpCode]*OpCode
	}
	var excJobs []excCloneJob

	for i := 0; i < n; i++ {
		op := ops[i]
		if op.Instr.OpCode == OP_START {
			newOps = append(newOps, op)
			continue
		}
		if inBody[i] {
			continue // emitted via cloning at each jsr site
		}
		if op.Instr.OpCode == OP_JSR {
			sub := jsrSub[i]
			if i+1 >= n || inBody[i+1] || ops[i+1].Instr.OpCode == OP_JSR {
				bail("jsr return site is not ordinary code")
				return // return site must be ordinary survivor code
			}
			returnSite := ops[i+1]
			// Clone the body interior [entry+1 .. ret-1] (drop the entry astore and the ret).
			cloneMap := make(map[*OpCode]*OpCode)
			cloneMap[ops[sub.retIdx]] = nil // placeholder; set to tail goto below
			var firstEmitted *OpCode
			for k := sub.entryIdx + 1; k <= sub.retIdx-1; k++ {
				src := ops[k]
				c := &OpCode{Instr: src.Instr, IsWide: src.IsWide}
				if len(src.Data) > 0 {
					c.Data = append([]byte(nil), src.Data...)
				}
				cloneMap[src] = c
				newOps = append(newOps, c)
				if firstEmitted == nil {
					firstEmitted = c
				}
			}
			// The tail goto replaces `ret`: jumping to the original ret == returning to caller.
			tail := &OpCode{Instr: InstrInfos[OP_GOTO], Data: []byte{0, 0}}
			cloneMap[ops[sub.retIdx]] = tail
			newOps = append(newOps, tail)
			if firstEmitted == nil {
				firstEmitted = tail
			}
			jsrFirstNew[op.CurrentOffset] = firstEmitted

			// Record branch fixups for cloned body branches: internal targets map through
			// cloneMap; exits to outside code keep the original (survivor) pointer.
			for k := sub.entryIdx + 1; k <= sub.retIdx-1; k++ {
				src := ops[k]
				if !isTwoByteBranch(src.Instr.OpCode) {
					continue
				}
				tOld := origTarget[src]
				if c, ok := cloneMap[tOld]; ok && c != nil {
					branches = append(branches, branchTarget{op: cloneMap[src], target: c})
				} else {
					branches = append(branches, branchTarget{op: cloneMap[src], target: tOld})
				}
			}
			branches = append(branches, branchTarget{op: tail, target: returnSite})
			// Clone any try/catch entries nested inside this finally body for this call site.
			for _, e := range insideExc[sub] {
				excJobs = append(excJobs, excCloneJob{src: e, cloneMap: cloneMap})
			}
			continue
		}
		// Ordinary survivor.
		newOps = append(newOps, op)
		if isTwoByteBranch(op.Instr.OpCode) {
			branches = append(branches, branchTarget{op: op, target: origTarget[op]})
		}
	}

	// A surviving branch may target a jsr instruction directly (e.g. a loop back-edge whose head is
	// the jsr). The jsr op is deleted, so redirect such targets to the first opcode emitted in its
	// place (semantically: jump to "execute the finally, then continue"). jsrFirstNew is fully
	// populated now that the build loop processed every jsr.
	for i := range branches {
		if t := branches[i].target; t != nil && t.Instr != nil && t.Instr.OpCode == OP_JSR {
			if first, ok := jsrFirstNew[t.CurrentOffset]; ok {
				branches[i].target = first
			}
		}
	}

	// Every branch target must resolve to an opcode that actually exists in the rewritten list
	// (survivors reuse their pointer, clones are new pointers). Anything else (a target that fell
	// inside a deleted body, or a survivor branch into a body) means the shape is not canonical.
	inNew := make(map[*OpCode]struct{}, len(newOps))
	for _, op := range newOps {
		inNew[op] = struct{}{}
	}
	for _, b := range branches {
		if b.target == nil {
			bail("unresolved branch target (nil)")
			return
		}
		if _, ok := inNew[b.target]; !ok {
			if jsrInlineDebug {
				log.Infof("jsr-inline bail: branch target not in list, target op=0x%x off=%d", b.target.Instr.OpCode, b.target.CurrentOffset)
			}
			bail("branch target not in rewritten list")
			return
		}
	}

	// Verify the rewritten method still fits in the 16-bit offset space.
	total := 0
	for _, op := range newOps {
		if op.Instr.OpCode == OP_START {
			continue
		}
		total += opByteLen(op)
	}
	if total >= 1<<16 {
		bail("rewritten method exceeds 16-bit offset space")
		return
	}

	// Pre-compute the new code length and the original code length for exception EndPc remapping.
	origCodeLen := 0
	for _, op := range ops {
		if op.Instr.OpCode == OP_START {
			continue
		}
		if end := int(op.CurrentOffset) + opByteLen(op); end > origCodeLen {
			origCodeLen = end
		}
	}

	// Build old-offset -> surviving-opcode before we overwrite CurrentOffset, so the exception
	// remap can resolve PCs that pointed at survivors or at removed jsr instructions.
	oldOffToSurvivor := map[uint16]*OpCode{}
	for oldOff, idx := range d.offsetToOpcodeIndex {
		if idx < 0 || idx >= n {
			continue
		}
		if _, ok := inNew[ops[idx]]; ok {
			oldOffToSurvivor[oldOff] = ops[idx]
		}
	}

	// Commit (cannot fail): offsets, ids, maps, then branch operands.
	offsetToIndex := make(map[uint16]int, len(newOps))
	indexToOffset := make(map[int]uint16, len(newOps))
	offset := 0
	id := 1
	for k, op := range newOps {
		if op.Instr.OpCode == OP_START {
			op.Id = 0
			continue
		}
		op.Id = id
		id++
		op.CurrentOffset = uint16(offset)
		offsetToIndex[uint16(offset)] = k
		indexToOffset[k] = uint16(offset)
		offset += opByteLen(op)
	}
	codeLenNew := uint16(offset)

	for _, b := range branches {
		delta := b.target.CurrentOffset - b.op.CurrentOffset
		if len(b.op.Data) != 2 {
			b.op.Data = make([]byte, 2)
		}
		b.op.Data[0] = byte(delta >> 8)
		b.op.Data[1] = byte(delta)
	}

	// Remap exception table PCs to the new offset space.
	mapPc := func(pc uint16, isEnd bool) (uint16, bool) {
		if s, ok := oldOffToSurvivor[pc]; ok {
			return s.CurrentOffset, true
		}
		if first, ok := jsrFirstNew[pc]; ok {
			return first.CurrentOffset, true
		}
		if isEnd && int(pc) == origCodeLen {
			return codeLenNew, true
		}
		return 0, false
	}
	newExc := make([]*ExceptionTableEntry, 0, len(d.ExceptionTable))
	for _, e := range outsideExc {
		s, ok1 := mapPc(e.StartPc, false)
		en, ok2 := mapPc(e.EndPc, true)
		h, ok3 := mapPc(e.HandlerPc, false)
		if !ok1 || !ok2 || !ok3 {
			bail("exception PC unmappable")
			return // unmappable PC: abandon the rewrite (d.opCodes not yet reassigned)
		}
		newExc = append(newExc, &ExceptionTableEntry{StartPc: s, EndPc: en, HandlerPc: h, CatchType: e.CatchType})
	}
	// Re-emit each try/catch nested inside a finally, once per inline site, mapping its PCs
	// through that site's cloneMap. d.offsetToOpcodeIndex is still the ORIGINAL map here.
	cloneOff := func(pc uint16, cm map[*OpCode]*OpCode) (uint16, bool) {
		idx, ok := d.offsetToOpcodeIndex[pc]
		if !ok || idx < 0 || idx >= n {
			return 0, false
		}
		c, ok := cm[ops[idx]]
		if !ok || c == nil {
			return 0, false
		}
		return c.CurrentOffset, true
	}
	for _, job := range excJobs {
		s, ok1 := cloneOff(job.src.StartPc, job.cloneMap)
		en, ok2 := cloneOff(job.src.EndPc, job.cloneMap)
		h, ok3 := cloneOff(job.src.HandlerPc, job.cloneMap)
		if !ok1 || !ok2 || !ok3 {
			bail("nested exception PC unmappable")
			return
		}
		newExc = append(newExc, &ExceptionTableEntry{StartPc: s, EndPc: en, HandlerPc: h, CatchType: job.src.CatchType})
	}

	d.opCodes = newOps
	d.ExceptionTable = newExc
	d.offsetToOpcodeIndex = offsetToIndex
	d.opcodeIndexToOffset = indexToOffset
	d.CurrentId = id
	if len(newOps) > 0 {
		d.RootOpCode = newOps[0]
	}
	if jsrInlineDebug {
		log.Infof("jsr-inline committed: %d subroutines, %d->%d opcodes", len(subsByEntry), n, len(newOps))
	}
}

// jsrInlineDebug, when set via the JSR_INLINE_DEBUG env at init, logs every committed inline so
// tests can identify which methods exercised the pass.
var jsrInlineDebug = false

// buildSubroutine validates that the opcode at entryIdx begins a canonical javac finally
// subroutine and returns its extent, or nil if the shape is not handled.
func buildSubroutine(ops []*OpCode, entryIdx int) *jsrSubroutine {
	if entryIdx < 0 || entryIdx >= len(ops) {
		return nil
	}
	entry := ops[entryIdx]
	if !isAstore(entry.Instr.OpCode) {
		return nil
	}
	local := opLocalIndex(entry)
	if local < 0 {
		return nil
	}
	// Find the closing ret: the first ret at/after the entry using the same local. Reject any
	// nested jsr, a second ret, or any reuse of the return-address local inside the body
	// (javac never loads the return address as a value; reuse means this is not a plain finally).
	for k := entryIdx + 1; k < len(ops); k++ {
		op := ops[k]
		switch op.Instr.OpCode {
		case OP_JSR, OP_JSR_W:
			return nil
		case OP_RET:
			if opLocalIndex(op) != local {
				return nil
			}
			return &jsrSubroutine{entryIdx: entryIdx, retIdx: k, local: local}
		}
		if GetRetrieveIdx(op) == local || GetStoreIdx(op) == local {
			return nil
		}
	}
	return nil
}

func opByteLen(op *OpCode) int {
	n := 1
	if op.IsWide {
		n++
	}
	n += len(op.Data)
	return n
}

func indexOfOpcode(ops []*OpCode, target *OpCode) int {
	if target == nil {
		return -1
	}
	for i, op := range ops {
		if op == target {
			return i
		}
	}
	return -1
}

func pcInRange(pc, lo, hi uint16) bool {
	return pc >= lo && pc <= hi
}

func isAstore(op int) bool {
	switch op {
	case OP_ASTORE, OP_ASTORE_0, OP_ASTORE_1, OP_ASTORE_2, OP_ASTORE_3:
		return true
	}
	return false
}

func opLocalIndex(op *OpCode) int {
	if op.IsWide {
		if len(op.Data) < 2 {
			return -1
		}
		return int(Convert2bytesToInt(op.Data))
	}
	switch op.Instr.OpCode {
	case OP_ASTORE, OP_RET:
		if len(op.Data) < 1 {
			return -1
		}
		return int(op.Data[0])
	case OP_ASTORE_0:
		return 0
	case OP_ASTORE_1:
		return 1
	case OP_ASTORE_2:
		return 2
	case OP_ASTORE_3:
		return 3
	}
	return -1
}

func isTwoByteBranch(op int) bool {
	switch op {
	case OP_IFEQ, OP_IFNE, OP_IFLT, OP_IFGE, OP_IFGT, OP_IFLE,
		OP_IF_ICMPEQ, OP_IF_ICMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPLE,
		OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IFNULL, OP_IFNONNULL, OP_GOTO:
		return true
	}
	return false
}

func isUnconditionalTransfer(op int) bool {
	switch op {
	case OP_GOTO, OP_GOTO_W, OP_RET,
		OP_RETURN, OP_IRETURN, OP_ARETURN, OP_LRETURN, OP_DRETURN, OP_FRETURN, OP_ATHROW:
		return true
	}
	return false
}
