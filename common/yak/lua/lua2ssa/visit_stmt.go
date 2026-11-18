package lua2ssa

import (
	luaparser "github.com/yaklang/yaklang/common/yak/antlr4Lua/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) VisitChunk(raw luaparser.IChunkContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	chunk, ok := raw.(*luaparser.ChunkContext)
	if !ok || chunk == nil {
		return
	}
	b.VisitBlock(chunk.Block())
}

func (b *singleFileBuilder) VisitBlock(raw luaparser.IBlockContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	block, ok := raw.(*luaparser.BlockContext)
	if !ok || block == nil {
		return
	}
	for _, stat := range block.AllStat() {
		b.VisitStat(stat)
		if b.IsStop() {
			return
		}
	}
	if last := block.Laststat(); last != nil {
		b.VisitLaststat(last)
	}
}

func (b *singleFileBuilder) VisitStat(raw luaparser.IStatContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	stat, ok := raw.(*luaparser.StatContext)
	if !ok || stat == nil {
		return
	}
	recoverRange := b.SetRange(stat)
	defer recoverRange()

	switch {
	case stat.SemiColon() != nil:
		return
	case stat.Varlist() != nil && stat.AssignEq() != nil:
		b.visitAssignment(stat)
	case stat.Functioncall() != nil:
		b.VisitFunctioncall(stat.Functioncall())
	case stat.Do() != nil:
		b.VisitBlock(stat.Block(0))
	case stat.While() != nil:
		b.visitWhile(stat)
	case stat.Repeat() != nil:
		b.visitRepeat(stat)
	case stat.If() != nil:
		b.visitIf(stat)
	case stat.For() != nil:
		b.visitFor(stat)
	case stat.Local() != nil && stat.Function() != nil:
		b.visitFunctionStat(stat, true)
	case stat.Function() != nil:
		b.visitFunctionStat(stat, false)
	case stat.Local() != nil:
		b.visitLocal(stat)
	case stat.Break() != nil:
		b.Break()
	case stat.Goto() != nil:
		// goto and labels do not have a first-class SSA form in this frontend.
		return
	case stat.Label() != nil:
		return
	}
}

func (b *singleFileBuilder) VisitLaststat(raw luaparser.ILaststatContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	last, ok := raw.(*luaparser.LaststatContext)
	if !ok || last == nil {
		return
	}
	recoverRange := b.SetRange(last)
	defer recoverRange()

	switch {
	case last.Return() != nil:
		b.EmitReturn(b.visitExpList(last.Explist()))
	case last.Break() != nil:
		b.Break()
	case last.Continue() != nil:
		b.Continue()
	}
}

func (b *singleFileBuilder) visitAssignment(stat *luaparser.StatContext) {
	varlist := stat.Varlist()
	if varlist == nil {
		return
	}
	values := b.visitExpList(stat.Explist())
	for index, target := range varlist.AllVar_() {
		var value ssa.Value
		if index < len(values) {
			value = values[index]
		}
		if value == nil {
			value = b.EmitConstInstNil()
		}
		b.assignAccess(b.evalVar(target), value)
	}
}

func (b *singleFileBuilder) visitLocal(stat *luaparser.StatContext) {
	names := stat.Attnamelist()
	if names == nil {
		return
	}
	values := b.visitExpList(stat.Explist())
	for index, name := range names.AllNAME() {
		var value ssa.Value
		if index < len(values) {
			value = values[index]
		}
		if value == nil {
			value = b.EmitConstInstNil()
		}
		b.AssignVariable(b.CreateLocalVariable(name.GetText()), value)
	}
}

func (b *singleFileBuilder) visitFunctionStat(stat *luaparser.StatContext, local bool) {
	if stat.Funcbody() == nil {
		return
	}
	name := "anonymous"
	if fn := stat.Funcname(); fn != nil && len(fn.AllNAME()) > 0 {
		name = fn.AllNAME()[0].GetText()
	} else if local {
		if stat.NAME() != nil {
			name = stat.NAME().GetText()
		} else if names := stat.Attnamelist(); names != nil && len(names.AllNAME()) > 0 {
			name = names.AllNAME()[0].GetText()
		}
	}
	isMethod := stat.Funcname() != nil && stat.Funcname().Colon() != nil
	fn := b.buildFunction(name, stat.Funcbody(), isMethod)

	if local {
		if stat.NAME() != nil {
			b.AssignVariable(b.CreateLocalVariable(stat.NAME().GetText()), fn)
			return
		}
		names := stat.Attnamelist()
		if names != nil && len(names.AllNAME()) > 0 {
			for _, n := range names.AllNAME() {
				b.AssignVariable(b.CreateLocalVariable(n.GetText()), fn)
			}
			return
		}
	}
	if stat.Funcname() != nil {
		b.bindFunctionName(stat.Funcname(), fn)
	}
}

func (b *singleFileBuilder) bindFunctionName(raw luaparser.IFuncnameContext, fn *ssa.Function) {
	nameCtx, ok := raw.(*luaparser.FuncnameContext)
	if !ok || nameCtx == nil || len(nameCtx.AllNAME()) == 0 {
		return
	}
	names := make([]string, 0, len(nameCtx.AllNAME()))
	for _, n := range nameCtx.AllNAME() {
		names = append(names, n.GetText())
	}
	if len(names) == 1 {
		variable := b.CreateVariable(names[0])
		b.AssignVariable(variable, fn)
		b.GetProgram().SetExportValue(names[0], fn)
		return
	}

	base := b.ReadValue(names[0])
	if base == nil {
		base = b.EmitUndefined(names[0])
	}
	for _, part := range names[1 : len(names)-1] {
		base = b.ReadMemberCallValue(base, b.EmitConstInst(part))
	}
	key := b.EmitConstInst(names[len(names)-1])
	b.AssignVariable(b.CreateMemberCallVariable(base, key), fn)
	b.GetProgram().SetExportValue(joinNames(names), fn)
}

func joinNames(names []string) string {
	result := ""
	for index, name := range names {
		if index > 0 {
			result += "."
		}
		result += name
	}
	return result
}

func (b *singleFileBuilder) visitWhile(stat *luaparser.StatContext) {
	loop := b.CreateLoopBuilder()
	if stat.Exp(0) != nil {
		loop.SetCondition(func() ssa.Value {
			return b.VisitExp(stat.Exp(0))
		})
	}
	loop.SetBody(func() {
		b.VisitBlock(stat.Block(0))
	})
	loop.Finish()
}

func (b *singleFileBuilder) visitRepeat(stat *luaparser.StatContext) {
	loop := b.CreateLoopBuilder()
	loop.SetCondition(func() ssa.Value {
		return b.EmitConstInst(true)
	})
	loop.SetBody(func() {
		b.VisitBlock(stat.Block(0))
		if stat.Exp(0) == nil {
			return
		}
		ifBuilder := b.CreateIfBuilder()
		ifBuilder.SetCondition(func() ssa.Value {
			return b.VisitExp(stat.Exp(0))
		}, func() {
			b.Break()
		})
		ifBuilder.Build()
	})
	loop.Finish()
}

func (b *singleFileBuilder) visitIf(stat *luaparser.StatContext) {
	if stat.Exp(0) == nil {
		b.VisitBlock(stat.Block(0))
		return
	}
	ifBuilder := b.CreateIfBuilder()
	ifBuilder.SetCondition(func() ssa.Value {
		return b.VisitExp(stat.Exp(0))
	}, func() {
		b.VisitBlock(stat.Block(0))
	})

	elseIfs := stat.AllElseIf()
	for index := range elseIfs {
		blockIndex := index + 1
		condIndex := index + 1
		ifBuilder.SetCondition(func() ssa.Value {
			return b.VisitExp(stat.Exp(condIndex))
		}, func() {
			b.VisitBlock(stat.Block(blockIndex))
		})
	}

	if stat.Else() != nil {
		elseBlockIndex := len(elseIfs) + 1
		ifBuilder.SetElse(func() {
			b.VisitBlock(stat.Block(elseBlockIndex))
		})
	}
	ifBuilder.Build()
}

func (b *singleFileBuilder) visitFor(stat *luaparser.StatContext) {
	loop := b.CreateLoopBuilder()
	if stat.Namelist() != nil {
		names := stat.Namelist().AllNAME()
		loop.SetFirst(func() []ssa.Value {
			for _, name := range names {
				b.AssignVariable(b.CreateLocalVariable(name.GetText()), b.EmitConstInstNil())
			}
			return nil
		})
		loop.SetCondition(func() ssa.Value {
			return b.EmitConstInst(true)
		})
		loop.SetBody(func() {
			b.VisitBlock(stat.Block(0))
		})
		loop.Finish()
		return
	}

	name := ""
	if stat.NAME() != nil {
		name = stat.NAME().GetText()
	}
	start := b.VisitExp(stat.Exp(0))
	limit := b.VisitExp(stat.Exp(1))
	var step ssa.Value = b.EmitConstInst(int64(1))
	if stat.Exp(2) != nil {
		step = b.VisitExp(stat.Exp(2))
	}
	loop.SetFirst(func() []ssa.Value {
		b.AssignVariable(b.CreateLocalVariable(name), start)
		return nil
	})
	loop.SetCondition(func() ssa.Value {
		current := b.ReadValue(name)
		if current == nil {
			current = b.EmitConstInstNil()
		}
		return b.EmitBinOp(ssa.OpLtEq, current, limit)
	})
	loop.SetBody(func() {
		b.VisitBlock(stat.Block(0))
	})
	loop.SetThird(func() []ssa.Value {
		current := b.ReadValue(name)
		if current == nil {
			current = b.EmitConstInstNil()
		}
		b.AssignVariable(b.CreateVariable(name), b.EmitBinOp(ssa.OpAdd, current, step))
		return nil
	})
	loop.Finish()
}
