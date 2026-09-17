package yakvm

import (
	"encoding/json"
	"reflect"
	"strconv"

	"github.com/yaklang/yaklang/common/utils"

	"google.golang.org/protobuf/encoding/protowire"
)

type CodesMarshaller struct {
	sourceCode string
	table      map[int]*SymbolTable
}

func NewCodesMarshaller() *CodesMarshaller {
	return &CodesMarshaller{sourceCode: "", table: map[int]*SymbolTable{}}
}

func (c *CodesMarshaller) SetSourceCode(s string) {
	c.sourceCode = s
}

func (c *CodesMarshaller) MarshalWithDebugInfo(tbl *SymbolTable, codes []*Code) ([]byte, error) {
	return c.Marshal(tbl, codes)
}

func (c *CodesMarshaller) Marshal(tbl *SymbolTable, codes []*Code) ([]byte, error) {
	var buf []byte

	var err error
	buf, err = c.marshalSymbolTable(buf, tbl)
	if err != nil {
		return nil, utils.Errorf("marshal symbol table: %s", err)
	}

	buf = protowire.AppendVarint(buf, uint64(len(codes)))
	for _, code := range codes {
		buf, err = c.marshalCode(buf, code)
		if err != nil {
			return nil, utils.Errorf("marshal code error: %s", err)
		}
	}
	return buf, nil
}

func (c *CodesMarshaller) walkSymbolTable(tbl *SymbolTable) []*SymbolTable {
	if tbl == nil {
		return nil
	}
	tables := []*SymbolTable{tbl}
	for _, t := range tbl.children {
		tables = append(tables, c.walkSymbolTable(t)...)
	}
	return tables
}

type SymbolTableDesc struct {
	Verbose            string
	currentSymbolIndex int
	Index              int
	Parent             int
	Children           []int
	NameToId           map[string]int
}

func (c *CodesMarshaller) marshalSymbolTable(buf []byte, tbl *SymbolTable) ([]byte, error) {
	// A cached top-level script can finish while one of its Yak async workers is
	// compiling eval code against the same live symbol table. Snapshot the table
	// under the same read locks used by compiler/runtime lookups so cache
	// serialization cannot race with symbol or nested-scope registration.
	symbolMutex.RLock()
	symbolTableMutex.RLock()
	requireSymbolMutex.RLock()
	defer symbolMutex.RUnlock()
	defer symbolTableMutex.RUnlock()
	defer requireSymbolMutex.RUnlock()

	if tbl.parent != nil {
		return nil, utils.Error("Symbol table is not root table.")
	}

	tables := c.walkSymbolTable(tbl)
	buf = protowire.AppendVarint(buf, uint64(len(tables)))

	for _, tbl := range tables {
		// index - verbose(bytes) index(varint) parentIndex(varint) - childrenCount(varint) childrenIndex(varint) - symbolCount(varint) symbolName(bytes) symbolId(varint)
		// index 最小为1

		// index
		buf = protowire.AppendVarint(buf, uint64(tbl.index))
		// verbose
		buf = protowire.AppendBytes(buf, []byte(tbl.Verbose))
		// currentSymbolIndex
		buf = protowire.AppendVarint(buf, uint64(tbl.currentSymbolIndex))
		// parent
		if tbl.parent == nil {
			buf = protowire.AppendVarint(buf, 0)
		} else {
			buf = protowire.AppendVarint(buf, uint64(tbl.parent.index))
		}
		// children
		buf = protowire.AppendVarint(buf, uint64(len(tbl.children)))
		for _, child := range tbl.children {
			buf = protowire.AppendVarint(buf, uint64(child.index))
		}
		// symbol name to id
		buf = protowire.AppendVarint(buf, uint64(len(tbl.symbolToId)))
		for name, id := range tbl.symbolToId {
			buf = protowire.AppendBytes(buf, []byte(name))
			buf = protowire.AppendVarint(buf, uint64(id))
		}
	}

	return buf, nil
}

func (c *CodesMarshaller) marshalFunc(buf []byte, f *Function) ([]byte, error) {
	buf = protowire.AppendBytes(buf, []byte(f.name))
	buf = protowire.AppendVarint(buf, uint64(f.id))
	if f.isVariableParameter {
		buf = protowire.AppendVarint(buf, uint64(1))
	} else {
		buf = protowire.AppendVarint(buf, uint64(0))
	}
	if f.symbolTable == nil {
		return nil, utils.Error("marshal function failed: no symbol table")
	}
	buf = protowire.AppendVarint(buf, uint64(f.symbolTable.index))
	buf = protowire.AppendVarint(buf, uint64(len(f.paramSymbols)))
	for _, i := range f.paramSymbols {
		buf = protowire.AppendVarint(buf, uint64(i))
	}
	buf = protowire.AppendVarint(buf, uint64(len(f.codes)))
	var err error
	for _, code := range f.codes {
		buf, err = c.marshalCode(buf, code)
		if err != nil {
			return nil, utils.Errorf("marshal code error: %s", err)
		}
	}
	return buf, nil
}

func (c *CodesMarshaller) marshalAny(buf []byte, i interface{}) ([]byte, error) {
	var err error
	switch ret := i.(type) {
	case []*Code:
		buf = protowire.AppendVarint(buf, 2)
		buf = protowire.AppendVarint(buf, uint64(len(ret)))
		for _, code := range ret {
			buf, err = c.marshalCode(buf, code)
			if err != nil {
				return nil, utils.Errorf("marshan Any.Codes in Op failed: %s", err)
			}
		}
		return buf, nil
	case *Function:
		buf = protowire.AppendVarint(buf, 1)
		buf, err = c.marshalFunc(buf, ret)
		if err != nil {
			return nil, utils.Errorf("marshal function failed: %s", err)
		}
		return buf, nil
	default:
		buf = protowire.AppendVarint(buf, 0)

		// isNil
		if i == nil {
			buf = protowire.AppendVarint(buf, 0)
			return buf, nil
		} else {
			switch v := i.(type) {
			case string: // string 直接quote，不需要json序列化
				buf = protowire.AppendVarint(buf, uint64(reflect.TypeOf(i).Kind()))
				buf = protowire.AppendBytes(buf, []byte(strconv.Quote(v)))
				return buf, nil
			case []byte: // 对[]byte做特殊处理
				buf = protowire.AppendVarint(buf, 27)
			default:
				buf = protowire.AppendVarint(buf, uint64(reflect.TypeOf(i).Kind()))
			}
			bufBytes, err := json.Marshal(i)
			if err != nil {
				return nil, err
			}
			buf = protowire.AppendBytes(buf, bufBytes)
			return buf, nil
		}
	}
}

func (c *CodesMarshaller) marshalValue(buf []byte, v *Value) ([]byte, error) {
	buf = protowire.AppendBytes(buf, []byte(v.TypeVerbose))
	buf = protowire.AppendBytes(buf, []byte(v.GetLiteral()))
	var err error
	buf, err = c.marshalAny(buf, v.Value)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func (c *CodesMarshaller) marshalCode(buf []byte, code *Code) ([]byte, error) {
	buf = protowire.AppendVarint(buf, uint64(code.Opcode))
	buf = protowire.AppendVarint(buf, uint64(code.Unary))

	if code.Op1 == nil {
		// no op1
		buf = protowire.AppendVarint(buf, 0)
	} else {
		// have op1
		buf = protowire.AppendVarint(buf, 1)
		var err error
		buf, err = c.marshalValue(buf, code.Op1)
		if err != nil {
			return nil, err
		}
	}

	if code.Op2 == nil {
		// have op2
		buf = protowire.AppendVarint(buf, 0)
	} else {
		buf = protowire.AppendVarint(buf, 1)
		var err error
		buf, err = c.marshalValue(buf, code.Op2)
		if err != nil {
			return nil, err
		}
	}

	if code.StartLineNumber > 0 {
		buf = protowire.AppendVarint(buf, 1)
		buf = protowire.AppendVarint(buf, uint64(code.StartLineNumber))
		buf = protowire.AppendVarint(buf, uint64(code.StartColumnNumber))
		buf = protowire.AppendVarint(buf, uint64(code.EndLineNumber))
		buf = protowire.AppendVarint(buf, uint64(code.EndColumnNumber))
	} else {
		buf = protowire.AppendVarint(buf, 0)
	}
	return buf, nil
}

func (c *CodesMarshaller) Unmarshal(buf []byte) (*SymbolTable, []*Code, error) {
	return c.UnmarshalWithLimits(buf, DecodeLimits{})
}
