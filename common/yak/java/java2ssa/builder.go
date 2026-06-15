package java2ssa

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var INNER_CLASS_SPLIT = "$"
var javaRecordPatternCaseRegexp = regexp.MustCompile(`(?m)(case\s+[A-Za-z_][\w$.<>]*\s*)\(\s*var\s+([A-Za-z_]\w*)(?:\s*,[\s\S]*?)?\)\s*(->)`)

// ========================================== For SSAAPI ==========================================

type SSABuilder struct {
	*ssa.PreHandlerBase
}

var _ ssa.Builder = (*SSABuilder)(nil)

func CreateBuilder() ssa.Builder {
	builder := &SSABuilder{
		PreHandlerBase: ssa.NewPreHandlerBase(),
	}
	builder.WithLanguageConfigOpts(
		ssa.WithLanguageConfigBind(true),
		ssa.WithLanguageConfigSupportClass(true),
		ssa.WithLanguageConfigIsSupportClassStaticModifier(true),
		ssa.WithLanguageConfigVirtualImport(true),
		ssa.WithLanguageBuilder(builder),
	)
	return builder
}

func (s *SSABuilder) FilterParseAST(path string) bool {
	extension := filepath.Ext(path)
	return extension == ".java"
}

func (s *SSABuilder) GetAntlrCache() *ssa.AntlrCache {
	return s.CreateAntlrCache(javaparser.GetJavaLexerSerializedATN(), javaparser.GetJavaParserSerializedATN())
}

func (s *SSABuilder) ParseAST(src string, cache *ssa.AntlrCache) (ssa.FrontAST, error) {
	return Frontend(src, cache)
}

func (*SSABuilder) BuildFromAST(raw ssa.FrontAST, b *ssa.FunctionBuilder) error {
	ast, ok := raw.(javaparser.ICompilationUnitContext)
	if !ok {
		return utils.Errorf("invalid AST type: %T, expected javaparser.ICompilationUnitContext", raw)
	}
	build := &singleFileBuilder{
		FunctionBuilder:   b,
		constMap:          make(map[string]ssa.Value),
		fullTypeNameMap:   make(map[string][]string),
		allImportPkgSlice: make([][]string, 0),
		selfPkgPath:       make([]string, 0),
	}
	build.initImport()
	build.VisitCompilationUnit(ast)
	return nil
}

func (s *SSABuilder) WrapWithPreprocessedFS(fs fi.FileSystem, jarRecursiveParse bool) fi.FileSystem {
	if fs == nil || !jarRecursiveParse {
		return fs
	}
	if _, ok := fs.(*javaclassparser.ExpandedZipFS); ok {
		return fs
	}

	wrapped := javaclassparser.MaybeWrapExpandedArchiveFS(fs, jarRecursiveParse)
	if wrapped == fs {
		return fs
	}
	// Jar/zip roots from parseFSFromInfo may already be UnifiedFS(.class -> .java).
	if _, ok := fs.(*filesys.UnifiedFS); !ok {
		wrapped = filesys.NewUnifiedFS(wrapped,
			filesys.WithUnifiedFsExtMap(".class", ".java"),
		)
	}
	return wrapped
}

func (*SSABuilder) FilterFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".java"
}

func (*SSABuilder) GetLanguage() ssaconfig.Language {
	return ssaconfig.JAVA
}

// ========================================== Build Front End ==========================================

type singleFileBuilder struct {
	*ssa.FunctionBuilder
	constMap map[string]ssa.Value

	// for full type name
	fullTypeNameMap   map[string][]string
	allImportPkgSlice [][]string
	selfPkgPath       []string

	// framework support for spring boot
	currentUIModel ssa.Value
	isInController bool
}

func Frontend(src string, caches ...*ssa.AntlrCache) (javaparser.ICompilationUnitContext, error) {
	var cache *ssa.AntlrCache
	if len(caches) > 0 {
		cache = caches[0]
	}
	src = preprocessJavaUnicodeEscapes(src)
	src = preprocessJavaRecordPatternSwitchCases(src)
	src = normalizeDecompiledJava(src)
	return antlr4util.ParseASTWithSLLFirst(
		src,
		javaparser.NewJavaLexer,
		javaparser.NewJavaParser,
		nil,
		func(lexer *javaparser.JavaLexer, parser *javaparser.JavaParser) {
			ssa.ParserSetAntlrCache(parser, lexer, cache)
		},
		func(parser *javaparser.JavaParser) javaparser.ICompilationUnitContext {
			return parser.CompilationUnit()
		},
	)
}

func preprocessJavaRecordPatternSwitchCases(src string) string {
	if !strings.Contains(src, "case ") || !strings.Contains(src, "->") || !strings.Contains(src, "(var ") {
		return src
	}
	return javaRecordPatternCaseRegexp.ReplaceAllString(src, `$1$2 $3`)
}

func preprocessJavaUnicodeEscapes(src string) string {
	if !strings.Contains(src, `\u`) {
		return src
	}

	var out strings.Builder
	out.Grow(len(src))
	backslashCount := 0

	for i := 0; i < len(src); i++ {
		if src[i] != '\\' {
			out.WriteByte(src[i])
			backslashCount = 0
			continue
		}

		if i+1 >= len(src) || src[i+1] != 'u' || backslashCount%2 != 0 {
			out.WriteByte(src[i])
			backslashCount++
			continue
		}

		j := i + 1
		for j < len(src) && src[j] == 'u' {
			j++
		}
		if j+4 > len(src) {
			out.WriteByte(src[i])
			continue
		}

		hex := src[j : j+4]
		code, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			out.WriteByte(src[i])
			backslashCount++
			continue
		}

		var buf [utf8.UTFMax]byte
		n := utf8.EncodeRune(buf[:], rune(code))
		out.Write(buf[:n])
		if rune(code) == '\\' {
			backslashCount = 1
		} else {
			backslashCount = 0
		}
		i = j + 3
	}

	return out.String()
}

func (b *singleFileBuilder) AssignConst(name string, value ssa.Value) bool {
	if ConstValue, ok := b.constMap[name]; ok {
		log.Warnf("const %v has been defined value is %v", name, ConstValue.String())
		return false
	}

	b.constMap[name] = value
	return true
}

func (b *singleFileBuilder) ReadConst(name string) (ssa.Value, bool) {
	v, ok := b.constMap[name]
	return v, ok
}

func (b *singleFileBuilder) AssignClassConst(className, key string, value ssa.Value) {
	name := fmt.Sprintf("%s_%s", className, key)
	b.AssignConst(name, value)
}
func (b *singleFileBuilder) ReadClassConst(className, key string) (ssa.Value, bool) {
	name := fmt.Sprintf("%s_%s", className, key)
	return b.ReadConst(name)
}

func (b *singleFileBuilder) SwitchFunctionBuilder(s *ssa.StoredFunctionBuilder) func() {
	t := b.StoreFunctionBuilder()
	b.LoadBuilder(s)
	return func() {
		b.LoadBuilder(t)
	}
}

func (b *singleFileBuilder) LoadBuilder(s *ssa.StoredFunctionBuilder) {
	b.FunctionBuilder = s.Current
	b.LoadFunctionBuilder(s.Store)
}

func (b *singleFileBuilder) initImport() {
	b.allImportPkgSlice = append(b.allImportPkgSlice, []string{"java", "lang", "*"})
}
