// Code generated from ./ASPParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package aspparser // ASPParser
import (
	"fmt"
	"strconv"
	"sync"

	"github.com/yaklang/antlr/v4"
)

// Suppress unused import errors
var _ = fmt.Printf
var _ = strconv.Itoa
var _ = sync.Once{}

type ASPParser struct {
	*antlr.BaseParser
}

var ASPParserParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	LiteralNames           []string
	SymbolicNames          []string
	RuleNames              []string
	PredictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func aspparserParserInit() {
	staticData := &ASPParserParserStaticData
	staticData.LiteralNames = []string{
		"", "", "", "", "", "'<%@'", "'<%!'", "'<%='", "'<%#'", "'<%'", "'</'",
		"'<'", "", "", "'%>'", "", "'/>'", "'>'", "'='", "", "", "'\\u0000'",
	}
	staticData.SymbolicNames = []string{
		"", "ASP_COMMENT", "HTML_COMMENT", "SCRIPT_OPEN", "STYLE_OPEN", "DIRECTIVE_BEGIN",
		"DECLARATION_BEGIN", "ECHO_EXPRESSION_OPEN", "DATABIND_OPEN", "SCRIPTLET_OPEN",
		"CLOSE_TAG_BEGIN", "TAG_BEGIN", "WHITESPACES", "ASP_STATIC_CONTENT_CHARS",
		"BLOB_CLOSE", "BLOB_CONTENT", "TAG_SLASH_END", "TAG_CLOSE", "TAG_EQUALS",
		"TAG_IDENTIFIER", "TAG_WHITESPACE", "ATTVAL_VALUE", "SCRIPT_BODY", "STYLE_BODY",
	}
	staticData.RuleNames = []string{
		"aspDocuments", "aspDocument", "aspScript", "aspDirective", "aspDeclaration",
		"aspExpression", "aspDatabind", "aspScriptlet", "blobContent", "htmlElement",
		"htmlCloseElement", "htmlTag", "htmlAttribute", "htmlContent", "htmlMisc",
		"script", "style",
	}
	staticData.PredictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 23, 165, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 1, 0, 5, 0, 36, 8, 0, 10, 0, 12, 0, 39, 9, 0, 1, 0, 1, 0,
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3, 1, 49, 8, 1, 1, 2, 1, 2, 1, 2, 1,
		2, 1, 2, 3, 2, 56, 8, 2, 1, 3, 1, 3, 3, 3, 60, 8, 3, 1, 3, 1, 3, 1, 4,
		1, 4, 3, 4, 66, 8, 4, 1, 4, 1, 4, 1, 5, 1, 5, 3, 5, 72, 8, 5, 1, 5, 1,
		5, 1, 6, 1, 6, 3, 6, 78, 8, 6, 1, 6, 1, 6, 1, 7, 1, 7, 3, 7, 84, 8, 7,
		1, 7, 1, 7, 1, 8, 4, 8, 89, 8, 8, 11, 8, 12, 8, 90, 1, 9, 1, 9, 1, 9, 5,
		9, 96, 8, 9, 10, 9, 12, 9, 99, 9, 9, 1, 9, 1, 9, 5, 9, 103, 8, 9, 10, 9,
		12, 9, 106, 9, 9, 1, 9, 1, 9, 1, 9, 1, 9, 1, 9, 1, 9, 1, 9, 5, 9, 115,
		8, 9, 10, 9, 12, 9, 118, 9, 9, 1, 9, 1, 9, 1, 9, 1, 9, 1, 9, 5, 9, 125,
		8, 9, 10, 9, 12, 9, 128, 9, 9, 1, 9, 1, 9, 3, 9, 132, 8, 9, 1, 10, 1, 10,
		1, 10, 1, 10, 1, 11, 1, 11, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1,
		12, 3, 12, 147, 8, 12, 1, 13, 1, 13, 1, 13, 1, 13, 1, 13, 1, 13, 3, 13,
		155, 8, 13, 1, 14, 1, 14, 1, 15, 1, 15, 1, 15, 1, 16, 1, 16, 1, 16, 1,
		16, 0, 0, 17, 0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30,
		32, 0, 1, 1, 0, 12, 13, 176, 0, 37, 1, 0, 0, 0, 2, 48, 1, 0, 0, 0, 4, 55,
		1, 0, 0, 0, 6, 57, 1, 0, 0, 0, 8, 63, 1, 0, 0, 0, 10, 69, 1, 0, 0, 0, 12,
		75, 1, 0, 0, 0, 14, 81, 1, 0, 0, 0, 16, 88, 1, 0, 0, 0, 18, 131, 1, 0,
		0, 0, 20, 133, 1, 0, 0, 0, 22, 137, 1, 0, 0, 0, 24, 146, 1, 0, 0, 0, 26,
		154, 1, 0, 0, 0, 28, 156, 1, 0, 0, 0, 30, 158, 1, 0, 0, 0, 32, 161, 1,
		0, 0, 0, 34, 36, 3, 2, 1, 0, 35, 34, 1, 0, 0, 0, 36, 39, 1, 0, 0, 0, 37,
		35, 1, 0, 0, 0, 37, 38, 1, 0, 0, 0, 38, 40, 1, 0, 0, 0, 39, 37, 1, 0, 0,
		0, 40, 41, 5, 0, 0, 1, 41, 1, 1, 0, 0, 0, 42, 49, 3, 4, 2, 0, 43, 49, 3,
		18, 9, 0, 44, 49, 3, 20, 10, 0, 45, 49, 3, 28, 14, 0, 46, 49, 3, 30, 15,
		0, 47, 49, 3, 32, 16, 0, 48, 42, 1, 0, 0, 0, 48, 43, 1, 0, 0, 0, 48, 44,
		1, 0, 0, 0, 48, 45, 1, 0, 0, 0, 48, 46, 1, 0, 0, 0, 48, 47, 1, 0, 0, 0,
		49, 3, 1, 0, 0, 0, 50, 56, 3, 6, 3, 0, 51, 56, 3, 8, 4, 0, 52, 56, 3, 10,
		5, 0, 53, 56, 3, 12, 6, 0, 54, 56, 3, 14, 7, 0, 55, 50, 1, 0, 0, 0, 55,
		51, 1, 0, 0, 0, 55, 52, 1, 0, 0, 0, 55, 53, 1, 0, 0, 0, 55, 54, 1, 0, 0,
		0, 56, 5, 1, 0, 0, 0, 57, 59, 5, 5, 0, 0, 58, 60, 3, 16, 8, 0, 59, 58,
		1, 0, 0, 0, 59, 60, 1, 0, 0, 0, 60, 61, 1, 0, 0, 0, 61, 62, 5, 14, 0, 0,
		62, 7, 1, 0, 0, 0, 63, 65, 5, 6, 0, 0, 64, 66, 3, 16, 8, 0, 65, 64, 1,
		0, 0, 0, 65, 66, 1, 0, 0, 0, 66, 67, 1, 0, 0, 0, 67, 68, 5, 14, 0, 0, 68,
		9, 1, 0, 0, 0, 69, 71, 5, 7, 0, 0, 70, 72, 3, 16, 8, 0, 71, 70, 1, 0, 0,
		0, 71, 72, 1, 0, 0, 0, 72, 73, 1, 0, 0, 0, 73, 74, 5, 14, 0, 0, 74, 11,
		1, 0, 0, 0, 75, 77, 5, 8, 0, 0, 76, 78, 3, 16, 8, 0, 77, 76, 1, 0, 0, 0,
		77, 78, 1, 0, 0, 0, 78, 79, 1, 0, 0, 0, 79, 80, 5, 14, 0, 0, 80, 13, 1,
		0, 0, 0, 81, 83, 5, 9, 0, 0, 82, 84, 3, 16, 8, 0, 83, 82, 1, 0, 0, 0, 83,
		84, 1, 0, 0, 0, 84, 85, 1, 0, 0, 0, 85, 86, 5, 14, 0, 0, 86, 15, 1, 0,
		0, 0, 87, 89, 5, 15, 0, 0, 88, 87, 1, 0, 0, 0, 89, 90, 1, 0, 0, 0, 90,
		88, 1, 0, 0, 0, 90, 91, 1, 0, 0, 0, 91, 17, 1, 0, 0, 0, 92, 93, 5, 11,
		0, 0, 93, 97, 3, 22, 11, 0, 94, 96, 3, 24, 12, 0, 95, 94, 1, 0, 0, 0, 96,
		99, 1, 0, 0, 0, 97, 95, 1, 0, 0, 0, 97, 98, 1, 0, 0, 0, 98, 100, 1, 0,
		0, 0, 99, 97, 1, 0, 0, 0, 100, 104, 5, 17, 0, 0, 101, 103, 3, 26, 13, 0,
		102, 101, 1, 0, 0, 0, 103, 106, 1, 0, 0, 0, 104, 102, 1, 0, 0, 0, 104,
		105, 1, 0, 0, 0, 105, 107, 1, 0, 0, 0, 106, 104, 1, 0, 0, 0, 107, 108,
		5, 10, 0, 0, 108, 109, 3, 22, 11, 0, 109, 110, 5, 17, 0, 0, 110, 132, 1,
		0, 0, 0, 111, 112, 5, 11, 0, 0, 112, 116, 3, 22, 11, 0, 113, 115, 3, 24,
		12, 0, 114, 113, 1, 0, 0, 0, 115, 118, 1, 0, 0, 0, 116, 114, 1, 0, 0, 0,
		116, 117, 1, 0, 0, 0, 117, 119, 1, 0, 0, 0, 118, 116, 1, 0, 0, 0, 119,
		120, 5, 16, 0, 0, 120, 132, 1, 0, 0, 0, 121, 122, 5, 11, 0, 0, 122, 126,
		3, 22, 11, 0, 123, 125, 3, 24, 12, 0, 124, 123, 1, 0, 0, 0, 125, 128, 1,
		0, 0, 0, 126, 124, 1, 0, 0, 0, 126, 127, 1, 0, 0, 0, 127, 129, 1, 0, 0,
		0, 128, 126, 1, 0, 0, 0, 129, 130, 5, 17, 0, 0, 130, 132, 1, 0, 0, 0, 131,
		92, 1, 0, 0, 0, 131, 111, 1, 0, 0, 0, 131, 121, 1, 0, 0, 0, 132, 19, 1,
		0, 0, 0, 133, 134, 5, 10, 0, 0, 134, 135, 3, 22, 11, 0, 135, 136, 5, 17,
		0, 0, 136, 21, 1, 0, 0, 0, 137, 138, 5, 19, 0, 0, 138, 23, 1, 0, 0, 0,
		139, 140, 5, 19, 0, 0, 140, 141, 5, 18, 0, 0, 141, 147, 5, 21, 0, 0, 142,
		143, 5, 19, 0, 0, 143, 144, 5, 18, 0, 0, 144, 147, 3, 4, 2, 0, 145, 147,
		5, 19, 0, 0, 146, 139, 1, 0, 0, 0, 146, 142, 1, 0, 0, 0, 146, 145, 1, 0,
		0, 0, 147, 25, 1, 0, 0, 0, 148, 155, 3, 28, 14, 0, 149, 155, 3, 18, 9,
		0, 150, 155, 3, 20, 10, 0, 151, 155, 3, 4, 2, 0, 152, 155, 3, 30, 15, 0,
		153, 155, 3, 32, 16, 0, 154, 148, 1, 0, 0, 0, 154, 149, 1, 0, 0, 0, 154,
		150, 1, 0, 0, 0, 154, 151, 1, 0, 0, 0, 154, 152, 1, 0, 0, 0, 154, 153,
		1, 0, 0, 0, 155, 27, 1, 0, 0, 0, 156, 157, 7, 0, 0, 0, 157, 29, 1, 0, 0,
		0, 158, 159, 5, 3, 0, 0, 159, 160, 5, 22, 0, 0, 160, 31, 1, 0, 0, 0, 161,
		162, 5, 4, 0, 0, 162, 163, 5, 23, 0, 0, 163, 33, 1, 0, 0, 0, 16, 37, 48,
		55, 59, 65, 71, 77, 83, 90, 97, 104, 116, 126, 131, 146, 154,
	}
	deserializer := antlr.NewATNDeserializer(nil)
	staticData.atn = deserializer.Deserialize(staticData.serializedATN)
	atn := staticData.atn
	staticData.decisionToDFA = make([]*antlr.DFA, len(atn.DecisionToState))
	decisionToDFA := staticData.decisionToDFA
	for index, state := range atn.DecisionToState {
		decisionToDFA[index] = antlr.NewDFA(state, index)
	}
}

// ASPParserInit initializes any static state used to implement ASPParser. By default the
// static state used to implement the parser is lazily initialized during the first call to
// NewASPParser(). You can call this function if you wish to initialize the static state ahead
// of time.
func ASPParserInit() {
	staticData := &ASPParserParserStaticData
	staticData.once.Do(aspparserParserInit)
}

// NewASPParser produces a new parser instance for the optional input antlr.TokenStream.
func NewASPParser(input antlr.TokenStream) *ASPParser {
	ASPParserInit()
	this := new(ASPParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &ASPParserParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.PredictionContextCache)
	this.RuleNames = staticData.RuleNames
	this.LiteralNames = staticData.LiteralNames
	this.SymbolicNames = staticData.SymbolicNames
	this.GrammarFileName = "ASPParser.g4"

	return this
}

// ASPParser tokens.
const (
	ASPParserEOF                      = antlr.TokenEOF
	ASPParserASP_COMMENT              = 1
	ASPParserHTML_COMMENT             = 2
	ASPParserSCRIPT_OPEN              = 3
	ASPParserSTYLE_OPEN               = 4
	ASPParserDIRECTIVE_BEGIN          = 5
	ASPParserDECLARATION_BEGIN        = 6
	ASPParserECHO_EXPRESSION_OPEN     = 7
	ASPParserDATABIND_OPEN            = 8
	ASPParserSCRIPTLET_OPEN           = 9
	ASPParserCLOSE_TAG_BEGIN          = 10
	ASPParserTAG_BEGIN                = 11
	ASPParserWHITESPACES              = 12
	ASPParserASP_STATIC_CONTENT_CHARS = 13
	ASPParserBLOB_CLOSE               = 14
	ASPParserBLOB_CONTENT             = 15
	ASPParserTAG_SLASH_END            = 16
	ASPParserTAG_CLOSE                = 17
	ASPParserTAG_EQUALS               = 18
	ASPParserTAG_IDENTIFIER           = 19
	ASPParserTAG_WHITESPACE           = 20
	ASPParserATTVAL_VALUE             = 21
	ASPParserSCRIPT_BODY              = 22
	ASPParserSTYLE_BODY               = 23
)

// ASPParser rules.
const (
	ASPParserRULE_aspDocuments     = 0
	ASPParserRULE_aspDocument      = 1
	ASPParserRULE_aspScript        = 2
	ASPParserRULE_aspDirective     = 3
	ASPParserRULE_aspDeclaration   = 4
	ASPParserRULE_aspExpression    = 5
	ASPParserRULE_aspDatabind      = 6
	ASPParserRULE_aspScriptlet     = 7
	ASPParserRULE_blobContent      = 8
	ASPParserRULE_htmlElement      = 9
	ASPParserRULE_htmlCloseElement = 10
	ASPParserRULE_htmlTag          = 11
	ASPParserRULE_htmlAttribute    = 12
	ASPParserRULE_htmlContent      = 13
	ASPParserRULE_htmlMisc         = 14
	ASPParserRULE_script           = 15
	ASPParserRULE_style            = 16
)

// IAspDocumentsContext is an interface to support dynamic dispatch.
type IAspDocumentsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	EOF() antlr.TerminalNode
	AllAspDocument() []IAspDocumentContext
	AspDocument(i int) IAspDocumentContext

	// IsAspDocumentsContext differentiates from other interfaces.
	IsAspDocumentsContext()
}

type AspDocumentsContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAspDocumentsContext() *AspDocumentsContext {
	var p = new(AspDocumentsContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspDocuments
	return p
}

func InitEmptyAspDocumentsContext(p *AspDocumentsContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspDocuments
}

func (*AspDocumentsContext) IsAspDocumentsContext() {}

func NewAspDocumentsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AspDocumentsContext {
	var p = new(AspDocumentsContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_aspDocuments

	return p
}

func (s *AspDocumentsContext) GetParser() antlr.Parser { return s.parser }

func (s *AspDocumentsContext) EOF() antlr.TerminalNode {
	return s.GetToken(ASPParserEOF, 0)
}

func (s *AspDocumentsContext) AllAspDocument() []IAspDocumentContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IAspDocumentContext); ok {
			len++
		}
	}

	tst := make([]IAspDocumentContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IAspDocumentContext); ok {
			tst[i] = t.(IAspDocumentContext)
			i++
		}
	}

	return tst
}

func (s *AspDocumentsContext) AspDocument(i int) IAspDocumentContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAspDocumentContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAspDocumentContext)
}

func (s *AspDocumentsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AspDocumentsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AspDocumentsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitAspDocuments(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) AspDocuments() (localctx IAspDocumentsContext) {
	this := p
	_ = this

	localctx = NewAspDocumentsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, ASPParserRULE_aspDocuments)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	p.SetState(37)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&16376) != 0 {
		{
			p.SetState(34)
			p.AspDocument()
		}

		p.SetState(39)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(40)
		p.Match(ASPParserEOF)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IAspDocumentContext is an interface to support dynamic dispatch.
type IAspDocumentContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	AspScript() IAspScriptContext
	HtmlElement() IHtmlElementContext
	HtmlCloseElement() IHtmlCloseElementContext
	HtmlMisc() IHtmlMiscContext
	Script() IScriptContext
	Style() IStyleContext

	// IsAspDocumentContext differentiates from other interfaces.
	IsAspDocumentContext()
}

type AspDocumentContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAspDocumentContext() *AspDocumentContext {
	var p = new(AspDocumentContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspDocument
	return p
}

func InitEmptyAspDocumentContext(p *AspDocumentContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspDocument
}

func (*AspDocumentContext) IsAspDocumentContext() {}

func NewAspDocumentContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AspDocumentContext {
	var p = new(AspDocumentContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_aspDocument

	return p
}

func (s *AspDocumentContext) GetParser() antlr.Parser { return s.parser }

func (s *AspDocumentContext) AspScript() IAspScriptContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAspScriptContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAspScriptContext)
}

func (s *AspDocumentContext) HtmlElement() IHtmlElementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlElementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlElementContext)
}

func (s *AspDocumentContext) HtmlCloseElement() IHtmlCloseElementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlCloseElementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlCloseElementContext)
}

func (s *AspDocumentContext) HtmlMisc() IHtmlMiscContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlMiscContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlMiscContext)
}

func (s *AspDocumentContext) Script() IScriptContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IScriptContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IScriptContext)
}

func (s *AspDocumentContext) Style() IStyleContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStyleContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStyleContext)
}

func (s *AspDocumentContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AspDocumentContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AspDocumentContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitAspDocument(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) AspDocument() (localctx IAspDocumentContext) {
	this := p
	_ = this

	localctx = NewAspDocumentContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 2, ASPParserRULE_aspDocument)
	p.SetState(48)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetTokenStream().LA(1) {
	case ASPParserDIRECTIVE_BEGIN, ASPParserDECLARATION_BEGIN, ASPParserECHO_EXPRESSION_OPEN, ASPParserDATABIND_OPEN, ASPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(42)
			p.AspScript()
		}

	case ASPParserTAG_BEGIN:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(43)
			p.HtmlElement()
		}

	case ASPParserCLOSE_TAG_BEGIN:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(44)
			p.HtmlCloseElement()
		}

	case ASPParserWHITESPACES, ASPParserASP_STATIC_CONTENT_CHARS:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(45)
			p.HtmlMisc()
		}

	case ASPParserSCRIPT_OPEN:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(46)
			p.Script()
		}

	case ASPParserSTYLE_OPEN:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(47)
			p.Style()
		}

	default:
		p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IAspScriptContext is an interface to support dynamic dispatch.
type IAspScriptContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	AspDirective() IAspDirectiveContext
	AspDeclaration() IAspDeclarationContext
	AspExpression() IAspExpressionContext
	AspDatabind() IAspDatabindContext
	AspScriptlet() IAspScriptletContext

	// IsAspScriptContext differentiates from other interfaces.
	IsAspScriptContext()
}

type AspScriptContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAspScriptContext() *AspScriptContext {
	var p = new(AspScriptContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspScript
	return p
}

func InitEmptyAspScriptContext(p *AspScriptContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspScript
}

func (*AspScriptContext) IsAspScriptContext() {}

func NewAspScriptContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AspScriptContext {
	var p = new(AspScriptContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_aspScript

	return p
}

func (s *AspScriptContext) GetParser() antlr.Parser { return s.parser }

func (s *AspScriptContext) AspDirective() IAspDirectiveContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAspDirectiveContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAspDirectiveContext)
}

func (s *AspScriptContext) AspDeclaration() IAspDeclarationContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAspDeclarationContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAspDeclarationContext)
}

func (s *AspScriptContext) AspExpression() IAspExpressionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAspExpressionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAspExpressionContext)
}

func (s *AspScriptContext) AspDatabind() IAspDatabindContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAspDatabindContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAspDatabindContext)
}

func (s *AspScriptContext) AspScriptlet() IAspScriptletContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAspScriptletContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAspScriptletContext)
}

func (s *AspScriptContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AspScriptContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AspScriptContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitAspScript(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) AspScript() (localctx IAspScriptContext) {
	this := p
	_ = this

	localctx = NewAspScriptContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, ASPParserRULE_aspScript)
	p.SetState(55)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetTokenStream().LA(1) {
	case ASPParserDIRECTIVE_BEGIN:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(50)
			p.AspDirective()
		}

	case ASPParserDECLARATION_BEGIN:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(51)
			p.AspDeclaration()
		}

	case ASPParserECHO_EXPRESSION_OPEN:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(52)
			p.AspExpression()
		}

	case ASPParserDATABIND_OPEN:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(53)
			p.AspDatabind()
		}

	case ASPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(54)
			p.AspScriptlet()
		}

	default:
		p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IAspDirectiveContext is an interface to support dynamic dispatch.
type IAspDirectiveContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	DIRECTIVE_BEGIN() antlr.TerminalNode
	BLOB_CLOSE() antlr.TerminalNode
	BlobContent() IBlobContentContext

	// IsAspDirectiveContext differentiates from other interfaces.
	IsAspDirectiveContext()
}

type AspDirectiveContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAspDirectiveContext() *AspDirectiveContext {
	var p = new(AspDirectiveContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspDirective
	return p
}

func InitEmptyAspDirectiveContext(p *AspDirectiveContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspDirective
}

func (*AspDirectiveContext) IsAspDirectiveContext() {}

func NewAspDirectiveContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AspDirectiveContext {
	var p = new(AspDirectiveContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_aspDirective

	return p
}

func (s *AspDirectiveContext) GetParser() antlr.Parser { return s.parser }

func (s *AspDirectiveContext) DIRECTIVE_BEGIN() antlr.TerminalNode {
	return s.GetToken(ASPParserDIRECTIVE_BEGIN, 0)
}

func (s *AspDirectiveContext) BLOB_CLOSE() antlr.TerminalNode {
	return s.GetToken(ASPParserBLOB_CLOSE, 0)
}

func (s *AspDirectiveContext) BlobContent() IBlobContentContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBlobContentContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBlobContentContext)
}

func (s *AspDirectiveContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AspDirectiveContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AspDirectiveContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitAspDirective(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) AspDirective() (localctx IAspDirectiveContext) {
	this := p
	_ = this

	localctx = NewAspDirectiveContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 6, ASPParserRULE_aspDirective)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(57)
		p.Match(ASPParserDIRECTIVE_BEGIN)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(59)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == ASPParserBLOB_CONTENT {
		{
			p.SetState(58)
			p.BlobContent()
		}

	}
	{
		p.SetState(61)
		p.Match(ASPParserBLOB_CLOSE)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IAspDeclarationContext is an interface to support dynamic dispatch.
type IAspDeclarationContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	DECLARATION_BEGIN() antlr.TerminalNode
	BLOB_CLOSE() antlr.TerminalNode
	BlobContent() IBlobContentContext

	// IsAspDeclarationContext differentiates from other interfaces.
	IsAspDeclarationContext()
}

type AspDeclarationContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAspDeclarationContext() *AspDeclarationContext {
	var p = new(AspDeclarationContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspDeclaration
	return p
}

func InitEmptyAspDeclarationContext(p *AspDeclarationContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspDeclaration
}

func (*AspDeclarationContext) IsAspDeclarationContext() {}

func NewAspDeclarationContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AspDeclarationContext {
	var p = new(AspDeclarationContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_aspDeclaration

	return p
}

func (s *AspDeclarationContext) GetParser() antlr.Parser { return s.parser }

func (s *AspDeclarationContext) DECLARATION_BEGIN() antlr.TerminalNode {
	return s.GetToken(ASPParserDECLARATION_BEGIN, 0)
}

func (s *AspDeclarationContext) BLOB_CLOSE() antlr.TerminalNode {
	return s.GetToken(ASPParserBLOB_CLOSE, 0)
}

func (s *AspDeclarationContext) BlobContent() IBlobContentContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBlobContentContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBlobContentContext)
}

func (s *AspDeclarationContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AspDeclarationContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AspDeclarationContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitAspDeclaration(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) AspDeclaration() (localctx IAspDeclarationContext) {
	this := p
	_ = this

	localctx = NewAspDeclarationContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, ASPParserRULE_aspDeclaration)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(63)
		p.Match(ASPParserDECLARATION_BEGIN)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(65)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == ASPParserBLOB_CONTENT {
		{
			p.SetState(64)
			p.BlobContent()
		}

	}
	{
		p.SetState(67)
		p.Match(ASPParserBLOB_CLOSE)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IAspExpressionContext is an interface to support dynamic dispatch.
type IAspExpressionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ECHO_EXPRESSION_OPEN() antlr.TerminalNode
	BLOB_CLOSE() antlr.TerminalNode
	BlobContent() IBlobContentContext

	// IsAspExpressionContext differentiates from other interfaces.
	IsAspExpressionContext()
}

type AspExpressionContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAspExpressionContext() *AspExpressionContext {
	var p = new(AspExpressionContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspExpression
	return p
}

func InitEmptyAspExpressionContext(p *AspExpressionContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspExpression
}

func (*AspExpressionContext) IsAspExpressionContext() {}

func NewAspExpressionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AspExpressionContext {
	var p = new(AspExpressionContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_aspExpression

	return p
}

func (s *AspExpressionContext) GetParser() antlr.Parser { return s.parser }

func (s *AspExpressionContext) ECHO_EXPRESSION_OPEN() antlr.TerminalNode {
	return s.GetToken(ASPParserECHO_EXPRESSION_OPEN, 0)
}

func (s *AspExpressionContext) BLOB_CLOSE() antlr.TerminalNode {
	return s.GetToken(ASPParserBLOB_CLOSE, 0)
}

func (s *AspExpressionContext) BlobContent() IBlobContentContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBlobContentContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBlobContentContext)
}

func (s *AspExpressionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AspExpressionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AspExpressionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitAspExpression(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) AspExpression() (localctx IAspExpressionContext) {
	this := p
	_ = this

	localctx = NewAspExpressionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, ASPParserRULE_aspExpression)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(69)
		p.Match(ASPParserECHO_EXPRESSION_OPEN)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(71)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == ASPParserBLOB_CONTENT {
		{
			p.SetState(70)
			p.BlobContent()
		}

	}
	{
		p.SetState(73)
		p.Match(ASPParserBLOB_CLOSE)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IAspDatabindContext is an interface to support dynamic dispatch.
type IAspDatabindContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	DATABIND_OPEN() antlr.TerminalNode
	BLOB_CLOSE() antlr.TerminalNode
	BlobContent() IBlobContentContext

	// IsAspDatabindContext differentiates from other interfaces.
	IsAspDatabindContext()
}

type AspDatabindContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAspDatabindContext() *AspDatabindContext {
	var p = new(AspDatabindContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspDatabind
	return p
}

func InitEmptyAspDatabindContext(p *AspDatabindContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspDatabind
}

func (*AspDatabindContext) IsAspDatabindContext() {}

func NewAspDatabindContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AspDatabindContext {
	var p = new(AspDatabindContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_aspDatabind

	return p
}

func (s *AspDatabindContext) GetParser() antlr.Parser { return s.parser }

func (s *AspDatabindContext) DATABIND_OPEN() antlr.TerminalNode {
	return s.GetToken(ASPParserDATABIND_OPEN, 0)
}

func (s *AspDatabindContext) BLOB_CLOSE() antlr.TerminalNode {
	return s.GetToken(ASPParserBLOB_CLOSE, 0)
}

func (s *AspDatabindContext) BlobContent() IBlobContentContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBlobContentContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBlobContentContext)
}

func (s *AspDatabindContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AspDatabindContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AspDatabindContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitAspDatabind(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) AspDatabind() (localctx IAspDatabindContext) {
	this := p
	_ = this

	localctx = NewAspDatabindContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, ASPParserRULE_aspDatabind)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(75)
		p.Match(ASPParserDATABIND_OPEN)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(77)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == ASPParserBLOB_CONTENT {
		{
			p.SetState(76)
			p.BlobContent()
		}

	}
	{
		p.SetState(79)
		p.Match(ASPParserBLOB_CLOSE)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IAspScriptletContext is an interface to support dynamic dispatch.
type IAspScriptletContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	SCRIPTLET_OPEN() antlr.TerminalNode
	BLOB_CLOSE() antlr.TerminalNode
	BlobContent() IBlobContentContext

	// IsAspScriptletContext differentiates from other interfaces.
	IsAspScriptletContext()
}

type AspScriptletContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAspScriptletContext() *AspScriptletContext {
	var p = new(AspScriptletContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspScriptlet
	return p
}

func InitEmptyAspScriptletContext(p *AspScriptletContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_aspScriptlet
}

func (*AspScriptletContext) IsAspScriptletContext() {}

func NewAspScriptletContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AspScriptletContext {
	var p = new(AspScriptletContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_aspScriptlet

	return p
}

func (s *AspScriptletContext) GetParser() antlr.Parser { return s.parser }

func (s *AspScriptletContext) SCRIPTLET_OPEN() antlr.TerminalNode {
	return s.GetToken(ASPParserSCRIPTLET_OPEN, 0)
}

func (s *AspScriptletContext) BLOB_CLOSE() antlr.TerminalNode {
	return s.GetToken(ASPParserBLOB_CLOSE, 0)
}

func (s *AspScriptletContext) BlobContent() IBlobContentContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBlobContentContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBlobContentContext)
}

func (s *AspScriptletContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AspScriptletContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AspScriptletContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitAspScriptlet(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) AspScriptlet() (localctx IAspScriptletContext) {
	this := p
	_ = this

	localctx = NewAspScriptletContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 14, ASPParserRULE_aspScriptlet)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(81)
		p.Match(ASPParserSCRIPTLET_OPEN)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(83)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == ASPParserBLOB_CONTENT {
		{
			p.SetState(82)
			p.BlobContent()
		}

	}
	{
		p.SetState(85)
		p.Match(ASPParserBLOB_CLOSE)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IBlobContentContext is an interface to support dynamic dispatch.
type IBlobContentContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	AllBLOB_CONTENT() []antlr.TerminalNode
	BLOB_CONTENT(i int) antlr.TerminalNode

	// IsBlobContentContext differentiates from other interfaces.
	IsBlobContentContext()
}

type BlobContentContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyBlobContentContext() *BlobContentContext {
	var p = new(BlobContentContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_blobContent
	return p
}

func InitEmptyBlobContentContext(p *BlobContentContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_blobContent
}

func (*BlobContentContext) IsBlobContentContext() {}

func NewBlobContentContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *BlobContentContext {
	var p = new(BlobContentContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_blobContent

	return p
}

func (s *BlobContentContext) GetParser() antlr.Parser { return s.parser }

func (s *BlobContentContext) AllBLOB_CONTENT() []antlr.TerminalNode {
	return s.GetTokens(ASPParserBLOB_CONTENT)
}

func (s *BlobContentContext) BLOB_CONTENT(i int) antlr.TerminalNode {
	return s.GetToken(ASPParserBLOB_CONTENT, i)
}

func (s *BlobContentContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BlobContentContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *BlobContentContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitBlobContent(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) BlobContent() (localctx IBlobContentContext) {
	this := p
	_ = this

	localctx = NewBlobContentContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 16, ASPParserRULE_blobContent)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	p.SetState(88)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == ASPParserBLOB_CONTENT {
		{
			p.SetState(87)
			p.Match(ASPParserBLOB_CONTENT)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

		p.SetState(90)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IHtmlElementContext is an interface to support dynamic dispatch.
type IHtmlElementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	TAG_BEGIN() antlr.TerminalNode
	AllHtmlTag() []IHtmlTagContext
	HtmlTag(i int) IHtmlTagContext
	AllTAG_CLOSE() []antlr.TerminalNode
	TAG_CLOSE(i int) antlr.TerminalNode
	CLOSE_TAG_BEGIN() antlr.TerminalNode
	AllHtmlAttribute() []IHtmlAttributeContext
	HtmlAttribute(i int) IHtmlAttributeContext
	AllHtmlContent() []IHtmlContentContext
	HtmlContent(i int) IHtmlContentContext
	TAG_SLASH_END() antlr.TerminalNode

	// IsHtmlElementContext differentiates from other interfaces.
	IsHtmlElementContext()
}

type HtmlElementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlElementContext() *HtmlElementContext {
	var p = new(HtmlElementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_htmlElement
	return p
}

func InitEmptyHtmlElementContext(p *HtmlElementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_htmlElement
}

func (*HtmlElementContext) IsHtmlElementContext() {}

func NewHtmlElementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlElementContext {
	var p = new(HtmlElementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_htmlElement

	return p
}

func (s *HtmlElementContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlElementContext) TAG_BEGIN() antlr.TerminalNode {
	return s.GetToken(ASPParserTAG_BEGIN, 0)
}

func (s *HtmlElementContext) AllHtmlTag() []IHtmlTagContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlTagContext); ok {
			len++
		}
	}

	tst := make([]IHtmlTagContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlTagContext); ok {
			tst[i] = t.(IHtmlTagContext)
			i++
		}
	}

	return tst
}

func (s *HtmlElementContext) HtmlTag(i int) IHtmlTagContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlTagContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlTagContext)
}

func (s *HtmlElementContext) AllTAG_CLOSE() []antlr.TerminalNode {
	return s.GetTokens(ASPParserTAG_CLOSE)
}

func (s *HtmlElementContext) TAG_CLOSE(i int) antlr.TerminalNode {
	return s.GetToken(ASPParserTAG_CLOSE, i)
}

func (s *HtmlElementContext) CLOSE_TAG_BEGIN() antlr.TerminalNode {
	return s.GetToken(ASPParserCLOSE_TAG_BEGIN, 0)
}

func (s *HtmlElementContext) AllHtmlAttribute() []IHtmlAttributeContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlAttributeContext); ok {
			len++
		}
	}

	tst := make([]IHtmlAttributeContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlAttributeContext); ok {
			tst[i] = t.(IHtmlAttributeContext)
			i++
		}
	}

	return tst
}

func (s *HtmlElementContext) HtmlAttribute(i int) IHtmlAttributeContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlAttributeContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlAttributeContext)
}

func (s *HtmlElementContext) AllHtmlContent() []IHtmlContentContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHtmlContentContext); ok {
			len++
		}
	}

	tst := make([]IHtmlContentContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHtmlContentContext); ok {
			tst[i] = t.(IHtmlContentContext)
			i++
		}
	}

	return tst
}

func (s *HtmlElementContext) HtmlContent(i int) IHtmlContentContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlContentContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlContentContext)
}

func (s *HtmlElementContext) TAG_SLASH_END() antlr.TerminalNode {
	return s.GetToken(ASPParserTAG_SLASH_END, 0)
}

func (s *HtmlElementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlElementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlElementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitHtmlElement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) HtmlElement() (localctx IHtmlElementContext) {
	this := p
	_ = this

	localctx = NewHtmlElementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 18, ASPParserRULE_htmlElement)
	var _la int

	var _alt int

	p.SetState(131)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 13, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(92)
			p.Match(ASPParserTAG_BEGIN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(93)
			p.HtmlTag()
		}
		p.SetState(97)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ASPParserTAG_IDENTIFIER {
			{
				p.SetState(94)
				p.HtmlAttribute()
			}

			p.SetState(99)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(100)
			p.Match(ASPParserTAG_CLOSE)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		p.SetState(104)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 10, p.GetParserRuleContext())
		if p.HasError() {
			goto errorExit
		}
		for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			if _alt == 1 {
				{
					p.SetState(101)
					p.HtmlContent()
				}

			}
			p.SetState(106)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_alt = p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 10, p.GetParserRuleContext())
			if p.HasError() {
				goto errorExit
			}
		}
		{
			p.SetState(107)
			p.Match(ASPParserCLOSE_TAG_BEGIN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(108)
			p.HtmlTag()
		}
		{
			p.SetState(109)
			p.Match(ASPParserTAG_CLOSE)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(111)
			p.Match(ASPParserTAG_BEGIN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(112)
			p.HtmlTag()
		}
		p.SetState(116)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ASPParserTAG_IDENTIFIER {
			{
				p.SetState(113)
				p.HtmlAttribute()
			}

			p.SetState(118)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(119)
			p.Match(ASPParserTAG_SLASH_END)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(121)
			p.Match(ASPParserTAG_BEGIN)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(122)
			p.HtmlTag()
		}
		p.SetState(126)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)

		for _la == ASPParserTAG_IDENTIFIER {
			{
				p.SetState(123)
				p.HtmlAttribute()
			}

			p.SetState(128)
			p.GetErrorHandler().Sync(p)
			if p.HasError() {
				goto errorExit
			}
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(129)
			p.Match(ASPParserTAG_CLOSE)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case antlr.ATNInvalidAltNumber:
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IHtmlCloseElementContext is an interface to support dynamic dispatch.
type IHtmlCloseElementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	CLOSE_TAG_BEGIN() antlr.TerminalNode
	HtmlTag() IHtmlTagContext
	TAG_CLOSE() antlr.TerminalNode

	// IsHtmlCloseElementContext differentiates from other interfaces.
	IsHtmlCloseElementContext()
}

type HtmlCloseElementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlCloseElementContext() *HtmlCloseElementContext {
	var p = new(HtmlCloseElementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_htmlCloseElement
	return p
}

func InitEmptyHtmlCloseElementContext(p *HtmlCloseElementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_htmlCloseElement
}

func (*HtmlCloseElementContext) IsHtmlCloseElementContext() {}

func NewHtmlCloseElementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlCloseElementContext {
	var p = new(HtmlCloseElementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_htmlCloseElement

	return p
}

func (s *HtmlCloseElementContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlCloseElementContext) CLOSE_TAG_BEGIN() antlr.TerminalNode {
	return s.GetToken(ASPParserCLOSE_TAG_BEGIN, 0)
}

func (s *HtmlCloseElementContext) HtmlTag() IHtmlTagContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlTagContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlTagContext)
}

func (s *HtmlCloseElementContext) TAG_CLOSE() antlr.TerminalNode {
	return s.GetToken(ASPParserTAG_CLOSE, 0)
}

func (s *HtmlCloseElementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlCloseElementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlCloseElementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitHtmlCloseElement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) HtmlCloseElement() (localctx IHtmlCloseElementContext) {
	this := p
	_ = this

	localctx = NewHtmlCloseElementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 20, ASPParserRULE_htmlCloseElement)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(133)
		p.Match(ASPParserCLOSE_TAG_BEGIN)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(134)
		p.HtmlTag()
	}
	{
		p.SetState(135)
		p.Match(ASPParserTAG_CLOSE)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IHtmlTagContext is an interface to support dynamic dispatch.
type IHtmlTagContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	TAG_IDENTIFIER() antlr.TerminalNode

	// IsHtmlTagContext differentiates from other interfaces.
	IsHtmlTagContext()
}

type HtmlTagContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlTagContext() *HtmlTagContext {
	var p = new(HtmlTagContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_htmlTag
	return p
}

func InitEmptyHtmlTagContext(p *HtmlTagContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_htmlTag
}

func (*HtmlTagContext) IsHtmlTagContext() {}

func NewHtmlTagContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlTagContext {
	var p = new(HtmlTagContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_htmlTag

	return p
}

func (s *HtmlTagContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlTagContext) TAG_IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ASPParserTAG_IDENTIFIER, 0)
}

func (s *HtmlTagContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlTagContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlTagContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitHtmlTag(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) HtmlTag() (localctx IHtmlTagContext) {
	this := p
	_ = this

	localctx = NewHtmlTagContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 22, ASPParserRULE_htmlTag)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(137)
		p.Match(ASPParserTAG_IDENTIFIER)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IHtmlAttributeContext is an interface to support dynamic dispatch.
type IHtmlAttributeContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	TAG_IDENTIFIER() antlr.TerminalNode
	TAG_EQUALS() antlr.TerminalNode
	ATTVAL_VALUE() antlr.TerminalNode
	AspScript() IAspScriptContext

	// IsHtmlAttributeContext differentiates from other interfaces.
	IsHtmlAttributeContext()
}

type HtmlAttributeContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlAttributeContext() *HtmlAttributeContext {
	var p = new(HtmlAttributeContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_htmlAttribute
	return p
}

func InitEmptyHtmlAttributeContext(p *HtmlAttributeContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_htmlAttribute
}

func (*HtmlAttributeContext) IsHtmlAttributeContext() {}

func NewHtmlAttributeContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlAttributeContext {
	var p = new(HtmlAttributeContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_htmlAttribute

	return p
}

func (s *HtmlAttributeContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlAttributeContext) TAG_IDENTIFIER() antlr.TerminalNode {
	return s.GetToken(ASPParserTAG_IDENTIFIER, 0)
}

func (s *HtmlAttributeContext) TAG_EQUALS() antlr.TerminalNode {
	return s.GetToken(ASPParserTAG_EQUALS, 0)
}

func (s *HtmlAttributeContext) ATTVAL_VALUE() antlr.TerminalNode {
	return s.GetToken(ASPParserATTVAL_VALUE, 0)
}

func (s *HtmlAttributeContext) AspScript() IAspScriptContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAspScriptContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAspScriptContext)
}

func (s *HtmlAttributeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlAttributeContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlAttributeContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitHtmlAttribute(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) HtmlAttribute() (localctx IHtmlAttributeContext) {
	this := p
	_ = this

	localctx = NewHtmlAttributeContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 24, ASPParserRULE_htmlAttribute)
	p.SetState(146)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetInterpreter().AdaptivePredict(p.BaseParser, p.GetTokenStream(), 14, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(139)
			p.Match(ASPParserTAG_IDENTIFIER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(140)
			p.Match(ASPParserTAG_EQUALS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(141)
			p.Match(ASPParserATTVAL_VALUE)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(142)
			p.Match(ASPParserTAG_IDENTIFIER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(143)
			p.Match(ASPParserTAG_EQUALS)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(144)
			p.AspScript()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(145)
			p.Match(ASPParserTAG_IDENTIFIER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case antlr.ATNInvalidAltNumber:
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IHtmlContentContext is an interface to support dynamic dispatch.
type IHtmlContentContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	HtmlMisc() IHtmlMiscContext
	HtmlElement() IHtmlElementContext
	HtmlCloseElement() IHtmlCloseElementContext
	AspScript() IAspScriptContext
	Script() IScriptContext
	Style() IStyleContext

	// IsHtmlContentContext differentiates from other interfaces.
	IsHtmlContentContext()
}

type HtmlContentContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlContentContext() *HtmlContentContext {
	var p = new(HtmlContentContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_htmlContent
	return p
}

func InitEmptyHtmlContentContext(p *HtmlContentContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_htmlContent
}

func (*HtmlContentContext) IsHtmlContentContext() {}

func NewHtmlContentContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlContentContext {
	var p = new(HtmlContentContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_htmlContent

	return p
}

func (s *HtmlContentContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlContentContext) HtmlMisc() IHtmlMiscContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlMiscContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlMiscContext)
}

func (s *HtmlContentContext) HtmlElement() IHtmlElementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlElementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlElementContext)
}

func (s *HtmlContentContext) HtmlCloseElement() IHtmlCloseElementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHtmlCloseElementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IHtmlCloseElementContext)
}

func (s *HtmlContentContext) AspScript() IAspScriptContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAspScriptContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAspScriptContext)
}

func (s *HtmlContentContext) Script() IScriptContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IScriptContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IScriptContext)
}

func (s *HtmlContentContext) Style() IStyleContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStyleContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStyleContext)
}

func (s *HtmlContentContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlContentContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlContentContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitHtmlContent(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) HtmlContent() (localctx IHtmlContentContext) {
	this := p
	_ = this

	localctx = NewHtmlContentContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 26, ASPParserRULE_htmlContent)
	p.SetState(154)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetTokenStream().LA(1) {
	case ASPParserWHITESPACES, ASPParserASP_STATIC_CONTENT_CHARS:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(148)
			p.HtmlMisc()
		}

	case ASPParserTAG_BEGIN:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(149)
			p.HtmlElement()
		}

	case ASPParserCLOSE_TAG_BEGIN:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(150)
			p.HtmlCloseElement()
		}

	case ASPParserDIRECTIVE_BEGIN, ASPParserDECLARATION_BEGIN, ASPParserECHO_EXPRESSION_OPEN, ASPParserDATABIND_OPEN, ASPParserSCRIPTLET_OPEN:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(151)
			p.AspScript()
		}

	case ASPParserSCRIPT_OPEN:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(152)
			p.Script()
		}

	case ASPParserSTYLE_OPEN:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(153)
			p.Style()
		}

	default:
		p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IHtmlMiscContext is an interface to support dynamic dispatch.
type IHtmlMiscContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ASP_STATIC_CONTENT_CHARS() antlr.TerminalNode
	WHITESPACES() antlr.TerminalNode

	// IsHtmlMiscContext differentiates from other interfaces.
	IsHtmlMiscContext()
}

type HtmlMiscContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHtmlMiscContext() *HtmlMiscContext {
	var p = new(HtmlMiscContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_htmlMisc
	return p
}

func InitEmptyHtmlMiscContext(p *HtmlMiscContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_htmlMisc
}

func (*HtmlMiscContext) IsHtmlMiscContext() {}

func NewHtmlMiscContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *HtmlMiscContext {
	var p = new(HtmlMiscContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_htmlMisc

	return p
}

func (s *HtmlMiscContext) GetParser() antlr.Parser { return s.parser }

func (s *HtmlMiscContext) ASP_STATIC_CONTENT_CHARS() antlr.TerminalNode {
	return s.GetToken(ASPParserASP_STATIC_CONTENT_CHARS, 0)
}

func (s *HtmlMiscContext) WHITESPACES() antlr.TerminalNode {
	return s.GetToken(ASPParserWHITESPACES, 0)
}

func (s *HtmlMiscContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *HtmlMiscContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *HtmlMiscContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitHtmlMisc(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) HtmlMisc() (localctx IHtmlMiscContext) {
	this := p
	_ = this

	localctx = NewHtmlMiscContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 28, ASPParserRULE_htmlMisc)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(156)
		_la = p.GetTokenStream().LA(1)

		if !(_la == ASPParserWHITESPACES || _la == ASPParserASP_STATIC_CONTENT_CHARS) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IScriptContext is an interface to support dynamic dispatch.
type IScriptContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	SCRIPT_OPEN() antlr.TerminalNode
	SCRIPT_BODY() antlr.TerminalNode

	// IsScriptContext differentiates from other interfaces.
	IsScriptContext()
}

type ScriptContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyScriptContext() *ScriptContext {
	var p = new(ScriptContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_script
	return p
}

func InitEmptyScriptContext(p *ScriptContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_script
}

func (*ScriptContext) IsScriptContext() {}

func NewScriptContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ScriptContext {
	var p = new(ScriptContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_script

	return p
}

func (s *ScriptContext) GetParser() antlr.Parser { return s.parser }

func (s *ScriptContext) SCRIPT_OPEN() antlr.TerminalNode {
	return s.GetToken(ASPParserSCRIPT_OPEN, 0)
}

func (s *ScriptContext) SCRIPT_BODY() antlr.TerminalNode {
	return s.GetToken(ASPParserSCRIPT_BODY, 0)
}

func (s *ScriptContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ScriptContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ScriptContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitScript(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) Script() (localctx IScriptContext) {
	this := p
	_ = this

	localctx = NewScriptContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 30, ASPParserRULE_script)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(158)
		p.Match(ASPParserSCRIPT_OPEN)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(159)
		p.Match(ASPParserSCRIPT_BODY)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IStyleContext is an interface to support dynamic dispatch.
type IStyleContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	STYLE_OPEN() antlr.TerminalNode
	STYLE_BODY() antlr.TerminalNode

	// IsStyleContext differentiates from other interfaces.
	IsStyleContext()
}

type StyleContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStyleContext() *StyleContext {
	var p = new(StyleContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_style
	return p
}

func InitEmptyStyleContext(p *StyleContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ASPParserRULE_style
}

func (*StyleContext) IsStyleContext() {}

func NewStyleContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StyleContext {
	var p = new(StyleContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ASPParserRULE_style

	return p
}

func (s *StyleContext) GetParser() antlr.Parser { return s.parser }

func (s *StyleContext) STYLE_OPEN() antlr.TerminalNode {
	return s.GetToken(ASPParserSTYLE_OPEN, 0)
}

func (s *StyleContext) STYLE_BODY() antlr.TerminalNode {
	return s.GetToken(ASPParserSTYLE_BODY, 0)
}

func (s *StyleContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StyleContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StyleContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ASPParserVisitor:
		return t.VisitStyle(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ASPParser) Style() (localctx IStyleContext) {
	this := p
	_ = this

	localctx = NewStyleContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 32, ASPParserRULE_style)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(161)
		p.Match(ASPParserSTYLE_OPEN)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(162)
		p.Match(ASPParserSTYLE_BODY)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}
