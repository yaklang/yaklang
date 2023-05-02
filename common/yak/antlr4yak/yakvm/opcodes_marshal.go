package yakvm

import (
	"encoding/json"
	"reflect"
	"strconv"
	"yaklang/common/utils"

	"google.golang.org/protobuf/encoding/protowire"
)

type CodesMarshaller struct {
	table map[int]*SymbolTable
}

func NewCodesMarshaller() *CodesMarshaller {
	return &CodesMarshaller{map[int]*SymbolTable{}}
}

func (c *CodesMarshaller) Unmarshal(buf []byte) (*SymbolTable, []*Code, error) {
	tbl, buf, err := c.consumeSymbolTable(buf)
	if err != nil {
		return nil, nil, utils.Errorf("consume symbol table failed: %s", err)
	}

	i, n := protowire.ConsumeVarint(buf)
	buf = buf[n:]
	codeLen := int(i)
	codes := make([]*Code, codeLen)
	for i := 0; i < codeLen; i++ {
		codes[i], buf, err = c.consumeCode(buf)
		if err != nil {
			return nil, nil, utils.Errorf("unmarshal yakvm.Code failed: %s", err)
		}
	}
	return tbl, codes, nil
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
	var tables = []*SymbolTable{tbl}
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

func (c *CodesMarshaller) descToIns(desc *SymbolTableDesc, id int) *SymbolTable {
	if id == 0 {
		return nil
	}
	tbl := c.getSymbolTableById(desc.Index)
	tbl.index = desc.Index
	tbl.Verbose = desc.Verbose
	tbl.currentSymbolIndex = desc.currentSymbolIndex
	tbl.parent = c.getSymbolTableById(desc.Parent)
	tbl.symbolToId = desc.NameToId
	if len(desc.Children) != 0 {
		tbl.children = make([]*SymbolTable, len(desc.Children))
		for i, childId := range desc.Children {
			tbl.children[i] = c.getSymbolTableById(childId)
		}
	}
	return tbl
}

func (c *CodesMarshaller) getSymbolTableById(id int) *SymbolTable {
	if tbl, ok := c.table[id]; ok {
		return tbl
	}
	return nil
}

func (c *CodesMarshaller) consumeSymbolTable(buf []byte) (*SymbolTable, []byte, error) {

	i, n := protowire.ConsumeVarint(buf)
	buf = buf[n:]
	blockLen := int(i)
	c.table = make(map[int]*SymbolTable, blockLen)

	var tableDesc = make(map[int]*SymbolTableDesc)
	for index := 1; index <= blockLen; index++ {
		tbl := &SymbolTableDesc{NameToId: make(map[string]int)}
		// index
		i, n = protowire.ConsumeVarint(buf)
		tbl.Index = int(i)
		buf = buf[n:]

		// verbose
		raw, n := protowire.ConsumeBytes(buf)
		tbl.Verbose = string(raw)
		buf = buf[n:]

		// currentSymbolIndex
		i, n = protowire.ConsumeVarint(buf)
		tbl.currentSymbolIndex = int(i)
		buf = buf[n:]

		// parent
		i, n = protowire.ConsumeVarint(buf)
		tbl.Parent = int(i)
		buf = buf[n:]

		// children
		i, n = protowire.ConsumeVarint(buf)
		tbl.Children = make([]int, int(i))
		buf = buf[n:]
		for i := 0; i < len(tbl.Children); i++ {
			childId, childIndexOffset := protowire.ConsumeVarint(buf)
			if childIndexOffset <= 0 {
				return nil, nil, utils.Error("consume children index failed")
			}
			buf = buf[childIndexOffset:]
			tbl.Children[i] = int(childId)
		}

		// symbol name to id
		i, n = protowire.ConsumeVarint(buf)
		buf = buf[n:]
		for index := 0; index < int(i); index++ {
			name, n := protowire.ConsumeBytes(buf)
			buf = buf[n:]
			id, n := protowire.ConsumeVarint(buf)
			buf = buf[n:]
			tbl.NameToId[string(name)] = int(id)
		}

		tableDesc[tbl.Index] = tbl
	}

	// 先初始化symboltable
	for index := 1; index <= blockLen; index++ {
		c.table[index] = &SymbolTable{}
	}
	// 对symboltable赋值
	for id, desc := range tableDesc {
		c.descToIns(desc, id)
	}
	rootTable := c.table[1]
	// 设置rootTable
	rootTable.tableCount = blockLen
	rootTable.idToSymbolTable = c.table

	return rootTable, buf, nil
}

func (c *CodesMarshaller) marshalSymbolTable(buf []byte, tbl *SymbolTable) ([]byte, error) {
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

func (c *CodesMarshaller) consumeFunc(buf []byte) (*Function, []byte, error) {
	f := &Function{
		id:                  0,
		name:                "",
		codes:               nil,
		symbolTable:         nil,
		paramSymbols:        nil,
		isVariableParameter: false,
	}
	results, n := protowire.ConsumeBytes(buf)
	f.name = string(results)
	buf = buf[n:]

	i, n := protowire.ConsumeVarint(buf)
	f.id = int(i)
	buf = buf[n:]

	i, n = protowire.ConsumeVarint(buf)
	if i == 1 {
		// 可变参数
		f.isVariableParameter = true
	}
	buf = buf[n:]

	// symbol table
	i, n = protowire.ConsumeVarint(buf)
	f.symbolTable = c.getSymbolTableById(int(i))
	buf = buf[n:]

	// param symbols
	i, n = protowire.ConsumeVarint(buf)
	f.paramSymbols = make([]int, int(i))
	buf = buf[n:]
	for index := range f.paramSymbols {
		i, n = protowire.ConsumeVarint(buf)
		f.paramSymbols[index] = int(i)
		buf = buf[n:]
	}

	// codes
	i, n = protowire.ConsumeVarint(buf)
	f.codes = make([]*Code, int(i))
	buf = buf[n:]
	for index := range f.codes {
		var err error
		f.codes[index], buf, err = c.consumeCode(buf)
		if err != nil {
			return nil, nil, err
		}
	}
	return f, buf, nil
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

func (c *CodesMarshaller) consumeAny(buf []byte) (interface{}, []byte, error) {
	i, n := protowire.ConsumeVarint(buf)
	buf = buf[n:]
	switch i {
	case 1:
		// function
		var f *Function
		var err error
		f, buf, err = c.consumeFunc(buf)
		if err != nil {
			return nil, nil, err
		}
		return f, buf, nil
	case 0:

		// type
		i, n := protowire.ConsumeVarint(buf)
		buf = buf[n:]
		if i == 0 {
			return nil, buf, nil
		}

		var typ reflect.Type
		kind := reflect.Kind(i)
		switch kind {
		case reflect.Int:
			typ = literalReflectType_Int
		case reflect.Int8:
			typ = literalReflectType_Int8
		case reflect.Int16:
			typ = literalReflectType_Int16
		case reflect.Int32:
			typ = literalReflectType_Int32
		case reflect.Int64:
			typ = literalReflectType_Int64
		case reflect.Uint:
			typ = literalReflectType_Uint
		case reflect.Uint8:
			typ = literalReflectType_Uint8
		case reflect.Uint16:
			typ = literalReflectType_Uint16
		case reflect.Uint32:
			typ = literalReflectType_Uint32
		case reflect.Uint64:
			typ = literalReflectType_Uint64
		case reflect.Float32:
			typ = literalReflectType_Float32
		case reflect.Float64:
			typ = literalReflectType_Float64
		case reflect.String:
			typ = literalReflectType_String
		case reflect.Bool:
			typ = literalReflectType_Bool
		case reflect.Map:
			typ = reflect.MapOf(literalReflectType_String, literalReflectType_Interface)
		case 27: // bytes
			typ = reflect.SliceOf(literalReflectType_Byte)
		}
		if typ == nil {
			return nil, nil, utils.Errorf("unsupported type: %s", kind)
		}

		refret := reflect.New(typ)
		b, n := protowire.ConsumeBytes(buf)
		buf = buf[n:]
		if typ == literalReflectType_String {
			ret, err := strconv.Unquote(string(b))
			if err != nil {
				return nil, nil, err
			}
			return ret, buf, nil
		} else {
			err := json.Unmarshal(b, refret.Interface())
			ret := refret.Elem().Interface()

			if err != nil {
				return nil, nil, err
			}
			return ret, buf, nil
		}

	case 2: // []*Code
		iInt, n := protowire.ConsumeVarint(buf)
		buf = buf[n:]
		codeLen := int(iInt)
		codes := make([]*Code, codeLen)
		var err error
		for i := 0; i < codeLen; i++ {
			codes[i], buf, err = c.consumeCode(buf)
			if err != nil {
				return nil, nil, utils.Errorf("consume code in Any(Op) failed: %s", err)
			}
		}
		return codes, buf, nil
	default:
		return nil, nil, utils.Errorf("cannot parse obj: %v", i)
	}
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

func (c *CodesMarshaller) consumeValue(buf []byte) (*Value, []byte, error) {
	v := &Value{
		TypeVerbose: "",
		Value:       nil,
		Literal:     "",
		SymbolId:    0,
		CallerRef:   nil,
		CalleeRef:   nil,
	}
	i, n := protowire.ConsumeBytes(buf)
	buf = buf[n:]
	v.TypeVerbose = string(i)

	i, n = protowire.ConsumeBytes(buf)
	buf = buf[n:]
	v.Literal = string(i)

	// value
	var err error
	v.Value, buf, err = c.consumeAny(buf)
	if err != nil {
		return nil, nil, utils.Errorf("consume any value error: %v", err)
	}
	return v, buf, nil
}

func (c *CodesMarshaller) marshalValue(buf []byte, v *Value) ([]byte, error) {
	buf = protowire.AppendBytes(buf, []byte(v.TypeVerbose))
	buf = protowire.AppendBytes(buf, []byte(v.Literal))
	var err error
	buf, err = c.marshalAny(buf, v.Value)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func (c *CodesMarshaller) consumeCode(buf []byte) (*Code, []byte, error) {
	code := &Code{}
	i, n := protowire.ConsumeVarint(buf)
	code.Opcode = OpcodeFlag(i)
	buf = buf[n:]

	i, n = protowire.ConsumeVarint(buf)
	code.Unary = int(i)
	buf = buf[n:]

	var err error
	i, n = protowire.ConsumeVarint(buf)
	buf = buf[n:]
	if i == 1 {
		// 有 op1
		code.Op1, buf, err = c.consumeValue(buf)
		if err != nil {
			return nil, nil, utils.Errorf("consume value(op1) failed: %v", err)
		}
	}

	i, n = protowire.ConsumeVarint(buf)
	buf = buf[n:]
	if i == 1 {
		code.Op2, buf, err = c.consumeValue(buf)
		if err != nil {
			return nil, nil, utils.Errorf("consume value(op2) failed: %v", err)
		}
	}

	i, n = protowire.ConsumeVarint(buf)
	buf = buf[n:]
	if i == 1 {
		i, n = protowire.ConsumeVarint(buf)
		code.StartLineNumber = int(i)
		buf = buf[n:]

		i, n = protowire.ConsumeVarint(buf)
		code.StartColumnNumber = int(i)
		buf = buf[n:]

		i, n = protowire.ConsumeVarint(buf)
		code.EndLineNumber = int(i)
		buf = buf[n:]

		i, n = protowire.ConsumeVarint(buf)
		code.EndColumnNumber = int(i)
		buf = buf[n:]
	}

	return code, buf, nil
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
