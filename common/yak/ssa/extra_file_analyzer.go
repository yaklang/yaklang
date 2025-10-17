package ssa

import (
	"runtime"
	"sync"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/consts"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

type PreHandlerAnalyzer interface {
	InitHandler(builder *FunctionBuilder)
	FilterPreHandlerFile(string) bool

	FilterParseAST(string) bool
	ParseAST(string) (FrontAST, error)
	PreHandlerProject(fi.FileSystem, FrontAST, *FunctionBuilder, *memedit.MemEditor) error
	PreHandlerFile(FrontAST, *memedit.MemEditor, *FunctionBuilder)

	AfterPreHandlerProject(builder *FunctionBuilder)

	Clearup()
}

type FrontAST interface {
}

type CreateBuilder func() Builder

type Builder interface {
	// create a new builder
	BuildFromAST(FrontAST, *FunctionBuilder) error
	FilterFile(string) bool
	GetLanguage() consts.Language
	PreHandlerAnalyzer
}

type initHanlderFunc func(*FunctionBuilder)

type PreHandlerBase struct {
	InitHandlerOnce       sync.Once
	initHandlerFunc       []initHanlderFunc
	beforeInitHandlerFunc []initHanlderFunc
	languageConfigOpts    []LanguageConfigOpt

	// antlr cache
	antlrMutex             sync.RWMutex
	antlrBuild             bool
	DfaCache               []*antlr.DFA
	PredictionContextCache *antlr.PredictionContextCache
}

func (d *PreHandlerBase) AfterPreHandlerProject(builder *FunctionBuilder) {
	d.InitHandler(builder)
}

func NewPreHandlerBase(fs ...initHanlderFunc) *PreHandlerBase {
	builder := &PreHandlerBase{
		InitHandlerOnce: sync.Once{},
		initHandlerFunc: fs,
	}

	return builder
}

func (d *PreHandlerBase) WithLanguageConfigOpts(opts ...LanguageConfigOpt) *PreHandlerBase {
	d.languageConfigOpts = opts
	return d
}
func (d *PreHandlerBase) WithPreInitHandler(fs ...initHanlderFunc) *PreHandlerBase {
	d.beforeInitHandlerFunc = fs
	return d
}

var ProjectConfigVariable = "__projectConfig__"

func (d *PreHandlerBase) InitHandler(b *FunctionBuilder) {
	d.InitHandlerOnce.Do(func() {
		// build the global dependency scope
		b.SetEmptyRange()
		b.SetLanguageConfig(d.languageConfigOpts...)
		beforeInit := d.beforeInitHandlerFunc
		d.beforeInitHandlerFunc = nil
		for _, handlerFunc := range beforeInit {
			handlerFunc(b)
		}
		variable := b.CreateVariable("__dependency__")
		container := b.EmitEmptyContainer()
		b.AssignVariable(variable, container)

		configVariable := b.CreateVariable(ProjectConfigVariable)
		configContainer := b.EmitEmptyContainer()
		b.AssignVariable(configVariable, configContainer)
		// run the init handler functions
		init := d.initHandlerFunc
		d.initHandlerFunc = nil
		for _, f := range init {
			f(b)
		}
	})
}

func (d *PreHandlerBase) PreHandlerFile(ast FrontAST, editor *memedit.MemEditor, builder *FunctionBuilder) {
}

func (d *PreHandlerBase) FilterPreHandlerFile(string) bool {
	return false
}

func (d *PreHandlerBase) PreHandlerProject(fi.FileSystem, FrontAST, *FunctionBuilder, *memedit.MemEditor) error {
	return nil
}

func (d *PreHandlerBase) Clearup() {
	d.antlrMutex.Lock()
	defer d.antlrMutex.Unlock()
	// Clear DFA cache explicitly
	d.DfaCache = nil
	// Clear prediction context cache
	d.PredictionContextCache = nil
	// Force garbage collection
	runtime.GC()
}

// createAntlrCache creates new DFA cache and prediction context cache for the given ATN
func createAntlrCache(atn *antlr.ATN) ([]*antlr.DFA, *antlr.PredictionContextCache) {
	decisionToDFA := make([]*antlr.DFA, len(atn.DecisionToState))
	for i := range decisionToDFA {
		decisionToDFA[i] = antlr.NewDFA(atn.DecisionToState[i], i)
	}
	predictionContextCache := antlr.NewPredictionContextCache()
	// log.Errorf("Created new ANTLR cache with %d DFAs", len(decisionToDFA))
	return decisionToDFA, predictionContextCache
}

// getOrCreateAntlrCache returns existing cache if available, otherwise creates new cache
func (builder *PreHandlerBase) getOrCreateAntlrCache(atn *antlr.ATN) ([]*antlr.DFA, *antlr.PredictionContextCache) {
	if builder == nil {
		// No builder available, create cache directly
		return createAntlrCache(atn)
	}

	// Check if cache is already built
	builder.antlrMutex.RLock()
	if len(builder.DfaCache) != 0 && builder.PredictionContextCache != nil {
		// Cache is available, return existing cache
		decisionToDFA := builder.DfaCache
		predictionContextCache := builder.PredictionContextCache
		builder.antlrMutex.RUnlock()
		// log.Errorf("Using existing ANTLR cache with %d DFAs", len(decisionToDFA))
		return decisionToDFA, predictionContextCache
	}
	builder.antlrMutex.RUnlock()

	// Cache not built yet, create new cache
	decisionToDFA, predictionContextCache := createAntlrCache(atn)

	// Store cache in builder for future use
	builder.antlrMutex.Lock()
	builder.DfaCache = decisionToDFA
	builder.PredictionContextCache = predictionContextCache
	builder.antlrMutex.Unlock()
	// log.Errorf("Storing new ANTLR cache with %d DFAs", len(decisionToDFA))

	return decisionToDFA, predictionContextCache
}

// ParserSetAntlrCache sets up ANTLR cache for the parser to improve performance
// If builder has existing cache, use it; otherwise create new cache and store in builder if available
func ParserSetAntlrCache(parser *antlr.BaseParser, builder *PreHandlerBase) *antlr.BaseParser {
	atn := parser.GetATN()
	decisionToDFA, predictionContextCache := builder.getOrCreateAntlrCache(atn)

	parser.Interpreter = antlr.NewParserATNSimulator(
		parser, atn, decisionToDFA, predictionContextCache,
	)
	return parser
}
