package visitors

import (
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func (c *Compiler) pushOpcodeFlag(f yakvm.OpcodeFlag) *yakvm.Code {
	code := &yakvm.Code{
		Opcode: f,
	}
	c.pushOpcode(code)
	return code
}

func (c *Compiler) pushOpcode(code *yakvm.Code) *yakvm.Code {
	code.StartLineNumber = c.position[0]
	code.StartColumnNumber = c.position[1]
	code.EndLineNumber = c.position[2]
	code.EndColumnNumber = c.position[3]
	code.SourceCodePointer = c.sourceCodePointer
	code.SourceCodeFilePath = c.sourceCodeFilePath
	c.codes = append(c.codes, code)
	return code
}

func (c *Compiler) pushScope(verbose string) {
	c.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpScope,
		Unary:  c.symbolTable.GetTableCount(),
		Op1: &yakvm.Value{
			TypeVerbose: verbose,
			Value:       verbose,
			Literal:     verbose,
		},
	})
}

func (c *Compiler) pushScopeEnd() {
	c.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpScopeEnd,
	})
}

func (s *Compiler) pushInt(i int) *yakvm.Code {
	return s.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "nasl_int",
			Value:       i,
		},
	})
}

func (s *Compiler) pushFloat(f float64) {
	panic("not implemented")
	s.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "float64",
			Value:       f,
		},
	})
}

func (s *Compiler) pushBool(i bool) *yakvm.Code {
	if i {
		return s.pushInt(1)
	} else {
		return s.pushInt(0)
	}
}

func (s *Compiler) pushString(i string) {
	s.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "nasl_string",
			Value:       i,
			Literal:     i,
		},
	})
}

func (c *Compiler) pushRef(name string) *yakvm.Code {
	var code *yakvm.Code
	code = &yakvm.Code{
		Opcode: yakvm.OpPushRef,
		Unary:  c.GetSymbolId(name),
	}
	c.pushOpcode(code)
	return code
}

func (c *Compiler) pushUninitedLeftRef(name string) *yakvm.Code {
	id, ok := c.symbolTable.GetSymbolByVariableName(name)
	if !ok {
		newid, err := c.symbolTable.NewSymbolWithReturn(name)
		if err != nil {
			c.AddError(err)
			return nil
		}
		id = newid
	}
	code := &yakvm.Code{
		Opcode: yakvm.OpPushLeftRef,
		Unary:  id,
	}
	c.pushOpcode(code)
	return code
}

func (c *Compiler) pushLeftRef(name string) *yakvm.Code {
	id, ok := c.symbolTable.GetSymbolByVariableName(name)
	if !ok {
		newid, err := c.symbolTable.NewSymbolWithReturn(name)
		if err != nil {
			c.AddError(err)
			return nil
		}
		id = newid
	}
	c.symbolTable.SetIdIsInited(id)
	code := &yakvm.Code{
		Opcode: yakvm.OpPushLeftRef,
		Unary:  id,
	}
	c.pushOpcode(code)
	return code
}

func (c *Compiler) pushJustAssigin() {
	c.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpAssign,
	})
}

func (c *Compiler) pushAutoMapAssigin() {
	c.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpAssign,
		Op1:    yakvm.NewAutoValue("auto_created"),
	})
}

func (c *Compiler) pushAssigin() {
	c.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpAssign,
	})
	//c.pushOpcode(&yakvm.Code{
	//	Opcode: yakvm.OpAssign,
	//	Unary:  1,
	//})
}

func (c *Compiler) pushGlobalDeclare() {
	c.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpAssign,
		Op1:    yakvm.NewStringValue("nasl_global_declare"),
	})
}

func (c *Compiler) pushDeclare() {
	c.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpAssign,
		Op1:    yakvm.NewStringValue("nasl_declare"),
	})
}

func (c *Compiler) pushCall(i int) *yakvm.Code {
	code := &yakvm.Code{
		Opcode: yakvm.OpCall,
		Unary:  i,
	}
	c.pushOpcode(code)
	return code
}

func (c *Compiler) pushList(i int) {
	c.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpNewSlice,
		Unary:  i,
	})
}

func (c *Compiler) pushGenList(i int) {
	c.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpList,
		Unary:  i,
	})
}

func (c *Compiler) pushBitOr() {
	c.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpOr,
	})
}

func (c *Compiler) pushBitAnd() {
	c.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpAnd,
	})
}

func (c *Compiler) pushJmp() *yakvm.Code {
	code := &yakvm.Code{
		Opcode: yakvm.OpJMP,
	}
	c.pushOpcode(code)
	return code
}

func (c *Compiler) pushJmpIfFalse() *yakvm.Code {
	code := &yakvm.Code{
		Opcode: yakvm.OpJMPFOP,
	}
	c.pushOpcode(code)
	return code
}

func (c *Compiler) pushJmpIfTrue() *yakvm.Code {
	code := &yakvm.Code{
		Opcode: yakvm.OpJMPTOP,
	}
	c.pushOpcode(code)
	return code
}

func (s *Compiler) pushValue(i *yakvm.Value) {
	s.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1:    i,
	})
}

func (s *Compiler) pushNewSlice(n int) {
	s.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpNewSlice,
		Unary:  n,
	})
}

func (s *Compiler) pushIterableCall() {
	s.pushOpcode(&yakvm.Code{
		Opcode: yakvm.OpIterableCall,
		Unary:  1,
	})
}
