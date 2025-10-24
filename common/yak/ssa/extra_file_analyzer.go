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
	ParseAST(string, *AntlrCache) (FrontAST, error)
	GetAntlrCache() *AntlrCache

	PreHandlerProject(fi.FileSystem, FrontAST, *FunctionBuilder, *memedit.MemEditor) error
	PreHandlerFile(FrontAST, *memedit.MemEditor, *FunctionBuilder)

	AfterPreHandlerProject(builder *FunctionBuilder)

	Clearup()
}

type FrontAST interface {
}

type AntlrCache struct {
	LexerATN                    *antlr.ATN
	LexerDfaCache               []*antlr.DFA
	LexerPredictionContextCache *antlr.PredictionContextCache

	ParserATN                    *antlr.ATN
	ParserDfaCache               []*antlr.DFA
	ParserPredictionContextCache *antlr.PredictionContextCache
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
	antlrMutex sync.RWMutex
	Caches     []*AntlrCache
	// antlrBuild bool

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
	d.Caches = nil
	// Force garbage collection
	runtime.GC()
}

func (a *AntlrCache) Empty() bool {
	return a == nil || (a.LexerATN == nil && a.ParserATN == nil)
}

// createAntlrCache creates new DFA cache and prediction context cache for the given ATN
func createAntlrCache(lexer, parser []int32) *AntlrCache {
	CreateCacheFromATN := func(serializedATN []int32) (*antlr.ATN, []*antlr.DFA, *antlr.PredictionContextCache) {
		atn := antlr.NewATNDeserializer(nil).Deserialize(serializedATN)
		decisionToDFA := make([]*antlr.DFA, len(atn.DecisionToState))
		for i, state := range atn.DecisionToState {
			decisionToDFA[i] = antlr.NewDFA(state, i)
		}
		predictionContextCache := antlr.NewPredictionContextCache()
		return atn, decisionToDFA, predictionContextCache
	}

	cache := &AntlrCache{}
	log.Errorf("Creating new ANTLR cache")
	if parser != nil {
		atn, decisionToDFA, predictionContextCache := CreateCacheFromATN(parser)
		cache.ParserATN = atn
		cache.ParserDfaCache = decisionToDFA
		cache.ParserPredictionContextCache = predictionContextCache
	}

	if lexer != nil {
		atn, decisionToDFA, predictionContextCache := CreateCacheFromATN(lexer)
		cache.LexerATN = atn
		cache.LexerDfaCache = decisionToDFA
		cache.LexerPredictionContextCache = predictionContextCache
	}
	return cache
}

func (b *PreHandlerBase) CreateAntlrCache(lexer []int32, parser []int32) *AntlrCache {
	cache := createAntlrCache(lexer, parser)
	if cache.Empty() {
		return nil
	}
	b.antlrMutex.Lock()
	b.Caches = append(b.Caches, cache)
	b.antlrMutex.Unlock()
	return cache
}

type LexerOrParser interface {
	SetInterpreter(*antlr.ATN, []*antlr.DFA, *antlr.PredictionContextCache)
}

// ParserSetAntlrCache sets up ANTLR cache for the parser to improve performance
// If builder has existing cache, use it; otherwise create new cache and store in builder if available
func ParserSetAntlrCache(parser, lexer LexerOrParser, cache *AntlrCache) {
	if cache.Empty() {
		return
	}
	parser.SetInterpreter(cache.ParserATN, cache.ParserDfaCache, cache.ParserPredictionContextCache)
	lexer.SetInterpreter(cache.LexerATN, cache.LexerDfaCache, cache.LexerPredictionContextCache)
}
