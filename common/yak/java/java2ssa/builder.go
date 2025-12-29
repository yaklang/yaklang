package java2ssa

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
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

func extractZipFSFromUnderlying(underlying fi.FileSystem) *filesys.ZipFS {
	if zfs, ok := underlying.(*filesys.ZipFS); ok {
		return zfs
	}
	if jarFS, ok := underlying.(*javaclassparser.JarFS); ok {
		return jarFS.ZipFS
	}
	return nil
}

func handleNestedJarHook() *filesys.ReadHook {
	return &filesys.ReadHook{
		Matcher: filesys.CustomMatcher(func(name string) bool {
			return strings.Contains(name, ".jar/") || strings.Contains(name, ".jar!")
		}),
		AfterRead: func(ctx *filesys.ReadHookContext, data []byte) ([]byte, error) {
			if len(data) > 0 {
				return data, nil
			}

			targetZipFS := extractZipFSFromUnderlying(ctx.Underlying)
			if targetZipFS == nil {
				return data, nil
			}

			helper := javaclassparser.NewZipJarHelper(targetZipFS)
			jarPath, internalPath, isJar := helper.ParseJarPath(ctx.Name)
			if !isJar {
				return data, nil
			}

			jarData, err := helper.ReadFileFromJar(jarPath, internalPath)
			if err != nil {
				return data, nil
			}

			return jarData, nil
		},
	}
}

func (s *SSABuilder) WrapWithPreprocessedFS(fs fi.FileSystem) fi.FileSystem {
	// HookFS 可能包装了两种类型：
	// 1. CodeSourceCompression: ZipFS -> HookFS (压缩文件)
	// 2. CodeSourceJar: ZipFS -> JarFS -> UnifiedFS -> HookFS (JAR 文件，已配置扩展名映射)
	// 两种情况都需要处理嵌套 JAR，通过 hook 机制统一处理

	var hasNestedJar bool
	filesys.Recursive(".", filesys.WithFileSystem(fs), filesys.WithFileStat(func(path string, info os.FileInfo) error {
		if !info.IsDir() && (strings.HasSuffix(strings.ToLower(path), ".jar") ||
			strings.HasSuffix(strings.ToLower(path), ".war")) {
			hasNestedJar = true
		}
		return nil
	}))

	if !hasNestedJar {
		return fs
	}

	// 如果已经是 HookFS，直接添加嵌套 JAR 处理的 hook
	// hook 中会通过 ReadHookContext.Underlying 访问底层文件系统（ZipFS 或 UnifiedFS）
	if hookFS, ok := fs.(*filesys.HookFS); ok {
		hookFS.AddReadHook(handleNestedJarHook())
		return hookFS
	}

	// 否则创建新的 HookFS 来处理嵌套 JAR
	newHookFS := filesys.NewHookFS(fs)
	newHookFS.AddReadHook(handleNestedJarHook())
	return newHookFS
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
	errListener := antlr4util.NewErrorListener()
	lexer := javaparser.NewJavaLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := javaparser.NewJavaParser(tokenStream)
	ssa.ParserSetAntlrCache(parser, lexer, cache)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.CompilationUnit()
	return ast, errListener.Error()
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
