package builtin

import (
	"fmt"
	yaklangspec "yaklang.io/yaklang/common/yak/yaklang/spec"
)

// -----------------------------------------------------------------------------

var YaklangBaseLib = map[string]interface{}{
	"append":    Append,
	"copy":      Copy,
	"delete":    Delete,
	"get":       Get,
	"len":       Len,
	"cap":       Cap,
	"mkmap":     Mkmap,
	"mapFrom":   MapFrom,
	"mapOf":     MapOf,
	"panic":     Panic,
	"panicf":    Panicf,
	"print":     fmt.Print,
	"printf":    fmt.Printf,
	"println":   fmt.Println,
	"sprint":    fmt.Sprint,
	"sprintf":   fmt.Sprintf,
	"sprintln":  fmt.Sprintln,
	"fprintln":  fmt.Fprintln,
	"set":       Set,
	"mkslice":   Mkslice,
	"slice":     Mkslice,
	"sliceFrom": sliceFrom,
	"sliceOf":   SliceOf,
	"sub":       SubSlice,
	"make":      Make,
	"close":     CloseChan,

	"float":   TyFloat64,
	"float64": TyFloat64,
	"float32": TyFloat32,
	"int8":    TyInt8,
	"int16":   TyInt16,
	"int32":   TyInt32,
	"int64":   TyInt64,
	"int":     TyInt,
	"uint":    TyUint,
	"byte":    TyUint8,
	"uint8":   TyUint8,
	"uint16":  TyUint16,
	"uint32":  TyUint32,
	"uint64":  TyUint64,
	"string":  TyString,
	"bool":    TyBool,
	"var":     TyVar,
	"type":    typeOf,

	"max": Max,
	"min": Min,

	"undefined": yaklangspec.Undefined,
	"nil":       nil,
	"true":      true,
	"false":     false,

	"$elem":    Elem,
	"$neg":     Neg,
	"$mul":     Mul,
	"$quo":     Quo,
	"$mod":     Mod,
	"$add":     Add,
	"$sub":     Sub,
	"$ternary": Ternary,
	"$in":      In,

	"$xor":    Xor,
	"$lshr":   Lshr,
	"$rshr":   Rshr,
	"$bitand": BitAnd,
	"$bitor":  BitOr,
	"$bitnot": BitNot,
	"$andnot": AndNot,

	"$lt":  LT,
	"$gt":  GT,
	"$le":  LE,
	"$ge":  GE,
	"$eq":  EQ,
	"$ne":  NE,
	"$not": Not,
}

func init() {
	yaklangspec.SubSlice = SubSlice
	yaklangspec.SliceFrom = SliceFrom
	yaklangspec.SliceFromTy = SliceFromTy
	yaklangspec.Slice = Slice
	yaklangspec.Map = Map
	yaklangspec.MapFrom = MapFrom
	yaklangspec.MapInit = MapInit
	yaklangspec.StructInit = StructInit
	yaklangspec.EQ = EQ
	yaklangspec.GetVar = GetVar
	yaklangspec.Get = Get
	yaklangspec.SetIndex = SetIndex
	yaklangspec.Add = Add
	yaklangspec.Sub = Sub
	yaklangspec.Mul = Mul
	yaklangspec.Quo = Quo
	yaklangspec.Mod = Mod
	yaklangspec.Xor = Xor
	yaklangspec.Lshr = Lshr
	yaklangspec.Rshr = Rshr
	yaklangspec.BitAnd = BitAnd
	yaklangspec.BitOr = BitOr
	yaklangspec.AndNot = AndNot
	yaklangspec.Inc = Inc
	yaklangspec.Dec = Dec
	//yaklangspec.Import("", exports)
}

// -----------------------------------------------------------------------------
