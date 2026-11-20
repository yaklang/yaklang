package cpp2ssa

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/cfamily/preprocess"
	cppparser "github.com/yaklang/yaklang/common/yak/cpp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// SSABuilder is the C++ language builder. It shares the C macro/include
// preprocessor (common/yak/cfamily/preprocess) and the SSA core with c2ssa, and
// keeps its own grammar and visitor.
type SSABuilder struct {
	*ssa.PreHandlerBase
}

var _ ssa.Builder = (*SSABuilder)(nil)

// compiledExtensions are the file kinds compiled as translation units. Headers
// are not translation units: their declarations enter the SSA through the
// #include inlining done by the preprocessor, which avoids emitting every
// header twice.
var compiledExtensions = map[string]bool{
	".cpp": true,
	".cc":  true,
	".cxx": true,
}

// headerExtensions belong to C++ for detection and rule scoping. They are
// readable through #include but are not compiled on their own.
var headerExtensions = map[string]bool{
	".hpp": true,
	".hxx": true,
	".hh":  true,
}

func CreateBuilder() ssa.Builder {
	builder := &SSABuilder{PreHandlerBase: ssa.NewPreHandlerBase()}
	builder.WithLanguageConfigOpts(
		ssa.WithLanguageConfigBind(true),
		ssa.WithLanguageConfigVirtualImport(true),
		// C++ class instances keep the blueprint as their type, so member
		// lookup and virtual/member call binding see GetNormalMethod.
		ssa.WithLanguageConfigSupportClass(true),
		ssa.WithLanguageBuilder(builder),
	)
	return builder
}

func (s *SSABuilder) GetLanguage() ssaconfig.Language {
	return ssaconfig.CPP
}

func (s *SSABuilder) FilterFile(path string) bool {
	return compiledExtensions[strings.ToLower(filepath.Ext(path))]
}

func (s *SSABuilder) FilterParseAST(path string) bool {
	return s.FilterFile(path)
}

func (s *SSABuilder) FilterPreHandlerFile(path string) bool {
	return s.FilterFile(path)
}

// IsCppHeaderExtension reports whether path is a C++ header by extension. The
// .h extension is not included: it stays owned by C, and a C++ translation
// unit still sees its declarations through #include inlining.
func IsCppHeaderExtension(path string) bool {
	return headerExtensions[strings.ToLower(filepath.Ext(path))]
}

// DefaultPreprocessConfig is the C++ preprocessing profile. System includes are
// dropped instead of failing, project includes are inlined, and no external C
// header pack is fetched.
func DefaultPreprocessConfig() preprocess.PreprocessConfig {
	config := preprocess.PreprocessConfig{
		SkipSystemIncludes:          true,
		ResolveProjectAngleIncludes: true,
		HeaderExtensions:            []string{".h", ".hpp", ".hxx", ".hh", ".inl", ".ipp", ".tpp"},
		InlineProjectHeaders:        true,
		MaxIncludeDepth:             64,
		ExternalIncludeDirs:         []string{},
		Defines: map[string]string{
			"__cplusplus": "201402L",
		},
	}
	return config
}

func newCppPreprocessFS(underlying fi.FileSystem) fi.FileSystem {
	project := preprocess.BuildProject(underlying, DefaultPreprocessConfig())
	hookFS := filesys.NewHookFS(underlying)
	hookFS.AddReadHook(&filesys.ReadHook{
		Matcher: filesys.SuffixMatcher(".cpp", ".cc", ".cxx"),
		AfterRead: func(ctx *filesys.ReadHookContext, data []byte) ([]byte, error) {
			out, err := project.PreprocessTU(ctx.Name, string(data))
			if err != nil {
				log.Warnf("C++ preprocess failed for %s: %v, using original source", ctx.Name, err)
				return data, nil
			}
			return []byte(out), nil
		},
	})
	return hookFS
}

func (s *SSABuilder) WrapWithPreprocessedFS(fs fi.FileSystem, _ bool) fi.FileSystem {
	return newCppPreprocessFS(fs)
}

func (s *SSABuilder) GetAntlrCache() *ssa.AntlrCache {
	return s.CreateAntlrCache(cppparser.GetCPP14LexerSerializedATN(), cppparser.GetCPP14ParserSerializedATN())
}

func (s *SSABuilder) ParseAST(src string, cache *ssa.AntlrCache) (ssa.FrontAST, error) {
	return Frontend(src, cache)
}

func (s *SSABuilder) BuildFromAST(raw ssa.FrontAST, builder *ssa.FunctionBuilder) error {
	ast, ok := raw.(cppparser.ITranslationUnitContext)
	if !ok || ast == nil {
		return utils.Errorf("invalid C++ AST type: %T", raw)
	}
	b := &singleFileBuilder{
		FunctionBuilder: builder,
		typeAliases:     make(map[string]string),
		templateTypes:   make(map[string]ssa.Type),
		usingNamespaces: make([]string, 0),
	}
	builder.WithExternValue(cppBuildIn)
	b.visitTranslationUnit(ast)
	return nil
}

// cppBuildIn lists the names the C++ frontend resolves without a declaration.
// println/print are the test helpers the C builder also provides, so SSA tests
// can assert values with ssatest.CheckPrintlnValue. Standard-library types and
// members come from the blueprint placeholders in stdlib.go.
var cppBuildIn = map[string]any{
	"println": func(args ...any) {},
	"print":   func(args ...any) {},
}

// singleFileBuilder carries the C++-only scopes: namespace path, the class body
// being visited, template parameters, using-declarations and typedef names.
type singleFileBuilder struct {
	*ssa.FunctionBuilder

	namespace       []string
	class           *ssa.Blueprint
	typeAliases     map[string]string
	templateTypes   map[string]ssa.Type
	usingNamespaces []string
}

func (b *singleFileBuilder) qualifiedName(name string) string {
	if len(b.namespace) == 0 {
		return name
	}
	return strings.Join(b.namespace, "::") + "::" + name
}
