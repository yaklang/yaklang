package ssa

// for DataFlowNode cover
func ToNode(a any) (Node, bool)   { u, ok := a.(Node); return u, ok }
func ToValue(n any) (Value, bool) { v, ok := n.(Value); return v, ok }
func ToUser(n any) (User, bool)   { u, ok := n.(User); return u, ok }

func ToFunction(n Node) (*Function, bool) { u, ok := n.(*Function); return u, ok }

// value
func ToConst(v Instruction) (*ConstInst, bool)     { c, ok := v.(*ConstInst); return c, ok }
func IsConst(v Instruction) bool                   { _, ok := ToConst(v); return ok }
func ToPhi(v Instruction) (*Phi, bool)             { p, ok := v.(*Phi); return p, ok }
func ToExternLib(v Instruction) (*ExternLib, bool) { p, ok := v.(*ExternLib); return p, ok }
func ToParameter(v Instruction) (*Parameter, bool) { p, ok := v.(*Parameter); return p, ok }
func ToUndefined(v Instruction) (*Undefined, bool) { p, ok := v.(*Undefined); return p, ok }

func ToBinOp(v Instruction) (*BinOp, bool) { c, ok := v.(*BinOp); return c, ok }
func ToUnOp(v Instruction) (*UnOp, bool)   { c, ok := v.(*UnOp); return c, ok }

func ToCall(v Instruction) (*Call, bool) { p, ok := v.(*Call); return p, ok }
func ToMake(v Instruction) (*Make, bool) { p, ok := v.(*Make); return p, ok }

// memory
func ToObject(v Instruction) (*Make, bool)   { o, ok := v.(*Make); return o, ok }
func ToField(v Instruction) (*Field, bool)   { o, ok := v.(*Field); return o, ok }
func ToUpdate(v Instruction) (*Update, bool) { o, ok := v.(*Update); return o, ok }

// type cover

func ToObjectType(t Type) (*ObjectType, bool)     { o, ok := t.(*ObjectType); return o, ok }
func ToFunctionType(t Type) (*FunctionType, bool) { f, ok := t.(*FunctionType); return f, ok }
