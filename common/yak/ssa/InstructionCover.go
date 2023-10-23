package ssa

// for DataFlowNode cover
func ToNode(a any) (Node, bool)    { u, ok := a.(Node); return u, ok }
func ToValue(n Node) (Value, bool) { v, ok := n.(Value); return v, ok }
func ToUser(n Node) (User, bool)   { u, ok := n.(User); return u, ok }

// value
func ToConst(v Instruction) (*ConstInst, bool) { c, ok := v.(*ConstInst); return c, ok }
func ToPhi(v Instruction) (*Phi, bool)         { p, ok := v.(*Phi); return p, ok }

// memory
func ToObject(v Instruction) (*Make, bool) { o, ok := v.(*Make); return o, ok }
func ToField(v Instruction) (*Field, bool) { o, ok := v.(*Field); return o, ok }

// type cover

func ToObjectType(t Type) (*ObjectType, bool)     { o, ok := t.(*ObjectType); return o, ok }
func ToFunctionType(t Type) (*FunctionType, bool) { f, ok := t.(*FunctionType); return f, ok }
