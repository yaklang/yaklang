package ssa

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
)

func (f *Function) SetReg(i Instruction) {
	if _, ok := f.instReg[i]; !ok {
		reg := fmt.Sprintf("t%d", len(f.instReg))
		f.instReg[i] = reg
	}
}

func GetReg(I Instruction, f *Function) string {
	return f.instReg[I]
}

func GetTypeStr(n Node) string {
	return fmt.Sprintf(
		"<%s> ", n.GetType(),
	)
}

func (p *Position) String() string {
	return fmt.Sprintf(
		"%3d:%-3d - %3d:%-3d: %s",
		p.StartLine, p.StartColumn,
		p.EndLine, p.EndColumn,
		p.SourceCode,
	)
}

func getStr(v Node) string {
	op := ""
	op += GetTypeStr(v)
	switch v := v.(type) {
	case Instruction:
		if i, ok := v.(*Interface); ok {
			if i == i.Func.symbol {
				return i.Func.Name + "-symbol"
			}
		}
		if i, ok := v.(*Field); ok {
			if i.OutCapture {
				return i.Key.String() + "-capture"
			}
		}
		if v.GetParent() != nil {
			op += GetReg(v, v.GetParent())
		}
	case *Const:
		op += v.String()
	case *Parameter:
		op += v.String()
	case *Function:
		op += v.Name
	default:
		panic("instruction unknow value type: " + v.String())
	}
	return op
}

// function

type FunctionAsmFlag int

const (
	DisAsmDefault FunctionAsmFlag = 1 << iota
	DisAsmWithoutSource
)

// implement value
func (f *Function) String() string {
	return f.DisAsm(DisAsmDefault)
}
func (f *Function) DisAsm(flag FunctionAsmFlag) string {
	ret := f.Name + " "
	ret += strings.Join(
		lo.Map(f.Param, func(item *Parameter, _ int) string { return GetTypeStr(item) + item.variable }),
		", ")
	ret += "\n"

	if parent := f.parent; parent != nil {
		ret += fmt.Sprintf("parent: %s\n", parent.Name)
	}

	if f.Pos != nil {
		ret += fmt.Sprintf("pos: %s\n", f.Pos)
	}

	if len(f.FreeValues) > 0 {
		ret += "freeValue: " + strings.Join(
			lo.Map(f.FreeValues, func(key Value, _ int) string {
				return getStr(key)
			}),
			// f.FreeValue,
			", ") + "\n"
	}
	if len(f.Return) > 0 {
		ret += "return: " + strings.Join(
			lo.Map(f.Return, func(ret *Return, _ int) string {
				return getStr(ret)
			}),
			", ") + "\n"
	}

	for _, b := range f.Blocks {
		ret += b.String() + "\n"

		if flag&DisAsmWithoutSource == 0 {
			for _, p := range b.Phis {
				ret += fmt.Sprintf("\t%s\n", p)
			}
			for _, i := range b.Instrs {
				ret += fmt.Sprintf("\t%s\n", i)
			}
		} else {
			insts := make([]string, 0, len(b.Instrs)+len(b.Phis))
			pos := make([]string, 0, len(b.Instrs)+len(b.Phis))
			for _, p := range b.Phis {
				insts = append(insts, fmt.Sprintf("\t%s", p))
				pos = append(pos, p.Pos())
			}
			for _, i := range b.Instrs {
				insts = append(insts, fmt.Sprintf("\t%s", i))
				pos = append(pos, i.Pos())
			}
			// get maxlen
			max := 0
			for _, s := range insts {
				if len(s) > max {
					max = len(s)
				}
			}
			format := fmt.Sprintf("\t%%-%ds\t\t%%s\n", max)
			for i := range insts {
				ret += fmt.Sprintf(format, insts[i], pos[i])
			}
		}
	}
	return ret
}

// ----------- basic block
func (b *BasicBlock) String() string {
	ret := b.Name + ":"
	if len(b.Preds) != 0 {
		ret += " <- "
		for _, pred := range b.Preds {
			ret += pred.Name + " "
		}
	}
	if b.Condition != nil {
		ret += " (" + b.Condition.String() + ")"
	}
	return ret
}

// ----------- const
func (c Const) String() string {
	return c.str
}

// ----------- Phi
func (p *Phi) String() string {
	ret := fmt.Sprintf("%s = phi ", getStr(p))
	for i := range p.Edge {
		v := p.Edge[i]
		b := p.Block.Preds[i]
		if v == nil {
			continue
		}
		ret += fmt.Sprintf("[%s, %s] ", getStr(v), b.Name)
	}
	return ret
}

// ----------- Parameter
func (p *Parameter) String() string {
	return p.variable
}

// ----------- Jump
func (j *Jump) String() string {
	return fmt.Sprintf("jump -> %s", j.To.Name)
}

// ----------- IF
func (i *If) String() string {
	// return i.StringByFunc(DefaultValueString)
	return fmt.Sprintf("If [%s] true -> %s, false -> %s", getStr(i.Cond), i.True.Name, i.False.Name)
}

// ----------- Return
func (r *Return) String() string {
	return fmt.Sprintf(
		"ret %s",
		strings.Join(
			lo.Map(r.Results, func(v Value, _ int) string { return getStr(v) }),
			", ",
		),
	)
}

// ----------- Call
func (c *Call) String() string {
	return fmt.Sprintf(
		"%s = call %s (%s) [%s]",
		getStr(c),
		getStr(c.Method),
		strings.Join(
			lo.Map(c.Args, func(v Value, _ int) string { return getStr(v) }),
			", ",
		),
		strings.Join(
			lo.Map(c.binding, func(v Value, _ int) string {
				return getStr(v)
			}),
			", ",
		),
	)
}

// ----------- Switch
func (sw *Switch) String() string {
	return fmt.Sprintf(
		"switch %s default:[%s] {%s}",
		getStr(sw.Cond),
		sw.DefaultBlock.Name,
		strings.Join(
			lo.Map(sw.Label, func(label SwitchLabel, _ int) string {
				return fmt.Sprintf("%s:%s", getStr(label.Value), label.Dest.Name)
			}),
			", ",
		),
	)
}

// ----------- BinOp
var BinaryOpcodeName = map[BinaryOpcode]string{
	OpAnd:    `and`,
	OpAndNot: `and-not`,
	OpOr:     `or`,
	OpXor:    `xor`,
	OpShl:    `shl`,
	OpShr:    `shr`,
	OpAdd:    `add`,
	OpSub:    `sub`,
	OpMod:    `mod`,
	OpMul:    `mul`,
	OpDiv:    `div`,
	OpGt:     `gt`,
	OpLt:     `lt`,
	OpLtEq:   `lt-eq`,
	OpGtEq:   `gt-eq`,
	OpNotEq:  `neq`,
	OpEq:     `eq`,
}

func (b *BinOp) String() string {
	return fmt.Sprintf("%s = %s %s %s", getStr(b), getStr(b.X), BinaryOpcodeName[b.Op], getStr(b.Y))
}

// ----------- UnOp
var UnaryOpcodeName = map[UnaryOpcode]string{
	OpNone: ``,
	OpNot:  `not`,
	OpPlus: `plus`,
	OpNeg:  `neg`,
	OpChan: `chan`,
}

func (u *UnOp) String() string {
	return fmt.Sprintf("%s = %s %s", getStr(u), UnaryOpcodeName[u.Op], getStr(u.X))
}

// ----------- Interface
func (i *Interface) String() string {
	return fmt.Sprintf(
		"%s = Interface %s [%s, %s]",
		getStr(i), i.typs, getStr(i.Len), getStr(i.Cap),
	)
}

// ----------- Field
func (f *Field) String() string {
	return fmt.Sprintf(
		"%s = %s field[%s]",
		getStr(f), getStr(f.I), getStr(f.Key),
	)
}

// ----------- Update
func (s *Update) String() string {
	return fmt.Sprintf(
		"update [%s] = %s",
		getStr(s.address), getStr(s.Value),
	)
}
