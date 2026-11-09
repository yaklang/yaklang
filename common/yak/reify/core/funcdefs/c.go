package funcdefs

var (
	voidType   = Type{Kind: Void}
	intType    = Type{Kind: Int, Bits: 32}
	longType   = Type{Kind: Int, Bits: 64}
	sizeType   = Type{Kind: Size, Bits: 64}
	ptrType    = Type{Kind: Pointer, Bits: 64}
	stringType = Type{Kind: String, Bits: 64}
)

func param(name string, typ Type) Param { return Param{Name: name, Type: typ} }

// cFunctions covers the libc and runtime entry points common in ELF binaries.
// Unknown functions keep their inferred LowIR types.
var cFunctions = map[string]Definition{
	"puts":   {Name: "puts", Params: []Param{param("s", stringType)}, Return: intType},
	"printf": {Name: "printf", Params: []Param{param("format", stringType)}, Return: intType, Variadic: true},
	"fprintf": {
		Name: "fprintf", Params: []Param{param("stream", ptrType), param("format", stringType)},
		Return: intType, Variadic: true,
	},
	"sprintf": {
		Name: "sprintf", Params: []Param{param("buffer", ptrType), param("format", stringType)},
		Return: intType, Variadic: true,
	},
	"snprintf": {
		Name: "snprintf", Params: []Param{param("buffer", ptrType), param("size", sizeType), param("format", stringType)},
		Return: intType, Variadic: true,
	},
	"malloc":  {Name: "malloc", Params: []Param{param("size", sizeType)}, Return: ptrType},
	"calloc":  {Name: "calloc", Params: []Param{param("count", sizeType), param("size", sizeType)}, Return: ptrType},
	"realloc": {Name: "realloc", Params: []Param{param("ptr", ptrType), param("size", sizeType)}, Return: ptrType},
	"free":    {Name: "free", Params: []Param{param("ptr", ptrType)}, Return: voidType},
	"memcpy": {
		Name: "memcpy", Params: []Param{param("destination", ptrType), param("source", ptrType), param("num", sizeType)},
		Return: ptrType,
	},
	"memmove": {
		Name: "memmove", Params: []Param{param("destination", ptrType), param("source", ptrType), param("num", sizeType)},
		Return: ptrType,
	},
	"memset": {
		Name: "memset", Params: []Param{param("destination", ptrType), param("value", intType), param("num", sizeType)},
		Return: ptrType,
	},
	"strlen": {Name: "strlen", Params: []Param{param("s", stringType)}, Return: sizeType},
	"strcmp": {Name: "strcmp", Params: []Param{param("left", stringType), param("right", stringType)}, Return: intType},
	"strncmp": {
		Name: "strncmp", Params: []Param{param("left", stringType), param("right", stringType), param("size", sizeType)},
		Return: intType,
	},
	"strcpy": {Name: "strcpy", Params: []Param{param("destination", ptrType), param("source", stringType)}, Return: ptrType},
	"strncpy": {
		Name: "strncpy", Params: []Param{param("destination", ptrType), param("source", stringType), param("num", sizeType)},
		Return: ptrType,
	},
	"strcat": {Name: "strcat", Params: []Param{param("destination", ptrType), param("source", stringType)}, Return: ptrType},
	"strchr": {Name: "strchr", Params: []Param{param("s", stringType), param("c", intType)}, Return: ptrType},
	"strstr": {Name: "strstr", Params: []Param{param("haystack", stringType), param("needle", stringType)}, Return: ptrType},
	"atoi":   {Name: "atoi", Params: []Param{param("s", stringType)}, Return: intType},
	"strtol": {
		Name: "strtol", Params: []Param{param("s", stringType), param("end", ptrType), param("base", intType)},
		Return: longType,
	},
	"getenv": {Name: "getenv", Params: []Param{param("name", stringType)}, Return: stringType},
	"open": {
		Name: "open", Params: []Param{param("path", stringType), param("flags", intType)},
		Return: intType, Variadic: true,
	},
	"read": {
		Name: "read", Params: []Param{param("fd", intType), param("buffer", ptrType), param("size", sizeType)},
		Return: longType,
	},
	"write": {
		Name: "write", Params: []Param{param("fd", intType), param("buffer", ptrType), param("size", sizeType)},
		Return: longType,
	},
	"close":  {Name: "close", Params: []Param{param("fd", intType)}, Return: intType},
	"exit":   {Name: "exit", Params: []Param{param("status", intType)}, Return: voidType},
	"abort":  {Name: "abort", Return: voidType},
	"fopen":  {Name: "fopen", Params: []Param{param("path", stringType), param("mode", stringType)}, Return: ptrType},
	"fclose": {Name: "fclose", Params: []Param{param("stream", ptrType)}, Return: intType},
	"fgets": {
		Name: "fgets", Params: []Param{param("buffer", ptrType), param("size", intType), param("stream", ptrType)},
		Return: ptrType,
	},
	"fputs": {Name: "fputs", Params: []Param{param("s", stringType), param("stream", ptrType)}, Return: intType},
	"__libc_start_main": {
		Name: "__libc_start_main",
		Params: []Param{
			param("main", ptrType), param("argc", intType), param("argv", ptrType),
			param("init", ptrType), param("fini", ptrType), param("rtld_fini", ptrType), param("stack_end", ptrType),
		},
		Return: intType,
	},
	"__errno_location": {Name: "__errno_location", Return: ptrType},
	"__stack_chk_fail": {Name: "__stack_chk_fail", Return: voidType},
	"__printf_chk": {
		Name: "__printf_chk", Params: []Param{param("flag", intType), param("format", stringType)},
		Return: intType, Variadic: true,
	},
	"__fprintf_chk": {
		Name: "__fprintf_chk", Params: []Param{param("stream", ptrType), param("flag", intType), param("format", stringType)},
		Return: intType, Variadic: true,
	},
	"__vfprintf_chk": {
		Name:   "__vfprintf_chk",
		Params: []Param{param("stream", ptrType), param("flag", intType), param("format", stringType), param("args", ptrType)},
		Return: intType,
	},
	"__sprintf_chk": {
		Name:   "__sprintf_chk",
		Params: []Param{param("buffer", ptrType), param("flag", intType), param("size", sizeType), param("format", stringType)},
		Return: intType, Variadic: true,
	},
	"dcgettext": {
		Name: "dcgettext", Params: []Param{param("domain", stringType), param("message", stringType), param("category", intType)},
		Return: stringType,
	},
	"strerror_r": {
		Name: "strerror_r", Params: []Param{param("errnum", intType), param("buffer", ptrType), param("size", sizeType)},
		Return: stringType,
	},
}
