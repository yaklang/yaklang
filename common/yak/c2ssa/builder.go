package c2ssa

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/ssa"

	cparser "github.com/yaklang/yaklang/common/yak/antlr4c/parser"
)

type SSABuilder struct {
	*ssa.PreHandlerBase
}

var Builder ssa.Builder = &SSABuilder{}

func CreateBuilder() ssa.Builder {
	builder := &SSABuilder{
		PreHandlerBase: ssa.NewPreHandlerBase(initHandler),
	}
	builder.WithLanguageConfigOpts(
		ssa.WithLanguageConfigBind(true),
		ssa.WithLanguageConfigVirtualImport(true),
		ssa.WithLanguageBuilder(builder),
	)
	return builder
}

func initHandler(fb *ssa.FunctionBuilder) {
	container := fb.EmitEmptyContainer()
	fb.GetProgram().GlobalScope = container
}

func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	extension := filepath.Ext(path)
	return extension == ".c" || extension == ".h"
}

func (s *SSABuilder) PreHandlerFile(ast ssa.FrontAST, editor *memedit.MemEditor, builder *ssa.FunctionBuilder) {
	builder.GetProgram().GetApplication().Build(ast, editor, builder)
}

func (s *SSABuilder) PreHandlerProject(fileSystem fi.FileSystem, ast ssa.FrontAST, functionBuilder *ssa.FunctionBuilder, editor *memedit.MemEditor) error {
	prog := functionBuilder.GetProgram()
	if prog == nil {
		log.Errorf("program is nil")
		return nil
	}
	if prog.ExtraFile == nil {
		prog.ExtraFile = make(map[string]string)
	}
	prog.Build(ast, editor, functionBuilder)
	prog.GetIncludeFiles()
	return nil
}

func (s *SSABuilder) BuildFromAST(raw ssa.FrontAST, builder *ssa.FunctionBuilder) error {
	ast, ok := raw.(*cparser.CompilationUnitContext)
	if !ok {
		return utils.Errorf("invalid AST type")
	}
	SpecialTypes := map[string]ssa.Type{
		"void":    ssa.CreateAnyType(),
		"bool":    ssa.CreateBooleanType(),
		"complex": ssa.CreateAnyType(),
	}
	SpecialValue := map[string]ssa.Value{}

	builder.SupportClosure = false
	builder.WithExternValue(cBuildIn)
	builder.WithExternSideEffect(cSideEffect)

	astBuilder := &astbuilder{
		FunctionBuilder: builder,
		cmap:            []map[string]struct{}{},
		importMap:       map[string]*PackageInfo{},
		result:          map[string][]string{},
		tpHandler:       map[string]func(){},
		labels:          map[string]*ssa.LabelBuilder{},
		specialValues:   SpecialValue,
		specialTypes:    SpecialTypes,
		pkgNameCurrent:  "",
	}
	// log.Infof("ast: %s", ast.ToStringTree(ast.GetParser().GetRuleNames(), ast.GetParser()))
	astBuilder.build(ast)
	fmt.Printf("Program: %v done\n", astBuilder.pkgNameCurrent)
	return nil
}

func (*SSABuilder) FilterFile(path string) bool {
	return filepath.Ext(path) == ".c"
}

func (*SSABuilder) GetLanguage() consts.Language {
	return consts.C
}
func (s *SSABuilder) ParseAST(src string) (ssa.FrontAST, error) {
	return Frontend(src, s)
}

type astbuilder struct {
	*ssa.FunctionBuilder
	cmap           []map[string]struct{}
	importMap      map[string]*PackageInfo
	result         map[string][]string
	tpHandler      map[string]func()
	labels         map[string]*ssa.LabelBuilder
	specialValues  map[string]ssa.Value
	specialTypes   map[string]ssa.Type
	pkgNameCurrent string
	SetGlobal      bool
}

func PreprocessCMacros(src string) (string, error) {
	var preprocessorCmd string
	var preprocessorArgs []string

	/* TODO: 未来改进：
	1. 将 gcc/clang 集成到项目中（可选）
	2. 提供在不同平台上构建 gcc/clang 的脚本
	3. 添加编译选项，让用户决定是否自动扩展宏
	*/

	candidates := []string{"gcc", "clang", "cc"}

	for _, cmd := range candidates {
		if _, err := exec.LookPath(cmd); err == nil {
			preprocessorCmd = cmd
			break
		}
	}

	if preprocessorCmd == "" {
		return "", fmt.Errorf("C preprocessor not found: please install gcc, clang, or compatible C compiler (Platform: %s/%s)", runtime.GOOS, runtime.GOARCH)
	}

	tmpFile, err := os.CreateTemp("", "c_preprocess_*.c")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpFileName := tmpFile.Name()
	defer os.Remove(tmpFileName)

	if _, err := tmpFile.WriteString(src); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write source to temp file: %w", err)
	}
	tmpFile.Close()

	preprocessorArgs = []string{
		"-E",
		"-P",
		"-nostdinc",
		"-undef",
		"-Wno-everything",
		tmpFileName,
	}

	cmd := exec.Command(preprocessorCmd, preprocessorArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("preprocessor failed: %w\nOutput: %s", err, string(output))
	}

	result := string(output)
	lines := strings.Split(result, "\n")
	cleanedLines := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			cleanedLines = append(cleanedLines, line)
		} else if strings.HasPrefix(trimmed, "#include") {
			cleanedLines = append(cleanedLines, line)
		}
	}

	return strings.Join(cleanedLines, "\n"), nil
}

func Frontend(src string, ssabuilder ...*SSABuilder) (*cparser.CompilationUnitContext, error) {
	var builder *ssa.PreHandlerBase
	if len(ssabuilder) > 0 {
		builder = ssabuilder[0].PreHandlerBase
	}

	preprocessedSrc := src
	if preprocessed, err := PreprocessCMacros(src); err == nil {
		preprocessedSrc = preprocessed
	} else {
		log.Warnf("C macro preprocessing failed: %v, using original source", err)
	}

	errListener := antlr4util.NewErrorListener()
	lexer := cparser.NewCLexer(antlr.NewInputStream(preprocessedSrc))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := cparser.NewCParser(tokenStream)
	ssa.ParserSetAntlrCache(parser.BaseParser, builder)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.CompilationUnit().(*cparser.CompilationUnitContext)
	return ast, errListener.Error()
}

type PackageInfo struct {
	Name string
	Path string
	Pos  ssa.CanStartStopToken
}

func (b *astbuilder) SwitchFunctionBuilder(s *ssa.StoredFunctionBuilder) func() {
	t := b.StoreFunctionBuilder()
	b.LoadBuilder(s)
	return func() {
		b.LoadBuilder(t)
	}
}

func (b *astbuilder) LoadBuilder(s *ssa.StoredFunctionBuilder) {
	b.FunctionBuilder = s.Current
	b.LoadFunctionBuilder(s.Store)
}

func (b *astbuilder) GetStructAll() map[string]ssa.Type {
	objs := make(map[string]ssa.Type)
	for s, o := range b.GetProgram().ExportType {
		objs[s] = o
	}
	return objs
}

func (b *astbuilder) GetAliasAll() map[string]*ssa.AliasType {
	objs := make(map[string]*ssa.AliasType)
	for s, o := range b.GetProgram().ExportType {
		if o, ok := o.(*ssa.AliasType); ok {
			objs[s] = o
		}
	}
	return objs
}

func (b *astbuilder) GetGlobalVariables() map[string]ssa.Value {
	variables := make(map[string]ssa.Value)
	for i, m := range b.GetProgram().GlobalScope.GetAllMember() {
		variables[i.String()] = m
	}
	return variables
}

func (b *astbuilder) GetDefaultValue(ityp ssa.Type) ssa.Value {
	switch ityp.GetTypeKind() {
	case ssa.NumberTypeKind:
		return b.EmitConstInst(0)
	case ssa.StringTypeKind:
		return b.EmitConstInst("")
	case ssa.BooleanTypeKind:
		return b.EmitConstInst(false)
	case ssa.FunctionTypeKind:
		return b.EmitUndefined("func")
	case ssa.AliasTypeKind:
		alias, _ := ssa.ToAliasType(ityp)
		return b.GetDefaultValue(alias.GetType())
	case ssa.StructTypeKind, ssa.ObjectTypeKind, ssa.InterfaceTypeKind, ssa.SliceTypeKind, ssa.MapTypeKind:
		return b.EmitMakeBuildWithType(ityp, nil, nil)
	default:
		return b.EmitConstInst(0)
	}
}

func (b *astbuilder) addSpecialValue(n string, v ssa.Value) {
	if _, ok := b.specialValues[n]; !ok {
		b.specialValues[n] = v
	}
}

func (b *astbuilder) getSpecialValue(n string) (ssa.Value, bool) {
	if v, ok := b.specialValues[n]; ok {
		return v, true
	}
	return nil, false
}

func (b *astbuilder) GetLabelByName(name string) *ssa.LabelBuilder {
	if b.labels[name] == nil {
		b.labels[name] = b.BuildLabel(name)
	}

	return b.labels[name]
}

var cSideEffect = map[string][]uint{
	// Standard I/O functions with side effects
	"printf":   {0, uint(ssa.SideEffectIn)},                             // format string is input
	"sprintf":  {uint(ssa.SideEffectOut), 0, uint(ssa.SideEffectIn)},    // str output, format input
	"snprintf": {uint(ssa.SideEffectOut), 0, 0, uint(ssa.SideEffectIn)}, // str output, format input
	"scanf":    {0, uint(ssa.SideEffectOut)},                            // format string is input, args are output
	"sscanf":   {0, 0, uint(ssa.SideEffectOut)},                         // str input, format input, args are output
	"fprintf":  {0, 0, uint(ssa.SideEffectIn)},                          // stream input, format input
	"fscanf":   {0, 0, uint(ssa.SideEffectOut)},                         // stream input, format input, args are output
	"puts":     {0},                                                     // str input
	"gets":     {uint(ssa.SideEffectOut)},                               // str output
	"fgets":    {uint(ssa.SideEffectOut), 0, 0},                         // str output, n input, stream input
	"fputs":    {0, 0},                                                  // str input, stream input
	"fread":    {uint(ssa.SideEffectOut), 0, 0, 0},                      // ptr output, size input, count input, stream input
	"fwrite":   {0, 0, 0, 0},                                            // ptr input, size input, count input, stream input
	"fgetpos":  {0, uint(ssa.SideEffectOut)},                            // stream input, pos output
	"fsetpos":  {0, 0},                                                  // stream input, pos input
	"perror":   {0},                                                     // str input
	"strerror": {0},                                                     // errnum input

	// String manipulation functions with side effects
	"strlen":      {0},                             // str input
	"strcpy":      {uint(ssa.SideEffectOut), 0},    // dest output, src input
	"strncpy":     {uint(ssa.SideEffectOut), 0, 0}, // dest output, src input, n input
	"strcat":      {uint(ssa.SideEffectOut), 0},    // dest output, src input
	"strncat":     {uint(ssa.SideEffectOut), 0, 0}, // dest output, src input, n input
	"strcmp":      {0, 0},                          // str1 input, str2 input
	"strncmp":     {0, 0, 0},                       // str1 input, str2 input, n input
	"strcoll":     {0, 0},                          // str1 input, str2 input
	"strchr":      {0, 0},                          // str input, c input
	"strrchr":     {0, 0},                          // str input, c input
	"strstr":      {0, 0},                          // haystack input, needle input
	"strpbrk":     {0, 0},                          // str1 input, str2 input
	"strspn":      {0, 0},                          // str1 input, str2 input
	"strcspn":     {0, 0},                          // str1 input, str2 input
	"strtok":      {0, 0},                          // str input, delim input
	"strtok_r":    {0, 0, 0},                       // str input, delim input, saveptr input/output
	"memcpy":      {uint(ssa.SideEffectOut), 0, 0}, // dest output, src input, n input
	"memmove":     {uint(ssa.SideEffectOut), 0, 0}, // dest output, src input, n input
	"memcmp":      {0, 0, 0},                       // ptr1 input, ptr2 input, n input
	"memchr":      {0, 0, 0},                       // ptr input, c input, n input
	"memset":      {uint(ssa.SideEffectOut), 0, 0}, // ptr output, c input, n input
	"strdup":      {0},                             // str input
	"strndup":     {0, 0},                          // str input, n input
	"strcasecmp":  {0, 0},                          // str1 input, str2 input
	"strncasecmp": {0, 0, 0},                       // str1 input, str2 input, n input

	// Memory management with side effects
	"malloc":   {uint(ssa.SideEffectOut)},    // size input, returns pointer
	"calloc":   {uint(ssa.SideEffectOut), 0}, // num input, size input, returns pointer
	"realloc":  {0, 0},                       // ptr input, size input, returns pointer (SideEffectOut for return value)
	"free":     {0},                          // ptr input
	"memalign": {uint(ssa.SideEffectOut), 0}, // alignment input, size input, returns pointer
	"valloc":   {uint(ssa.SideEffectOut)},    // size input, returns pointer

	// Mathematical functions with side effects
	"abs":      {0},                             // x input
	"labs":     {0},                             // x input
	"llabs":    {0},                             // x input
	"div":      {0, 0},                          // numer input, denom input
	"ldiv":     {0, 0},                          // numer input, denom input
	"lldiv":    {0, 0},                          // numer input, denom input
	"rand":     {},                              // no parameters
	"srand":    {0},                             // seed input
	"atoi":     {0},                             // str input
	"atol":     {0},                             // str input
	"atoll":    {0},                             // str input
	"atof":     {0},                             // str input
	"strtol":   {0, uint(ssa.SideEffectOut), 0}, // str input, endptr output, base input
	"strtoll":  {0, uint(ssa.SideEffectOut), 0}, // str input, endptr output, base input
	"strtoul":  {0, uint(ssa.SideEffectOut), 0}, // str input, endptr output, base input
	"strtoull": {0, uint(ssa.SideEffectOut), 0}, // str input, endptr output, base input
	"strtod":   {0, uint(ssa.SideEffectOut)},    // str input, endptr output
	"strtof":   {0, uint(ssa.SideEffectOut)},    // str input, endptr output
	"strtold":  {0, uint(ssa.SideEffectOut)},    // str input, endptr output

	// System and process control with side effects
	"exit":          {0},                             // status input
	"abort":         {},                              // no parameters
	"atexit":        {0},                             // handler input
	"at_quick_exit": {0},                             // handler input
	"quick_exit":    {0},                             // status input
	"getenv":        {0},                             // name input
	"setenv":        {0, 0, 0},                       // name input, value input, overwrite input
	"unsetenv":      {0},                             // name input
	"putenv":        {0},                             // str input
	"system":        {0},                             // command input
	"execv":         {0, 0},                          // path input, argv input
	"execvp":        {0, 0},                          // file input, argv input
	"execve":        {0, 0, 0},                       // path input, argv input, envp input
	"fork":          {},                              // no parameters
	"wait":          {uint(ssa.SideEffectOut)},       // status output
	"waitpid":       {0, uint(ssa.SideEffectOut), 0}, // pid input, status output, options input

	// File and directory operations with side effects
	"open":      {0, 0, 0},                       // pathname input, flags input, mode input
	"close":     {0},                             // fd input
	"read":      {0, uint(ssa.SideEffectOut), 0}, // fd input, buf output, count input
	"write":     {0, 0, 0},                       // fd input, buf input, count input
	"lseek":     {0, 0, 0},                       // fd input, offset input, whence input
	"dup":       {0},                             // oldfd input
	"dup2":      {0, 0},                          // oldfd input, newfd input
	"pipe":      {uint(ssa.SideEffectOut)},       // pipefd output
	"chmod":     {0, 0},                          // pathname input, mode input
	"fchmod":    {0, 0},                          // fd input, mode input
	"chown":     {0, 0, 0},                       // pathname input, owner input, group input
	"fchown":    {0, 0, 0},                       // fd input, owner input, group input
	"lchown":    {0, 0, 0},                       // pathname input, owner input, group input
	"link":      {0, 0},                          // oldpath input, newpath input
	"unlink":    {0},                             // pathname input
	"symlink":   {0, 0},                          // target input, linkpath input
	"readlink":  {0, uint(ssa.SideEffectOut), 0}, // pathname input, buf output, bufsiz input
	"mkdir":     {0, 0},                          // pathname input, mode input
	"rmdir":     {0},                             // pathname input
	"opendir":   {0},                             // name input
	"readdir":   {0},                             // dirp input
	"closedir":  {0},                             // dirp input
	"rewinddir": {0},                             // dirp input
	"telldir":   {0},                             // dirp input
	"seekdir":   {0, 0},                          // dirp input, loc input
	"stat":      {0, uint(ssa.SideEffectOut)},    // pathname input, statbuf output
	"fstat":     {0, uint(ssa.SideEffectOut)},    // fd input, statbuf output
	"lstat":     {0, uint(ssa.SideEffectOut)},    // pathname input, statbuf output
	"access":    {0, 0},                          // pathname input, mode input
	"utime":     {0, 0},                          // filename input, times input
	"utimes":    {0, 0},                          // filename input, times input

	// Time functions with side effects
	"time":      {uint(ssa.SideEffectOut)},          // tloc output
	"ctime":     {0},                                // time input
	"asctime":   {0},                                // timeptr input
	"gmtime":    {0},                                // timer input
	"localtime": {0},                                // timer input
	"mktime":    {0},                                // timeptr input
	"strftime":  {uint(ssa.SideEffectOut), 0, 0, 0}, // s output, maxsize input, format input, timeptr input
	"clock":     {},                                 // no parameters
	"difftime":  {0, 0},                             // time1 input, time0 input

	// Signal handling with side effects
	"signal": {0, 0}, // signum input, handler input
	"raise":  {0},    // signum input
	"kill":   {0, 0}, // pid input, sig input
	"alarm":  {0},    // seconds input
	"pause":  {},     // no parameters
	"sleep":  {0},    // seconds input
	"usleep": {0},    // useconds input

	// Variadic functions with side effects
	"va_start": {0, 0}, // ap input, param input
	"va_arg":   {0, 0}, // ap input, typeSize input
	"va_end":   {0},    // ap input
	"va_copy":  {0, 0}, // dest input, src input
}

// cBuildIn defines common C standard library functions with proper pointer types
var cBuildIn = map[string]any{
	// Standard I/O functions
	"printf":   func(format *byte, args ...any) int { return 0 },
	"sprintf":  func(str *byte, format *byte, args ...any) int { return 0 },
	"snprintf": func(str *byte, size int, format *byte, args ...any) int { return 0 },
	"scanf":    func(format *byte, args ...any) int { return 0 },
	"sscanf":   func(str *byte, format *byte, args ...any) int { return 0 },
	"fprintf":  func(stream *any, format *byte, args ...any) int { return 0 },
	"fscanf":   func(stream *any, format *byte, args ...any) int { return 0 },
	"puts":     func(str *byte) int { return 0 },
	"putchar":  func(c int) int { return 0 },
	"getchar":  func() int { return 0 },
	"gets":     func(str *byte) *byte { return nil },
	"fgets":    func(str *byte, n int, stream *any) *byte { return nil },
	"fputs":    func(str *byte, stream *any) int { return 0 },
	"putc":     func(c int, stream *any) int { return 0 },
	"getc":     func(stream *any) int { return 0 },
	"fgetc":    func(stream *any) int { return 0 },
	"fputc":    func(c int, stream *any) int { return 0 },
	"ungetc":   func(c int, stream *any) int { return 0 },
	"fread":    func(ptr *any, size int, count int, stream *any) int { return 0 },
	"fwrite":   func(ptr *any, size int, count int, stream *any) int { return 0 },
	"fseek":    func(stream *any, offset int, origin int) int { return 0 },
	"ftell":    func(stream *any) int { return 0 },
	"rewind":   func(stream *any) {},
	"fgetpos":  func(stream *any, pos *any) int { return 0 },
	"fsetpos":  func(stream *any, pos *any) int { return 0 },
	"clearerr": func(stream *any) {},
	"feof":     func(stream *any) int { return 0 },
	"ferror":   func(stream *any) int { return 0 },
	"perror":   func(str *byte) {},
	"remove":   func(filename *byte) int { return 0 },
	"rename":   func(oldname *byte, newname *byte) int { return 0 },
	"tmpfile":  func() *any { return nil },
	"tmpnam":   func(str *byte) *byte { return nil },
	"fopen":    func(filename *byte, mode *byte) *any { return nil },
	"freopen":  func(filename *byte, mode *byte, stream *any) *any { return nil },
	"fclose":   func(stream *any) int { return 0 },
	"fflush":   func(stream *any) int { return 0 },

	// String manipulation functions
	"strlen":      func(str *byte) int { return 0 },
	"strcpy":      func(dest *byte, src *byte) *byte { return nil },
	"strncpy":     func(dest *byte, src *byte, n int) *byte { return nil },
	"strcat":      func(dest *byte, src *byte) *byte { return nil },
	"strncat":     func(dest *byte, src *byte, n int) *byte { return nil },
	"strcmp":      func(str1 *byte, str2 *byte) int { return 0 },
	"strncmp":     func(str1 *byte, str2 *byte, n int) int { return 0 },
	"strcoll":     func(str1 *byte, str2 *byte) int { return 0 },
	"strchr":      func(str *byte, c int) *byte { return nil },
	"strrchr":     func(str *byte, c int) *byte { return nil },
	"strstr":      func(haystack *byte, needle *byte) *byte { return nil },
	"strpbrk":     func(str1 *byte, str2 *byte) *byte { return nil },
	"strspn":      func(str1 *byte, str2 *byte) int { return 0 },
	"strcspn":     func(str1 *byte, str2 *byte) int { return 0 },
	"strtok":      func(str *byte, delim *byte) *byte { return nil },
	"strtok_r":    func(str *byte, delim *byte, saveptr **byte) *byte { return nil },
	"strerror":    func(errnum int) *byte { return nil },
	"memcpy":      func(dest *any, src *any, n int) *any { return nil },
	"memmove":     func(dest *any, src *any, n int) *any { return nil },
	"memcmp":      func(ptr1 *any, ptr2 *any, n int) int { return 0 },
	"memchr":      func(ptr *any, c int, n int) *any { return nil },
	"memset":      func(ptr *any, c int, n int) *any { return nil },
	"strdup":      func(str *byte) *byte { return nil },
	"strndup":     func(str *byte, n int) *byte { return nil },
	"strcasecmp":  func(str1 *byte, str2 *byte) int { return 0 },
	"strncasecmp": func(str1 *byte, str2 *byte, n int) int { return 0 },

	// Character classification and conversion
	"isalnum":  func(c int) int { return 0 },
	"isalpha":  func(c int) int { return 0 },
	"iscntrl":  func(c int) int { return 0 },
	"isdigit":  func(c int) int { return 0 },
	"isgraph":  func(c int) int { return 0 },
	"islower":  func(c int) int { return 0 },
	"isprint":  func(c int) int { return 0 },
	"ispunct":  func(c int) int { return 0 },
	"isspace":  func(c int) int { return 0 },
	"isupper":  func(c int) int { return 0 },
	"isxdigit": func(c int) int { return 0 },
	"tolower":  func(c int) int { return 0 },
	"toupper":  func(c int) int { return 0 },

	// Memory management
	"malloc":   func(size int) *any { return nil },
	"calloc":   func(num int, size int) *any { return nil },
	"realloc":  func(ptr *any, size int) *any { return nil },
	"free":     func(ptr *any) {},
	"memalign": func(alignment int, size int) *any { return nil },
	"valloc":   func(size int) *any { return nil },

	// Mathematical functions
	"abs":      func(x int) int { return 0 },
	"labs":     func(x int) int { return 0 },
	"llabs":    func(x int) int { return 0 },
	"div":      func(numer int, denom int) any { return nil },
	"ldiv":     func(numer int, denom int) any { return nil },
	"lldiv":    func(numer int, denom int) any { return nil },
	"rand":     func() int { return 0 },
	"srand":    func(seed int) {},
	"atoi":     func(str *byte) int { return 0 },
	"atol":     func(str *byte) int { return 0 },
	"atoll":    func(str *byte) int { return 0 },
	"atof":     func(str *byte) float64 { return 0 },
	"strtol":   func(str *byte, endptr **byte, base int) int { return 0 },
	"strtoll":  func(str *byte, endptr **byte, base int) int { return 0 },
	"strtoul":  func(str *byte, endptr **byte, base int) int { return 0 },
	"strtoull": func(str *byte, endptr **byte, base int) int { return 0 },
	"strtod":   func(str *byte, endptr **byte) float64 { return 0 },
	"strtof":   func(str *byte, endptr **byte) float32 { return 0 },
	"strtold":  func(str *byte, endptr **byte) float64 { return 0 },

	// System and process control
	"exit":          func(status int) {},
	"abort":         func() {},
	"atexit":        func(handler func()) int { return 0 },
	"at_quick_exit": func(handler func()) int { return 0 },
	"quick_exit":    func(status int) {},
	"getenv":        func(name *byte) *byte { return nil },
	"setenv":        func(name *byte, value *byte, overwrite int) int { return 0 },
	"unsetenv":      func(name *byte) int { return 0 },
	"putenv":        func(str *byte) int { return 0 },
	"system":        func(command *byte) int { return 0 },
	"execv":         func(path *byte, argv **byte) int { return 0 },
	"execvp":        func(file *byte, argv **byte) int { return 0 },
	"execve":        func(path *byte, argv **byte, envp **byte) int { return 0 },
	"fork":          func() int { return 0 },
	"wait":          func(status *int) int { return 0 },
	"waitpid":       func(pid int, status *int, options int) int { return 0 },

	// File and directory operations
	"open":      func(pathname *byte, flags int, mode int) int { return 0 },
	"close":     func(fd int) int { return 0 },
	"read":      func(fd int, buf *any, count int) int { return 0 },
	"write":     func(fd int, buf *any, count int) int { return 0 },
	"lseek":     func(fd int, offset int, whence int) int { return 0 },
	"dup":       func(oldfd int) int { return 0 },
	"dup2":      func(oldfd int, newfd int) int { return 0 },
	"pipe":      func(pipefd *int) int { return 0 },
	"chmod":     func(pathname *byte, mode int) int { return 0 },
	"fchmod":    func(fd int, mode int) int { return 0 },
	"chown":     func(pathname *byte, owner int, group int) int { return 0 },
	"fchown":    func(fd int, owner int, group int) int { return 0 },
	"lchown":    func(pathname *byte, owner int, group int) int { return 0 },
	"link":      func(oldpath *byte, newpath *byte) int { return 0 },
	"unlink":    func(pathname *byte) int { return 0 },
	"symlink":   func(target *byte, linkpath *byte) int { return 0 },
	"readlink":  func(pathname *byte, buf *byte, bufsiz int) int { return 0 },
	"mkdir":     func(pathname *byte, mode int) int { return 0 },
	"rmdir":     func(pathname *byte) int { return 0 },
	"opendir":   func(name *byte) *any { return nil },
	"readdir":   func(dirp *any) *any { return nil },
	"closedir":  func(dirp *any) int { return 0 },
	"rewinddir": func(dirp *any) {},
	"telldir":   func(dirp *any) int { return 0 },
	"seekdir":   func(dirp *any, loc int) {},
	"stat":      func(pathname *byte, statbuf *any) int { return 0 },
	"fstat":     func(fd int, statbuf *any) int { return 0 },
	"lstat":     func(pathname *byte, statbuf *any) int { return 0 },
	"access":    func(pathname *byte, mode int) int { return 0 },
	"utime":     func(filename *byte, times *any) int { return 0 },
	"utimes":    func(filename *byte, times *any) int { return 0 },

	// Time functions
	"time":      func(tloc *int) int { return 0 },
	"ctime":     func(time *int) *byte { return nil },
	"asctime":   func(timeptr *any) *byte { return nil },
	"gmtime":    func(timer *int) *any { return nil },
	"localtime": func(timer *int) *any { return nil },
	"mktime":    func(timeptr *any) int { return 0 },
	"strftime":  func(s *byte, maxsize int, format *byte, timeptr *any) int { return 0 },
	"clock":     func() int { return 0 },
	"difftime":  func(time1 int, time0 int) float64 { return 0 },

	// Signal handling
	"signal": func(signum int, handler func(int)) func(int) { return nil },
	"raise":  func(signum int) int { return 0 },
	"kill":   func(pid int, sig int) int { return 0 },
	"alarm":  func(seconds int) int { return 0 },
	"pause":  func() int { return 0 },
	"sleep":  func(seconds int) int { return 0 },
	"usleep": func(useconds int) int { return 0 },

	// Error handling
	"errno": 0,

	// Variadic functions
	"va_start": func(ap *any, param int) {},
	"va_arg":   func(ap *any, typeSize int) any { return nil },
	"va_end":   func(ap *any) {},
	"va_copy":  func(dest *any, src *any) {},

	// Common macros and constants
	"NULL":         nil,
	"EOF":          -1,
	"BUFSIZ":       8192,
	"FOPEN_MAX":    16,
	"FILENAME_MAX": 256,
	"L_tmpnam":     20,
	"SEEK_SET":     0,
	"SEEK_CUR":     1,
	"SEEK_END":     2,
	"TMP_MAX":      238328,

	// Custom functions for testing
	"println": func(args ...any) {},
	"print":   func(args ...any) {},
}

var cBuildInSE = map[string]any{}
