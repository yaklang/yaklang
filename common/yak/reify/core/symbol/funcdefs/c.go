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
	"strlen":  {Name: "strlen", Params: []Param{param("s", stringType)}, Return: sizeType},
	"strnlen": {Name: "strnlen", Params: []Param{param("s", stringType), param("maxlen", sizeType)}, Return: sizeType},
	"strdup":  {Name: "strdup", Params: []Param{param("s", stringType)}, Return: stringType},
	"strndup": {Name: "strndup", Params: []Param{param("s", stringType), param("n", sizeType)}, Return: stringType},
	"memcmp":  {Name: "memcmp", Params: []Param{param("left", ptrType), param("right", ptrType), param("num", sizeType)}, Return: intType},
	"bzero":   {Name: "bzero", Params: []Param{param("dest", ptrType), param("num", sizeType)}, Return: voidType},
	"bcopy":   {Name: "bcopy", Params: []Param{param("source", ptrType), param("dest", ptrType), param("num", sizeType)}, Return: voidType},
	"strcmp":  {Name: "strcmp", Params: []Param{param("left", stringType), param("right", stringType)}, Return: intType},
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
	"fputs":     {Name: "fputs", Params: []Param{param("s", stringType), param("stream", ptrType)}, Return: intType},
	"fread":     {Name: "fread", Params: []Param{param("buffer", ptrType), param("size", sizeType), param("count", sizeType), param("stream", ptrType)}, Return: sizeType},
	"fwrite":    {Name: "fwrite", Params: []Param{param("buffer", ptrType), param("size", sizeType), param("count", sizeType), param("stream", ptrType)}, Return: sizeType},
	"perror":    {Name: "perror", Params: []Param{param("message", stringType)}, Return: voidType},
	"time":      {Name: "time", Params: []Param{param("result", ptrType)}, Return: longType},
	"localtime": {Name: "localtime", Params: []Param{param("timer", ptrType)}, Return: ptrType},
	"getpid":    {Name: "getpid", Return: intType},
	"getuid":    {Name: "getuid", Return: intType},
	"qsort":     {Name: "qsort", Params: []Param{param("base", ptrType), param("count", sizeType), param("size", sizeType), param("compare", ptrType)}, Return: voidType},

	"strrchr":     {Name: "strrchr", Params: []Param{param("s", stringType), param("c", intType)}, Return: ptrType},
	"strpbrk":     {Name: "strpbrk", Params: []Param{param("s", stringType), param("accept", stringType)}, Return: ptrType},
	"strspn":      {Name: "strspn", Params: []Param{param("s", stringType), param("accept", stringType)}, Return: sizeType},
	"strcspn":     {Name: "strcspn", Params: []Param{param("s", stringType), param("reject", stringType)}, Return: sizeType},
	"strtok":      {Name: "strtok", Params: []Param{param("s", stringType), param("delim", stringType)}, Return: stringType},
	"strcasecmp":  {Name: "strcasecmp", Params: []Param{param("left", stringType), param("right", stringType)}, Return: intType},
	"strncasecmp": {Name: "strncasecmp", Params: []Param{param("left", stringType), param("right", stringType), param("size", sizeType)}, Return: intType},
	"memchr":      {Name: "memchr", Params: []Param{param("s", ptrType), param("c", intType), param("num", sizeType)}, Return: ptrType},
	"strerror":    {Name: "strerror", Params: []Param{param("errnum", intType)}, Return: stringType},
	"vfprintf":    {Name: "vfprintf", Params: []Param{param("stream", ptrType), param("format", stringType), param("args", ptrType)}, Return: intType},
	"vprintf":     {Name: "vprintf", Params: []Param{param("format", stringType), param("args", ptrType)}, Return: intType},
	"vsnprintf":   {Name: "vsnprintf", Params: []Param{param("buffer", ptrType), param("size", sizeType), param("format", stringType), param("args", ptrType)}, Return: intType},
	"scanf":       {Name: "scanf", Params: []Param{param("format", stringType)}, Return: intType, Variadic: true},
	"sscanf":      {Name: "sscanf", Params: []Param{param("s", stringType), param("format", stringType)}, Return: intType, Variadic: true},
	"fscanf":      {Name: "fscanf", Params: []Param{param("stream", ptrType), param("format", stringType)}, Return: intType, Variadic: true},
	"fgetc":       {Name: "fgetc", Params: []Param{param("stream", ptrType)}, Return: intType},
	"fputc":       {Name: "fputc", Params: []Param{param("c", intType), param("stream", ptrType)}, Return: intType},
	"getchar":     {Name: "getchar", Return: intType},
	"putchar":     {Name: "putchar", Params: []Param{param("c", intType)}, Return: intType},
	"fseek":       {Name: "fseek", Params: []Param{param("stream", ptrType), param("offset", longType), param("whence", intType)}, Return: intType},
	"ftell":       {Name: "ftell", Params: []Param{param("stream", ptrType)}, Return: longType},
	"rewind":      {Name: "rewind", Params: []Param{param("stream", ptrType)}, Return: voidType},
	"feof":        {Name: "feof", Params: []Param{param("stream", ptrType)}, Return: intType},
	"ferror":      {Name: "ferror", Params: []Param{param("stream", ptrType)}, Return: intType},
	"fflush":      {Name: "fflush", Params: []Param{param("stream", ptrType)}, Return: intType},
	"remove":      {Name: "remove", Params: []Param{param("path", stringType)}, Return: intType},
	"rename":      {Name: "rename", Params: []Param{param("oldpath", stringType), param("newpath", stringType)}, Return: intType},
	"unlink":      {Name: "unlink", Params: []Param{param("path", stringType)}, Return: intType},
	"access":      {Name: "access", Params: []Param{param("path", stringType), param("mode", intType)}, Return: intType},
	"getcwd":      {Name: "getcwd", Params: []Param{param("buffer", ptrType), param("size", sizeType)}, Return: stringType},
	"chdir":       {Name: "chdir", Params: []Param{param("path", stringType)}, Return: intType},
	"sleep":       {Name: "sleep", Params: []Param{param("seconds", intType)}, Return: intType},
	"usleep":      {Name: "usleep", Params: []Param{param("usec", intType)}, Return: intType},
	"rand":        {Name: "rand", Return: intType},
	"srand":       {Name: "srand", Params: []Param{param("seed", intType)}, Return: voidType},
	"abs":         {Name: "abs", Params: []Param{param("value", intType)}, Return: intType},
	"labs":        {Name: "labs", Params: []Param{param("value", longType)}, Return: longType},
	"strtoul":     {Name: "strtoul", Params: []Param{param("s", stringType), param("end", ptrType), param("base", intType)}, Return: sizeType},
	"strtoll":     {Name: "strtoll", Params: []Param{param("s", stringType), param("end", ptrType), param("base", intType)}, Return: longType},
	"strtoull":    {Name: "strtoull", Params: []Param{param("s", stringType), param("end", ptrType), param("base", intType)}, Return: sizeType},
	"strtod":      {Name: "strtod", Params: []Param{param("s", stringType), param("end", ptrType)}, Return: Type{Kind: Float, Bits: 64}},
	"strtof":      {Name: "strtof", Params: []Param{param("s", stringType), param("end", ptrType)}, Return: Type{Kind: Float, Bits: 32}},
	"mkstemp":     {Name: "mkstemp", Params: []Param{param("template", ptrType)}, Return: intType},
	"getline":     {Name: "getline", Params: []Param{param("lineptr", ptrType), param("n", ptrType), param("stream", ptrType)}, Return: longType},
	"getdelim":    {Name: "getdelim", Params: []Param{param("lineptr", ptrType), param("n", ptrType), param("delim", intType), param("stream", ptrType)}, Return: longType},
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
