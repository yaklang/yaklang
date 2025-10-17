package builtin

import (
	"github.com/yaklang/yaklang/common/utils"
	yaklangspec "github.com/yaklang/yaklang/common/yak/yaklang/spec"
)

// -----------------------------------------------------------------------------

var YaklangBaseLib = map[string]interface{}{
	"retry":     utils.Retry2,
	"append":    Append,
	"copy":      Copy,
	"delete":    Delete,
	"get":       Get,
	"len":       Len,
	"cap":       Cap,
	"mkmap":     Mkmap,
	"mapFrom":   MapFrom,
	"mapOf":     MapOf,
	"mkslice":   Mkslice,
	"slice":     Mkslice,
	"sliceFrom": SliceFrom,
	"sliceOf":   SliceOf,
	"sub":       SubSlice,
	"panic":     Panic,
	"panicf":    Panicf,
	"print":     print,
	"printf":    printf,
	"println":   println,
	"sprint":    sprint,
	"sprintf":   sprintf,
	"sprintln":  sprintln,
	"fprint":    fprint,
	"fprintf":   fprintf,
	"fprintln":  fprintln,
	"set":       Set,
	"make":      Make,
	"close":     CloseChan,
	"max":       Max,
	"min":       Min,
	"error":     utils.Error,

	"undefined": yaklangspec.Undefined,

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
	// yaklangspec.Import("", exports)
}

// -----------------------------------------------------------------------------
