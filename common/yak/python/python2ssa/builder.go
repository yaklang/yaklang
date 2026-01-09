package python2ssa

import (
	"path/filepath"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	pythonparser "github.com/yaklang/yaklang/common/yak/python/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// ========================================== For SSAAPI ==========================================

// SSABuilder implements the ssa.Builder interface for Python code.
// It provides methods to parse Python source code and build SSA representation.
// Similar to java2ssa.SSABuilder, this builder handles Python-specific parsing and SSA construction.
type SSABuilder struct {
	*ssa.PreHandlerBase
}

var _ ssa.Builder = (*SSABuilder)(nil)

// CreateBuilder creates a new SSABuilder instance for Python.
// This function is used by the SSA API to create language-specific builders.
// Returns a builder configured for Python language features.
func CreateBuilder() ssa.Builder {
	builder := &SSABuilder{
		PreHandlerBase: ssa.NewPreHandlerBase(),
	}
	builder.WithLanguageConfigOpts(
		ssa.WithLanguageConfigBind(true),
		ssa.WithLanguageConfigSupportClass(true),
		ssa.WithLanguageConfigIsSupportClassStaticModifier(false), // Python doesn't have static modifiers like Java
		ssa.WithLanguageConfigVirtualImport(true),
		ssa.WithLanguageBuilder(builder),
	)
	return builder
}

// FilterParseAST determines if a file should be parsed based on its extension.
// Returns true if the file has a .py extension.
func (s *SSABuilder) FilterParseAST(path string) bool {
	extension := filepath.Ext(path)
	return extension == ".py"
}

// GetAntlrCache returns the AntlrCache for Python parser and lexer.
// This cache improves parsing performance by reusing ATN data.
func (s *SSABuilder) GetAntlrCache() *ssa.AntlrCache {
	return s.CreateAntlrCache(pythonparser.GetPythonLexerSerializedATN(), pythonparser.GetPythonParserSerializedATN())
}

// ParseAST parses Python source code and returns the AST.
// It uses the provided cache if available to improve performance.
func (s *SSABuilder) ParseAST(src string, cache *ssa.AntlrCache) (ssa.FrontAST, error) {
	return FrontendWithCache(src, cache)
}

// BuildFromAST builds SSA representation from the parsed AST.
// This is the main entry point for converting Python AST to SSA.
func (*SSABuilder) BuildFromAST(raw ssa.FrontAST, b *ssa.FunctionBuilder) error {
	ast, ok := raw.(pythonparser.IRootContext)
	if !ok {
		return utils.Errorf("invalid AST type: %T, expected pythonparser.IRootContext", raw)
	}
	build := &singleFileBuilder{
		FunctionBuilder: b,
		constMap:        make(map[string]ssa.Value),
		globalNames:     make(map[string]bool),
	}
	build.VisitRoot(ast)
	return nil
}

// WrapWithPreprocessedFS wraps the filesystem with preprocessing if needed.
// For Python, this is a no-op currently as Python doesn't need template preprocessing like Java.
func (s *SSABuilder) WrapWithPreprocessedFS(fs fi.FileSystem) fi.FileSystem {
	// Python doesn't need special filesystem preprocessing like Java's template files (JSP, Freemarker, etc.)
	return fs
}

// FilterFile determines if a file should be processed based on its extension.
// Returns true if the file has a .py extension.
func (*SSABuilder) FilterFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".py"
}

// GetLanguage returns the language identifier for Python.
func (*SSABuilder) GetLanguage() ssaconfig.Language {
	return ssaconfig.PYTHON
}

// ========================================== PreHandlerAnalyzer Implementation ==========================================

var _ ssa.PreHandlerAnalyzer = &SSABuilder{}

// FilterPreHandlerFile determines if a file should be preprocessed.
// For Python, we use a whitelist approach to only process Python-related files.
// This avoids conflicts with other language builders (e.g., Java's JSP files).
func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	extension := filepath.Ext(path)
	// Whitelist: only process files that Python projects typically contain
	whiteList := []string{".py", ".pyi", ".pyw", ".yaml", ".yml", ".txt", ".toml", ".cfg", ".ini", ".json"}
	for _, ext := range whiteList {
		if extension == ext {
			return true
		}
	}
	return false
}

// PreHandlerFile handles preprocessing of a single file.
// For Python, we don't need to call Build here as the framework calls it automatically.
// This is used for any pre-processing before the main build.
func (s *SSABuilder) PreHandlerFile(ast ssa.FrontAST, editor *memedit.MemEditor, builder *ssa.FunctionBuilder) {
	// Empty - the framework calls Build automatically
}

// PreHandlerProject handles preprocessing at the project level.
// For Python, this handles project-level configuration and dependencies.
func (s *SSABuilder) PreHandlerProject(fileSystem fi.FileSystem, ast ssa.FrontAST, fb *ssa.FunctionBuilder, editor *memedit.MemEditor) error {
	prog := fb.GetProgram()
	if prog == nil {
		return nil
	}
	if prog.ExtraFile == nil {
		prog.ExtraFile = make(map[string]string)
	}

	path := editor.GetUrl()
	ext := filepath.Ext(path)

	// Save extra file information
	saveExtraFile := func(path string) {
		if prog.GetProgramName() == "" {
			prog.ExtraFile[path] = editor.GetIrSourceHash()
		} else {
			prog.ExtraFile[path] = editor.GetIrSourceHash()
		}
	}

	switch ext {
	case ".py":
		// Python files are built by PreHandlerFile, not here
		// This prevents double compilation
		return nil
	case ".jpg", ".png", ".gif", ".jpeg", ".css", ".js", ".avi", ".mp4", ".mp3", ".pdf", ".doc":
		// Skip binary/media files
		return nil
	case ".yaml", ".yml":
		// Handle YAML configuration files (e.g., requirements.yaml, setup.yaml)
		saveExtraFile(path)
		if err := prog.ParseProjectConfig([]byte(editor.GetSourceCode()), path, ssa.PROJECT_CONFIG_YAML); err != nil {
			return err
		}
	case ".txt":
		// Handle requirements.txt
		if filepath.Base(path) == "requirements.txt" {
			saveExtraFile(path)
			// TODO: Parse requirements.txt for dependency information
		} else {
			saveExtraFile(path)
		}
	default:
		saveExtraFile(path)
	}
	return nil
}

// ========================================== Build Front End ==========================================

// singleFileBuilder handles the conversion of a single Python file's AST to SSA.
// It maintains state during the conversion process, including constants and function builders.
type singleFileBuilder struct {
	*ssa.FunctionBuilder
	constMap    map[string]ssa.Value
	globalNames map[string]bool
}

// FrontendWithCache parses Python source code and returns the root AST node.
// It supports optional AntlrCache for improved parsing performance.
// Similar to java2ssa.Frontend, but for Python syntax.
func FrontendWithCache(src string, caches ...*ssa.AntlrCache) (pythonparser.IRootContext, error) {
	var cache *ssa.AntlrCache
	if len(caches) > 0 {
		cache = caches[0]
	}
	errListener := antlr4util.NewErrorListener()
	lexer := pythonparser.NewPythonLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := pythonparser.NewPythonParser(tokenStream)
	ssa.ParserSetAntlrCache(parser, lexer, cache)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.Root()
	return ast, errListener.Error()
}

// Frontend parses Python source code and returns the root AST node.
// This is a convenience function that calls the Frontend function in builder.go.
// For better performance with caching, use the Frontend function in builder.go directly.
func Frontend(src string) (pythonparser.IRootContext, error) {
	return FrontendWithCache(src)
}
