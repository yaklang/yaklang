package java2ssa

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

// expandedZipFSCache 缓存 ExpandedZipFS 实例，避免重复创建和浪费缓存
var expandedZipFSCache = utils.NewSafeMapWithKey[*filesys.ZipFS, *javaclassparser.ExpandedZipFS]()

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

// extractZipFSFromUnderlying 从文件系统中提取底层的 ZipFS
// 支持递归提取，处理 HookFS 包装的情况
func extractZipFSFromUnderlying(underlying fi.FileSystem) *filesys.ZipFS {
	if zfs, ok := underlying.(*filesys.ZipFS); ok {
		return zfs
	}
	if jarFS, ok := underlying.(*javaclassparser.JarFS); ok {
		return jarFS.ZipFS
	}
	// 如果是 HookFS，使用反射递归检查 underlying
	if hookFS, ok := underlying.(*filesys.HookFS); ok {
		// 使用反射访问私有字段
		v := reflect.ValueOf(hookFS).Elem()
		underlyingField := v.FieldByName("underlying")
		if underlyingField.IsValid() && underlyingField.CanInterface() {
			if underlyingFS, ok := underlyingField.Interface().(fi.FileSystem); ok {
				return extractZipFSFromUnderlying(underlyingFS)
			}
		}
	}
	return nil
}

func (s *SSABuilder) WrapWithPreprocessedFS(fs fi.FileSystem) fi.FileSystem {
	// 文件系统可能有两种类型：
	// 1. CodeSourceCompression: ZipFS (压缩文件，.zip 后缀)
	// 2. CodeSourceJar: UnifiedFS -> JarFS -> ZipFS (JAR 文件，.jar 后缀，已配置扩展名映射 .class -> .java)
	//
	// 如果文件系统包含嵌套的 JAR/ZIP 文件，使用 ExpandedZipFS 来展开：
	// - ExpandedZipFS 将 JAR/ZIP 文件视为目录，自动展开其内容
	// - 实现了完整的嵌套归档处理（ReadDir、Stat、ReadFile）
	// - 支持多层嵌套（ZIP 中包含 JAR，JAR 中包含 ZIP 等）
	//
	// 提取底层的 ZipFS
	zipFS := extractZipFSFromUnderlying(fs)
	if zipFS == nil {
		// 如果没有 ZipFS，直接返回原文件系统
		return fs
	}

	// 检查是否有嵌套的归档文件
	var hasNestedArchive bool
	filesys.Recursive(".", filesys.WithFileSystem(fs), filesys.WithFileStat(func(path string, info os.FileInfo) error {
		if !info.IsDir() && (strings.HasSuffix(strings.ToLower(path), ".jar") ||
			strings.HasSuffix(strings.ToLower(path), ".war") ||
			strings.HasSuffix(strings.ToLower(path), ".zip")) {
			hasNestedArchive = true
		}
		return nil
	}))

	if !hasNestedArchive {
		return fs
	}

	expandedFS := expandedZipFSCache.GetOrLoad(zipFS, func() *javaclassparser.ExpandedZipFS {
		return javaclassparser.NewExpandedZipFS(fs, zipFS)
	})

	// 如果原文件系统已经是 HookFS，保持 HookFS 包装以保留原有的 hook
	if _, ok := fs.(*filesys.HookFS); ok {
		return filesys.NewHookFS(expandedFS)
	}

	return expandedFS
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
