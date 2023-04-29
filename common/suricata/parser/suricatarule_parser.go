// Code generated from ./SuricataRuleParser.g4 by ANTLR 4.12.0. DO NOT EDIT.

package parser // SuricataRuleParser

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

// Suppress unused import errors
var _ = fmt.Printf
var _ = strconv.Itoa
var _ = sync.Once{}

type SuricataRuleParser struct {
	*antlr.BaseParser
}

var suricataruleparserParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	literalNames           []string
	symbolicNames          []string
	ruleNames              []string
	predictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func suricataruleparserParserInit() {
	staticData := &suricataruleparserParserStaticData
	staticData.literalNames = []string{
		"", "'any'", "'!'", "'$'", "'->'", "'*'", "'/'", "'%'", "'&'", "'+'",
		"'-'", "'^'", "'<'", "'>'", "'<='", "'>='", "':'", "'::'", "'['", "']'",
		"'('", "'{'", "'}'", "','", "'='", "'~'", "'.'", "", "", "", "", "",
		"", "", "", "", "", "';'", "", "')'",
	}
	staticData.symbolicNames = []string{
		"", "Any", "Negative", "Dollar", "Arrow", "Mul", "Div", "Mod", "Amp",
		"Plus", "Sub", "Power", "Lt", "Gt", "LtEq", "GtEq", "Colon", "DoubleColon",
		"LBracket", "RBracket", "ParamStart", "LBrace", "RBrace", "Comma", "Eq",
		"NotSymbol", "Dot", "LINE_COMMENT", "ID", "NORMALSTRING", "INT", "HEX",
		"FLOAT", "WS", "NonSemiColon", "SHEBANG", "ParamQuotedString", "ParamSep",
		"ParamValue", "ParamEnd",
	}
	staticData.ruleNames = []string{
		"rules", "rule", "action", "protocol", "src_address", "dest_address",
		"address", "ipv4", "ipv4block", "ipv4mask", "environment_var", "ipv6",
		"hex_part", "h16", "src_port", "dest_port", "port", "params", "param",
		"string",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 39, 185, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 1, 0, 4, 0, 42,
		8, 0, 11, 0, 12, 0, 43, 1, 0, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 2, 1, 2, 1, 3, 1, 3, 1, 4, 1, 4, 1, 5, 1, 5, 1, 6,
		1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 5, 6, 75, 8, 6, 10,
		6, 12, 6, 78, 9, 6, 1, 6, 1, 6, 3, 6, 82, 8, 6, 1, 7, 1, 7, 1, 7, 1, 7,
		1, 7, 1, 7, 1, 7, 1, 7, 1, 7, 3, 7, 93, 8, 7, 1, 8, 1, 8, 1, 9, 1, 9, 1,
		10, 1, 10, 1, 10, 1, 11, 1, 11, 1, 11, 5, 11, 105, 8, 11, 10, 11, 12, 11,
		108, 9, 11, 1, 11, 1, 11, 1, 11, 1, 11, 5, 11, 114, 8, 11, 10, 11, 12,
		11, 117, 9, 11, 1, 11, 3, 11, 120, 8, 11, 1, 12, 1, 12, 3, 12, 124, 8,
		12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 3, 12, 132, 8, 12, 1, 13,
		1, 13, 1, 14, 1, 14, 1, 15, 1, 15, 1, 16, 1, 16, 1, 16, 1, 16, 1, 16, 3,
		16, 145, 8, 16, 1, 16, 1, 16, 1, 16, 1, 16, 1, 16, 1, 16, 1, 16, 1, 16,
		5, 16, 155, 8, 16, 10, 16, 12, 16, 158, 9, 16, 1, 16, 1, 16, 1, 16, 3,
		16, 163, 8, 16, 1, 17, 1, 17, 1, 17, 1, 17, 5, 17, 169, 8, 17, 10, 17,
		12, 17, 172, 9, 17, 1, 17, 3, 17, 175, 8, 17, 1, 17, 1, 17, 1, 18, 1, 18,
		3, 18, 181, 8, 18, 1, 19, 1, 19, 1, 19, 0, 0, 20, 0, 2, 4, 6, 8, 10, 12,
		14, 16, 18, 20, 22, 24, 26, 28, 30, 32, 34, 36, 38, 0, 0, 189, 0, 41, 1,
		0, 0, 0, 2, 47, 1, 0, 0, 0, 4, 56, 1, 0, 0, 0, 6, 58, 1, 0, 0, 0, 8, 60,
		1, 0, 0, 0, 10, 62, 1, 0, 0, 0, 12, 81, 1, 0, 0, 0, 14, 83, 1, 0, 0, 0,
		16, 94, 1, 0, 0, 0, 18, 96, 1, 0, 0, 0, 20, 98, 1, 0, 0, 0, 22, 101, 1,
		0, 0, 0, 24, 131, 1, 0, 0, 0, 26, 133, 1, 0, 0, 0, 28, 135, 1, 0, 0, 0,
		30, 137, 1, 0, 0, 0, 32, 162, 1, 0, 0, 0, 34, 164, 1, 0, 0, 0, 36, 178,
		1, 0, 0, 0, 38, 182, 1, 0, 0, 0, 40, 42, 3, 2, 1, 0, 41, 40, 1, 0, 0, 0,
		42, 43, 1, 0, 0, 0, 43, 41, 1, 0, 0, 0, 43, 44, 1, 0, 0, 0, 44, 45, 1,
		0, 0, 0, 45, 46, 5, 0, 0, 1, 46, 1, 1, 0, 0, 0, 47, 48, 3, 4, 2, 0, 48,
		49, 3, 6, 3, 0, 49, 50, 3, 8, 4, 0, 50, 51, 3, 28, 14, 0, 51, 52, 5, 4,
		0, 0, 52, 53, 3, 10, 5, 0, 53, 54, 3, 30, 15, 0, 54, 55, 3, 34, 17, 0,
		55, 3, 1, 0, 0, 0, 56, 57, 5, 28, 0, 0, 57, 5, 1, 0, 0, 0, 58, 59, 5, 28,
		0, 0, 59, 7, 1, 0, 0, 0, 60, 61, 3, 12, 6, 0, 61, 9, 1, 0, 0, 0, 62, 63,
		3, 12, 6, 0, 63, 11, 1, 0, 0, 0, 64, 82, 5, 1, 0, 0, 65, 66, 5, 2, 0, 0,
		66, 82, 3, 12, 6, 0, 67, 82, 3, 20, 10, 0, 68, 82, 3, 14, 7, 0, 69, 82,
		3, 22, 11, 0, 70, 71, 5, 18, 0, 0, 71, 76, 3, 12, 6, 0, 72, 73, 5, 23,
		0, 0, 73, 75, 3, 12, 6, 0, 74, 72, 1, 0, 0, 0, 75, 78, 1, 0, 0, 0, 76,
		74, 1, 0, 0, 0, 76, 77, 1, 0, 0, 0, 77, 79, 1, 0, 0, 0, 78, 76, 1, 0, 0,
		0, 79, 80, 5, 19, 0, 0, 80, 82, 1, 0, 0, 0, 81, 64, 1, 0, 0, 0, 81, 65,
		1, 0, 0, 0, 81, 67, 1, 0, 0, 0, 81, 68, 1, 0, 0, 0, 81, 69, 1, 0, 0, 0,
		81, 70, 1, 0, 0, 0, 82, 13, 1, 0, 0, 0, 83, 84, 3, 16, 8, 0, 84, 85, 5,
		26, 0, 0, 85, 86, 3, 16, 8, 0, 86, 87, 5, 26, 0, 0, 87, 88, 3, 16, 8, 0,
		88, 89, 5, 26, 0, 0, 89, 92, 3, 16, 8, 0, 90, 91, 5, 6, 0, 0, 91, 93, 3,
		18, 9, 0, 92, 90, 1, 0, 0, 0, 92, 93, 1, 0, 0, 0, 93, 15, 1, 0, 0, 0, 94,
		95, 5, 30, 0, 0, 95, 17, 1, 0, 0, 0, 96, 97, 5, 30, 0, 0, 97, 19, 1, 0,
		0, 0, 98, 99, 5, 3, 0, 0, 99, 100, 5, 28, 0, 0, 100, 21, 1, 0, 0, 0, 101,
		106, 3, 24, 12, 0, 102, 103, 5, 16, 0, 0, 103, 105, 3, 24, 12, 0, 104,
		102, 1, 0, 0, 0, 105, 108, 1, 0, 0, 0, 106, 104, 1, 0, 0, 0, 106, 107,
		1, 0, 0, 0, 107, 119, 1, 0, 0, 0, 108, 106, 1, 0, 0, 0, 109, 115, 5, 17,
		0, 0, 110, 111, 3, 24, 12, 0, 111, 112, 5, 16, 0, 0, 112, 114, 1, 0, 0,
		0, 113, 110, 1, 0, 0, 0, 114, 117, 1, 0, 0, 0, 115, 113, 1, 0, 0, 0, 115,
		116, 1, 0, 0, 0, 116, 118, 1, 0, 0, 0, 117, 115, 1, 0, 0, 0, 118, 120,
		3, 24, 12, 0, 119, 109, 1, 0, 0, 0, 119, 120, 1, 0, 0, 0, 120, 23, 1, 0,
		0, 0, 121, 132, 3, 26, 13, 0, 122, 124, 3, 26, 13, 0, 123, 122, 1, 0, 0,
		0, 123, 124, 1, 0, 0, 0, 124, 125, 1, 0, 0, 0, 125, 126, 5, 16, 0, 0, 126,
		127, 5, 16, 0, 0, 127, 132, 3, 26, 13, 0, 128, 129, 5, 16, 0, 0, 129, 130,
		5, 16, 0, 0, 130, 132, 3, 26, 13, 0, 131, 121, 1, 0, 0, 0, 131, 123, 1,
		0, 0, 0, 131, 128, 1, 0, 0, 0, 132, 25, 1, 0, 0, 0, 133, 134, 5, 31, 0,
		0, 134, 27, 1, 0, 0, 0, 135, 136, 3, 32, 16, 0, 136, 29, 1, 0, 0, 0, 137,
		138, 3, 32, 16, 0, 138, 31, 1, 0, 0, 0, 139, 163, 5, 1, 0, 0, 140, 163,
		5, 30, 0, 0, 141, 142, 5, 30, 0, 0, 142, 144, 5, 16, 0, 0, 143, 145, 5,
		30, 0, 0, 144, 143, 1, 0, 0, 0, 144, 145, 1, 0, 0, 0, 145, 163, 1, 0, 0,
		0, 146, 147, 5, 16, 0, 0, 147, 163, 5, 30, 0, 0, 148, 149, 5, 2, 0, 0,
		149, 163, 3, 32, 16, 0, 150, 151, 5, 18, 0, 0, 151, 156, 3, 32, 16, 0,
		152, 153, 5, 23, 0, 0, 153, 155, 3, 32, 16, 0, 154, 152, 1, 0, 0, 0, 155,
		158, 1, 0, 0, 0, 156, 154, 1, 0, 0, 0, 156, 157, 1, 0, 0, 0, 157, 159,
		1, 0, 0, 0, 158, 156, 1, 0, 0, 0, 159, 160, 5, 19, 0, 0, 160, 163, 1, 0,
		0, 0, 161, 163, 3, 20, 10, 0, 162, 139, 1, 0, 0, 0, 162, 140, 1, 0, 0,
		0, 162, 141, 1, 0, 0, 0, 162, 146, 1, 0, 0, 0, 162, 148, 1, 0, 0, 0, 162,
		150, 1, 0, 0, 0, 162, 161, 1, 0, 0, 0, 163, 33, 1, 0, 0, 0, 164, 165, 5,
		20, 0, 0, 165, 170, 3, 36, 18, 0, 166, 167, 5, 37, 0, 0, 167, 169, 3, 36,
		18, 0, 168, 166, 1, 0, 0, 0, 169, 172, 1, 0, 0, 0, 170, 168, 1, 0, 0, 0,
		170, 171, 1, 0, 0, 0, 171, 174, 1, 0, 0, 0, 172, 170, 1, 0, 0, 0, 173,
		175, 5, 37, 0, 0, 174, 173, 1, 0, 0, 0, 174, 175, 1, 0, 0, 0, 175, 176,
		1, 0, 0, 0, 176, 177, 5, 39, 0, 0, 177, 35, 1, 0, 0, 0, 178, 180, 5, 38,
		0, 0, 179, 181, 3, 38, 19, 0, 180, 179, 1, 0, 0, 0, 180, 181, 1, 0, 0,
		0, 181, 37, 1, 0, 0, 0, 182, 183, 5, 36, 0, 0, 183, 39, 1, 0, 0, 0, 15,
		43, 76, 81, 92, 106, 115, 119, 123, 131, 144, 156, 162, 170, 174, 180,
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

// SuricataRuleParserInit initializes any static state used to implement SuricataRuleParser. By default the
// static state used to implement the parser is lazily initialized during the first call to
// NewSuricataRuleParser(). You can call this function if you wish to initialize the static state ahead
// of time.
func SuricataRuleParserInit() {
	staticData := &suricataruleparserParserStaticData
	staticData.once.Do(suricataruleparserParserInit)
}

// NewSuricataRuleParser produces a new parser instance for the optional input antlr.TokenStream.
func NewSuricataRuleParser(input antlr.TokenStream) *SuricataRuleParser {
	SuricataRuleParserInit()
	this := new(SuricataRuleParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &suricataruleparserParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.predictionContextCache)
	this.RuleNames = staticData.ruleNames
	this.LiteralNames = staticData.literalNames
	this.SymbolicNames = staticData.symbolicNames
	this.GrammarFileName = "SuricataRuleParser.g4"

	return this
}

// SuricataRuleParser tokens.
const (
	SuricataRuleParserEOF               = antlr.TokenEOF
	SuricataRuleParserAny               = 1
	SuricataRuleParserNegative          = 2
	SuricataRuleParserDollar            = 3
	SuricataRuleParserArrow             = 4
	SuricataRuleParserMul               = 5
	SuricataRuleParserDiv               = 6
	SuricataRuleParserMod               = 7
	SuricataRuleParserAmp               = 8
	SuricataRuleParserPlus              = 9
	SuricataRuleParserSub               = 10
	SuricataRuleParserPower             = 11
	SuricataRuleParserLt                = 12
	SuricataRuleParserGt                = 13
	SuricataRuleParserLtEq              = 14
	SuricataRuleParserGtEq              = 15
	SuricataRuleParserColon             = 16
	SuricataRuleParserDoubleColon       = 17
	SuricataRuleParserLBracket          = 18
	SuricataRuleParserRBracket          = 19
	SuricataRuleParserParamStart        = 20
	SuricataRuleParserLBrace            = 21
	SuricataRuleParserRBrace            = 22
	SuricataRuleParserComma             = 23
	SuricataRuleParserEq                = 24
	SuricataRuleParserNotSymbol         = 25
	SuricataRuleParserDot               = 26
	SuricataRuleParserLINE_COMMENT      = 27
	SuricataRuleParserID                = 28
	SuricataRuleParserNORMALSTRING      = 29
	SuricataRuleParserINT               = 30
	SuricataRuleParserHEX               = 31
	SuricataRuleParserFLOAT             = 32
	SuricataRuleParserWS                = 33
	SuricataRuleParserNonSemiColon      = 34
	SuricataRuleParserSHEBANG           = 35
	SuricataRuleParserParamQuotedString = 36
	SuricataRuleParserParamSep          = 37
	SuricataRuleParserParamValue        = 38
	SuricataRuleParserParamEnd          = 39
)

// SuricataRuleParser rules.
const (
	SuricataRuleParserRULE_rules           = 0
	SuricataRuleParserRULE_rule            = 1
	SuricataRuleParserRULE_action          = 2
	SuricataRuleParserRULE_protocol        = 3
	SuricataRuleParserRULE_src_address     = 4
	SuricataRuleParserRULE_dest_address    = 5
	SuricataRuleParserRULE_address         = 6
	SuricataRuleParserRULE_ipv4            = 7
	SuricataRuleParserRULE_ipv4block       = 8
	SuricataRuleParserRULE_ipv4mask        = 9
	SuricataRuleParserRULE_environment_var = 10
	SuricataRuleParserRULE_ipv6            = 11
	SuricataRuleParserRULE_hex_part        = 12
	SuricataRuleParserRULE_h16             = 13
	SuricataRuleParserRULE_src_port        = 14
	SuricataRuleParserRULE_dest_port       = 15
	SuricataRuleParserRULE_port            = 16
	SuricataRuleParserRULE_params          = 17
	SuricataRuleParserRULE_param           = 18
	SuricataRuleParserRULE_string          = 19
)

// IRulesContext is an interface to support dynamic dispatch.
type IRulesContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	EOF() antlr.TerminalNode
	AllRule_() []IRuleContext
	Rule_(i int) IRuleContext

	// IsRulesContext differentiates from other interfaces.
	IsRulesContext()
}

type RulesContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyRulesContext() *RulesContext {
	var p = new(RulesContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_rules
	return p
}

func (*RulesContext) IsRulesContext() {}

func NewRulesContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *RulesContext {
	var p = new(RulesContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_rules

	return p
}

func (s *RulesContext) GetParser() antlr.Parser { return s.parser }

func (s *RulesContext) EOF() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserEOF, 0)
}

func (s *RulesContext) AllRule_() []IRuleContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IRuleContext); ok {
			len++
		}
	}

	tst := make([]IRuleContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IRuleContext); ok {
			tst[i] = t.(IRuleContext)
			i++
		}
	}

	return tst
}

func (s *RulesContext) Rule_(i int) IRuleContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRuleContext); ok {
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

	return t.(IRuleContext)
}

func (s *RulesContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RulesContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *RulesContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitRules(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Rules() (localctx IRulesContext) {
	this := p
	_ = this

	localctx = NewRulesContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, SuricataRuleParserRULE_rules)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	p.SetState(41)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == SuricataRuleParserID {
		{
			p.SetState(40)
			p.Rule_()
		}

		p.SetState(43)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(45)
		p.Match(SuricataRuleParserEOF)
	}

	return localctx
}

// IRuleContext is an interface to support dynamic dispatch.
type IRuleContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Action_() IActionContext
	Protocol() IProtocolContext
	Src_address() ISrc_addressContext
	Src_port() ISrc_portContext
	Arrow() antlr.TerminalNode
	Dest_address() IDest_addressContext
	Dest_port() IDest_portContext
	Params() IParamsContext

	// IsRuleContext differentiates from other interfaces.
	IsRuleContext()
}

type RuleContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyRuleContext() *RuleContext {
	var p = new(RuleContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_rule
	return p
}

func (*RuleContext) IsRuleContext() {}

func NewRuleContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *RuleContext {
	var p = new(RuleContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_rule

	return p
}

func (s *RuleContext) GetParser() antlr.Parser { return s.parser }

func (s *RuleContext) Action_() IActionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IActionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IActionContext)
}

func (s *RuleContext) Protocol() IProtocolContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IProtocolContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IProtocolContext)
}

func (s *RuleContext) Src_address() ISrc_addressContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISrc_addressContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISrc_addressContext)
}

func (s *RuleContext) Src_port() ISrc_portContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISrc_portContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISrc_portContext)
}

func (s *RuleContext) Arrow() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserArrow, 0)
}

func (s *RuleContext) Dest_address() IDest_addressContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDest_addressContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDest_addressContext)
}

func (s *RuleContext) Dest_port() IDest_portContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IDest_portContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IDest_portContext)
}

func (s *RuleContext) Params() IParamsContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IParamsContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IParamsContext)
}

func (s *RuleContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RuleContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *RuleContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitRule(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Rule_() (localctx IRuleContext) {
	this := p
	_ = this

	localctx = NewRuleContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 2, SuricataRuleParserRULE_rule)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(47)
		p.Action_()
	}
	{
		p.SetState(48)
		p.Protocol()
	}
	{
		p.SetState(49)
		p.Src_address()
	}
	{
		p.SetState(50)
		p.Src_port()
	}
	{
		p.SetState(51)
		p.Match(SuricataRuleParserArrow)
	}
	{
		p.SetState(52)
		p.Dest_address()
	}
	{
		p.SetState(53)
		p.Dest_port()
	}
	{
		p.SetState(54)
		p.Params()
	}

	return localctx
}

// IActionContext is an interface to support dynamic dispatch.
type IActionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ID() antlr.TerminalNode

	// IsActionContext differentiates from other interfaces.
	IsActionContext()
}

type ActionContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyActionContext() *ActionContext {
	var p = new(ActionContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_action
	return p
}

func (*ActionContext) IsActionContext() {}

func NewActionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ActionContext {
	var p = new(ActionContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_action

	return p
}

func (s *ActionContext) GetParser() antlr.Parser { return s.parser }

func (s *ActionContext) ID() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserID, 0)
}

func (s *ActionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ActionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ActionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitAction(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Action_() (localctx IActionContext) {
	this := p
	_ = this

	localctx = NewActionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, SuricataRuleParserRULE_action)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(56)
		p.Match(SuricataRuleParserID)
	}

	return localctx
}

// IProtocolContext is an interface to support dynamic dispatch.
type IProtocolContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ID() antlr.TerminalNode

	// IsProtocolContext differentiates from other interfaces.
	IsProtocolContext()
}

type ProtocolContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyProtocolContext() *ProtocolContext {
	var p = new(ProtocolContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_protocol
	return p
}

func (*ProtocolContext) IsProtocolContext() {}

func NewProtocolContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ProtocolContext {
	var p = new(ProtocolContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_protocol

	return p
}

func (s *ProtocolContext) GetParser() antlr.Parser { return s.parser }

func (s *ProtocolContext) ID() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserID, 0)
}

func (s *ProtocolContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ProtocolContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ProtocolContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitProtocol(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Protocol() (localctx IProtocolContext) {
	this := p
	_ = this

	localctx = NewProtocolContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 6, SuricataRuleParserRULE_protocol)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(58)
		p.Match(SuricataRuleParserID)
	}

	return localctx
}

// ISrc_addressContext is an interface to support dynamic dispatch.
type ISrc_addressContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Address() IAddressContext

	// IsSrc_addressContext differentiates from other interfaces.
	IsSrc_addressContext()
}

type Src_addressContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySrc_addressContext() *Src_addressContext {
	var p = new(Src_addressContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_src_address
	return p
}

func (*Src_addressContext) IsSrc_addressContext() {}

func NewSrc_addressContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Src_addressContext {
	var p = new(Src_addressContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_src_address

	return p
}

func (s *Src_addressContext) GetParser() antlr.Parser { return s.parser }

func (s *Src_addressContext) Address() IAddressContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAddressContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAddressContext)
}

func (s *Src_addressContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Src_addressContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Src_addressContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitSrc_address(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Src_address() (localctx ISrc_addressContext) {
	this := p
	_ = this

	localctx = NewSrc_addressContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, SuricataRuleParserRULE_src_address)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(60)
		p.Address()
	}

	return localctx
}

// IDest_addressContext is an interface to support dynamic dispatch.
type IDest_addressContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Address() IAddressContext

	// IsDest_addressContext differentiates from other interfaces.
	IsDest_addressContext()
}

type Dest_addressContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDest_addressContext() *Dest_addressContext {
	var p = new(Dest_addressContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_dest_address
	return p
}

func (*Dest_addressContext) IsDest_addressContext() {}

func NewDest_addressContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Dest_addressContext {
	var p = new(Dest_addressContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_dest_address

	return p
}

func (s *Dest_addressContext) GetParser() antlr.Parser { return s.parser }

func (s *Dest_addressContext) Address() IAddressContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAddressContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAddressContext)
}

func (s *Dest_addressContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Dest_addressContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Dest_addressContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitDest_address(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Dest_address() (localctx IDest_addressContext) {
	this := p
	_ = this

	localctx = NewDest_addressContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, SuricataRuleParserRULE_dest_address)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(62)
		p.Address()
	}

	return localctx
}

// IAddressContext is an interface to support dynamic dispatch.
type IAddressContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Any() antlr.TerminalNode
	Negative() antlr.TerminalNode
	AllAddress() []IAddressContext
	Address(i int) IAddressContext
	Environment_var() IEnvironment_varContext
	Ipv4() IIpv4Context
	Ipv6() IIpv6Context
	LBracket() antlr.TerminalNode
	RBracket() antlr.TerminalNode
	AllComma() []antlr.TerminalNode
	Comma(i int) antlr.TerminalNode

	// IsAddressContext differentiates from other interfaces.
	IsAddressContext()
}

type AddressContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAddressContext() *AddressContext {
	var p = new(AddressContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_address
	return p
}

func (*AddressContext) IsAddressContext() {}

func NewAddressContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AddressContext {
	var p = new(AddressContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_address

	return p
}

func (s *AddressContext) GetParser() antlr.Parser { return s.parser }

func (s *AddressContext) Any() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserAny, 0)
}

func (s *AddressContext) Negative() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserNegative, 0)
}

func (s *AddressContext) AllAddress() []IAddressContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IAddressContext); ok {
			len++
		}
	}

	tst := make([]IAddressContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IAddressContext); ok {
			tst[i] = t.(IAddressContext)
			i++
		}
	}

	return tst
}

func (s *AddressContext) Address(i int) IAddressContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAddressContext); ok {
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

	return t.(IAddressContext)
}

func (s *AddressContext) Environment_var() IEnvironment_varContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IEnvironment_varContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IEnvironment_varContext)
}

func (s *AddressContext) Ipv4() IIpv4Context {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIpv4Context); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIpv4Context)
}

func (s *AddressContext) Ipv6() IIpv6Context {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIpv6Context); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIpv6Context)
}

func (s *AddressContext) LBracket() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserLBracket, 0)
}

func (s *AddressContext) RBracket() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserRBracket, 0)
}

func (s *AddressContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(SuricataRuleParserComma)
}

func (s *AddressContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserComma, i)
}

func (s *AddressContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AddressContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AddressContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitAddress(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Address() (localctx IAddressContext) {
	this := p
	_ = this

	localctx = NewAddressContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, SuricataRuleParserRULE_address)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(81)
	p.GetErrorHandler().Sync(p)

	switch p.GetTokenStream().LA(1) {
	case SuricataRuleParserAny:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(64)
			p.Match(SuricataRuleParserAny)
		}

	case SuricataRuleParserNegative:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(65)
			p.Match(SuricataRuleParserNegative)
		}
		{
			p.SetState(66)
			p.Address()
		}

	case SuricataRuleParserDollar:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(67)
			p.Environment_var()
		}

	case SuricataRuleParserINT:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(68)
			p.Ipv4()
		}

	case SuricataRuleParserColon, SuricataRuleParserHEX:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(69)
			p.Ipv6()
		}

	case SuricataRuleParserLBracket:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(70)
			p.Match(SuricataRuleParserLBracket)
		}
		{
			p.SetState(71)
			p.Address()
		}
		p.SetState(76)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == SuricataRuleParserComma {
			{
				p.SetState(72)
				p.Match(SuricataRuleParserComma)
			}
			{
				p.SetState(73)
				p.Address()
			}

			p.SetState(78)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(79)
			p.Match(SuricataRuleParserRBracket)
		}

	default:
		panic(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
	}

	return localctx
}

// IIpv4Context is an interface to support dynamic dispatch.
type IIpv4Context interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	AllIpv4block() []IIpv4blockContext
	Ipv4block(i int) IIpv4blockContext
	AllDot() []antlr.TerminalNode
	Dot(i int) antlr.TerminalNode
	Div() antlr.TerminalNode
	Ipv4mask() IIpv4maskContext

	// IsIpv4Context differentiates from other interfaces.
	IsIpv4Context()
}

type Ipv4Context struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyIpv4Context() *Ipv4Context {
	var p = new(Ipv4Context)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_ipv4
	return p
}

func (*Ipv4Context) IsIpv4Context() {}

func NewIpv4Context(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Ipv4Context {
	var p = new(Ipv4Context)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_ipv4

	return p
}

func (s *Ipv4Context) GetParser() antlr.Parser { return s.parser }

func (s *Ipv4Context) AllIpv4block() []IIpv4blockContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IIpv4blockContext); ok {
			len++
		}
	}

	tst := make([]IIpv4blockContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IIpv4blockContext); ok {
			tst[i] = t.(IIpv4blockContext)
			i++
		}
	}

	return tst
}

func (s *Ipv4Context) Ipv4block(i int) IIpv4blockContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIpv4blockContext); ok {
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

	return t.(IIpv4blockContext)
}

func (s *Ipv4Context) AllDot() []antlr.TerminalNode {
	return s.GetTokens(SuricataRuleParserDot)
}

func (s *Ipv4Context) Dot(i int) antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserDot, i)
}

func (s *Ipv4Context) Div() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserDiv, 0)
}

func (s *Ipv4Context) Ipv4mask() IIpv4maskContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIpv4maskContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIpv4maskContext)
}

func (s *Ipv4Context) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Ipv4Context) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Ipv4Context) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitIpv4(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Ipv4() (localctx IIpv4Context) {
	this := p
	_ = this

	localctx = NewIpv4Context(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 14, SuricataRuleParserRULE_ipv4)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(83)
		p.Ipv4block()
	}
	{
		p.SetState(84)
		p.Match(SuricataRuleParserDot)
	}
	{
		p.SetState(85)
		p.Ipv4block()
	}
	{
		p.SetState(86)
		p.Match(SuricataRuleParserDot)
	}
	{
		p.SetState(87)
		p.Ipv4block()
	}
	{
		p.SetState(88)
		p.Match(SuricataRuleParserDot)
	}
	{
		p.SetState(89)
		p.Ipv4block()
	}
	p.SetState(92)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserDiv {
		{
			p.SetState(90)
			p.Match(SuricataRuleParserDiv)
		}
		{
			p.SetState(91)
			p.Ipv4mask()
		}

	}

	return localctx
}

// IIpv4blockContext is an interface to support dynamic dispatch.
type IIpv4blockContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	INT() antlr.TerminalNode

	// IsIpv4blockContext differentiates from other interfaces.
	IsIpv4blockContext()
}

type Ipv4blockContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyIpv4blockContext() *Ipv4blockContext {
	var p = new(Ipv4blockContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_ipv4block
	return p
}

func (*Ipv4blockContext) IsIpv4blockContext() {}

func NewIpv4blockContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Ipv4blockContext {
	var p = new(Ipv4blockContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_ipv4block

	return p
}

func (s *Ipv4blockContext) GetParser() antlr.Parser { return s.parser }

func (s *Ipv4blockContext) INT() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserINT, 0)
}

func (s *Ipv4blockContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Ipv4blockContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Ipv4blockContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitIpv4block(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Ipv4block() (localctx IIpv4blockContext) {
	this := p
	_ = this

	localctx = NewIpv4blockContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 16, SuricataRuleParserRULE_ipv4block)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(94)
		p.Match(SuricataRuleParserINT)
	}

	return localctx
}

// IIpv4maskContext is an interface to support dynamic dispatch.
type IIpv4maskContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	INT() antlr.TerminalNode

	// IsIpv4maskContext differentiates from other interfaces.
	IsIpv4maskContext()
}

type Ipv4maskContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyIpv4maskContext() *Ipv4maskContext {
	var p = new(Ipv4maskContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_ipv4mask
	return p
}

func (*Ipv4maskContext) IsIpv4maskContext() {}

func NewIpv4maskContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Ipv4maskContext {
	var p = new(Ipv4maskContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_ipv4mask

	return p
}

func (s *Ipv4maskContext) GetParser() antlr.Parser { return s.parser }

func (s *Ipv4maskContext) INT() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserINT, 0)
}

func (s *Ipv4maskContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Ipv4maskContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Ipv4maskContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitIpv4mask(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Ipv4mask() (localctx IIpv4maskContext) {
	this := p
	_ = this

	localctx = NewIpv4maskContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 18, SuricataRuleParserRULE_ipv4mask)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(96)
		p.Match(SuricataRuleParserINT)
	}

	return localctx
}

// IEnvironment_varContext is an interface to support dynamic dispatch.
type IEnvironment_varContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Dollar() antlr.TerminalNode
	ID() antlr.TerminalNode

	// IsEnvironment_varContext differentiates from other interfaces.
	IsEnvironment_varContext()
}

type Environment_varContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyEnvironment_varContext() *Environment_varContext {
	var p = new(Environment_varContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_environment_var
	return p
}

func (*Environment_varContext) IsEnvironment_varContext() {}

func NewEnvironment_varContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Environment_varContext {
	var p = new(Environment_varContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_environment_var

	return p
}

func (s *Environment_varContext) GetParser() antlr.Parser { return s.parser }

func (s *Environment_varContext) Dollar() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserDollar, 0)
}

func (s *Environment_varContext) ID() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserID, 0)
}

func (s *Environment_varContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Environment_varContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Environment_varContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitEnvironment_var(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Environment_var() (localctx IEnvironment_varContext) {
	this := p
	_ = this

	localctx = NewEnvironment_varContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 20, SuricataRuleParserRULE_environment_var)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(98)
		p.Match(SuricataRuleParserDollar)
	}
	{
		p.SetState(99)
		p.Match(SuricataRuleParserID)
	}

	return localctx
}

// IIpv6Context is an interface to support dynamic dispatch.
type IIpv6Context interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	AllHex_part() []IHex_partContext
	Hex_part(i int) IHex_partContext
	AllColon() []antlr.TerminalNode
	Colon(i int) antlr.TerminalNode
	DoubleColon() antlr.TerminalNode

	// IsIpv6Context differentiates from other interfaces.
	IsIpv6Context()
}

type Ipv6Context struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyIpv6Context() *Ipv6Context {
	var p = new(Ipv6Context)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_ipv6
	return p
}

func (*Ipv6Context) IsIpv6Context() {}

func NewIpv6Context(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Ipv6Context {
	var p = new(Ipv6Context)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_ipv6

	return p
}

func (s *Ipv6Context) GetParser() antlr.Parser { return s.parser }

func (s *Ipv6Context) AllHex_part() []IHex_partContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IHex_partContext); ok {
			len++
		}
	}

	tst := make([]IHex_partContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IHex_partContext); ok {
			tst[i] = t.(IHex_partContext)
			i++
		}
	}

	return tst
}

func (s *Ipv6Context) Hex_part(i int) IHex_partContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IHex_partContext); ok {
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

	return t.(IHex_partContext)
}

func (s *Ipv6Context) AllColon() []antlr.TerminalNode {
	return s.GetTokens(SuricataRuleParserColon)
}

func (s *Ipv6Context) Colon(i int) antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserColon, i)
}

func (s *Ipv6Context) DoubleColon() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserDoubleColon, 0)
}

func (s *Ipv6Context) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Ipv6Context) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Ipv6Context) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitIpv6(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Ipv6() (localctx IIpv6Context) {
	this := p
	_ = this

	localctx = NewIpv6Context(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 22, SuricataRuleParserRULE_ipv6)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(101)
		p.Hex_part()
	}
	p.SetState(106)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 4, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(102)
				p.Match(SuricataRuleParserColon)
			}
			{
				p.SetState(103)
				p.Hex_part()
			}

		}
		p.SetState(108)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 4, p.GetParserRuleContext())
	}
	p.SetState(119)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserDoubleColon {
		{
			p.SetState(109)
			p.Match(SuricataRuleParserDoubleColon)
		}
		p.SetState(115)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 5, p.GetParserRuleContext())

		for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
			if _alt == 1 {
				{
					p.SetState(110)
					p.Hex_part()
				}
				{
					p.SetState(111)
					p.Match(SuricataRuleParserColon)
				}

			}
			p.SetState(117)
			p.GetErrorHandler().Sync(p)
			_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 5, p.GetParserRuleContext())
		}
		{
			p.SetState(118)
			p.Hex_part()
		}

	}

	return localctx
}

// IHex_partContext is an interface to support dynamic dispatch.
type IHex_partContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	AllH16() []IH16Context
	H16(i int) IH16Context
	AllColon() []antlr.TerminalNode
	Colon(i int) antlr.TerminalNode

	// IsHex_partContext differentiates from other interfaces.
	IsHex_partContext()
}

type Hex_partContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyHex_partContext() *Hex_partContext {
	var p = new(Hex_partContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_hex_part
	return p
}

func (*Hex_partContext) IsHex_partContext() {}

func NewHex_partContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Hex_partContext {
	var p = new(Hex_partContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_hex_part

	return p
}

func (s *Hex_partContext) GetParser() antlr.Parser { return s.parser }

func (s *Hex_partContext) AllH16() []IH16Context {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IH16Context); ok {
			len++
		}
	}

	tst := make([]IH16Context, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IH16Context); ok {
			tst[i] = t.(IH16Context)
			i++
		}
	}

	return tst
}

func (s *Hex_partContext) H16(i int) IH16Context {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IH16Context); ok {
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

	return t.(IH16Context)
}

func (s *Hex_partContext) AllColon() []antlr.TerminalNode {
	return s.GetTokens(SuricataRuleParserColon)
}

func (s *Hex_partContext) Colon(i int) antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserColon, i)
}

func (s *Hex_partContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Hex_partContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Hex_partContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitHex_part(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Hex_part() (localctx IHex_partContext) {
	this := p
	_ = this

	localctx = NewHex_partContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 24, SuricataRuleParserRULE_hex_part)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(131)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 8, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(121)
			p.H16()
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		p.SetState(123)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SuricataRuleParserHEX {
			{
				p.SetState(122)
				p.H16()
			}

		}
		{
			p.SetState(125)
			p.Match(SuricataRuleParserColon)
		}
		{
			p.SetState(126)
			p.Match(SuricataRuleParserColon)
		}
		{
			p.SetState(127)
			p.H16()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(128)
			p.Match(SuricataRuleParserColon)
		}
		{
			p.SetState(129)
			p.Match(SuricataRuleParserColon)
		}
		{
			p.SetState(130)
			p.H16()
		}

	}

	return localctx
}

// IH16Context is an interface to support dynamic dispatch.
type IH16Context interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	HEX() antlr.TerminalNode

	// IsH16Context differentiates from other interfaces.
	IsH16Context()
}

type H16Context struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyH16Context() *H16Context {
	var p = new(H16Context)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_h16
	return p
}

func (*H16Context) IsH16Context() {}

func NewH16Context(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *H16Context {
	var p = new(H16Context)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_h16

	return p
}

func (s *H16Context) GetParser() antlr.Parser { return s.parser }

func (s *H16Context) HEX() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserHEX, 0)
}

func (s *H16Context) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *H16Context) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *H16Context) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitH16(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) H16() (localctx IH16Context) {
	this := p
	_ = this

	localctx = NewH16Context(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 26, SuricataRuleParserRULE_h16)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(133)
		p.Match(SuricataRuleParserHEX)
	}

	return localctx
}

// ISrc_portContext is an interface to support dynamic dispatch.
type ISrc_portContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Port() IPortContext

	// IsSrc_portContext differentiates from other interfaces.
	IsSrc_portContext()
}

type Src_portContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySrc_portContext() *Src_portContext {
	var p = new(Src_portContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_src_port
	return p
}

func (*Src_portContext) IsSrc_portContext() {}

func NewSrc_portContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Src_portContext {
	var p = new(Src_portContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_src_port

	return p
}

func (s *Src_portContext) GetParser() antlr.Parser { return s.parser }

func (s *Src_portContext) Port() IPortContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IPortContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IPortContext)
}

func (s *Src_portContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Src_portContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Src_portContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitSrc_port(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Src_port() (localctx ISrc_portContext) {
	this := p
	_ = this

	localctx = NewSrc_portContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 28, SuricataRuleParserRULE_src_port)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(135)
		p.Port()
	}

	return localctx
}

// IDest_portContext is an interface to support dynamic dispatch.
type IDest_portContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Port() IPortContext

	// IsDest_portContext differentiates from other interfaces.
	IsDest_portContext()
}

type Dest_portContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyDest_portContext() *Dest_portContext {
	var p = new(Dest_portContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_dest_port
	return p
}

func (*Dest_portContext) IsDest_portContext() {}

func NewDest_portContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Dest_portContext {
	var p = new(Dest_portContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_dest_port

	return p
}

func (s *Dest_portContext) GetParser() antlr.Parser { return s.parser }

func (s *Dest_portContext) Port() IPortContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IPortContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IPortContext)
}

func (s *Dest_portContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Dest_portContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Dest_portContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitDest_port(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Dest_port() (localctx IDest_portContext) {
	this := p
	_ = this

	localctx = NewDest_portContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 30, SuricataRuleParserRULE_dest_port)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(137)
		p.Port()
	}

	return localctx
}

// IPortContext is an interface to support dynamic dispatch.
type IPortContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Any() antlr.TerminalNode
	AllINT() []antlr.TerminalNode
	INT(i int) antlr.TerminalNode
	Colon() antlr.TerminalNode
	Negative() antlr.TerminalNode
	AllPort() []IPortContext
	Port(i int) IPortContext
	LBracket() antlr.TerminalNode
	RBracket() antlr.TerminalNode
	AllComma() []antlr.TerminalNode
	Comma(i int) antlr.TerminalNode
	Environment_var() IEnvironment_varContext

	// IsPortContext differentiates from other interfaces.
	IsPortContext()
}

type PortContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyPortContext() *PortContext {
	var p = new(PortContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_port
	return p
}

func (*PortContext) IsPortContext() {}

func NewPortContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *PortContext {
	var p = new(PortContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_port

	return p
}

func (s *PortContext) GetParser() antlr.Parser { return s.parser }

func (s *PortContext) Any() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserAny, 0)
}

func (s *PortContext) AllINT() []antlr.TerminalNode {
	return s.GetTokens(SuricataRuleParserINT)
}

func (s *PortContext) INT(i int) antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserINT, i)
}

func (s *PortContext) Colon() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserColon, 0)
}

func (s *PortContext) Negative() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserNegative, 0)
}

func (s *PortContext) AllPort() []IPortContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IPortContext); ok {
			len++
		}
	}

	tst := make([]IPortContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IPortContext); ok {
			tst[i] = t.(IPortContext)
			i++
		}
	}

	return tst
}

func (s *PortContext) Port(i int) IPortContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IPortContext); ok {
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

	return t.(IPortContext)
}

func (s *PortContext) LBracket() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserLBracket, 0)
}

func (s *PortContext) RBracket() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserRBracket, 0)
}

func (s *PortContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(SuricataRuleParserComma)
}

func (s *PortContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserComma, i)
}

func (s *PortContext) Environment_var() IEnvironment_varContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IEnvironment_varContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IEnvironment_varContext)
}

func (s *PortContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *PortContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *PortContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitPort(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Port() (localctx IPortContext) {
	this := p
	_ = this

	localctx = NewPortContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 32, SuricataRuleParserRULE_port)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.SetState(162)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 11, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(139)
			p.Match(SuricataRuleParserAny)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(140)
			p.Match(SuricataRuleParserINT)
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(141)
			p.Match(SuricataRuleParserINT)
		}
		{
			p.SetState(142)
			p.Match(SuricataRuleParserColon)
		}
		p.SetState(144)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SuricataRuleParserINT {
			{
				p.SetState(143)
				p.Match(SuricataRuleParserINT)
			}

		}

	case 4:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(146)
			p.Match(SuricataRuleParserColon)
		}
		{
			p.SetState(147)
			p.Match(SuricataRuleParserINT)
		}

	case 5:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(148)
			p.Match(SuricataRuleParserNegative)
		}
		{
			p.SetState(149)
			p.Port()
		}

	case 6:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(150)
			p.Match(SuricataRuleParserLBracket)
		}
		{
			p.SetState(151)
			p.Port()
		}
		p.SetState(156)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == SuricataRuleParserComma {
			{
				p.SetState(152)
				p.Match(SuricataRuleParserComma)
			}
			{
				p.SetState(153)
				p.Port()
			}

			p.SetState(158)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(159)
			p.Match(SuricataRuleParserRBracket)
		}

	case 7:
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(161)
			p.Environment_var()
		}

	}

	return localctx
}

// IParamsContext is an interface to support dynamic dispatch.
type IParamsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ParamStart() antlr.TerminalNode
	AllParam() []IParamContext
	Param(i int) IParamContext
	ParamEnd() antlr.TerminalNode
	AllParamSep() []antlr.TerminalNode
	ParamSep(i int) antlr.TerminalNode

	// IsParamsContext differentiates from other interfaces.
	IsParamsContext()
}

type ParamsContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyParamsContext() *ParamsContext {
	var p = new(ParamsContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_params
	return p
}

func (*ParamsContext) IsParamsContext() {}

func NewParamsContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ParamsContext {
	var p = new(ParamsContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_params

	return p
}

func (s *ParamsContext) GetParser() antlr.Parser { return s.parser }

func (s *ParamsContext) ParamStart() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamStart, 0)
}

func (s *ParamsContext) AllParam() []IParamContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IParamContext); ok {
			len++
		}
	}

	tst := make([]IParamContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IParamContext); ok {
			tst[i] = t.(IParamContext)
			i++
		}
	}

	return tst
}

func (s *ParamsContext) Param(i int) IParamContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IParamContext); ok {
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

	return t.(IParamContext)
}

func (s *ParamsContext) ParamEnd() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamEnd, 0)
}

func (s *ParamsContext) AllParamSep() []antlr.TerminalNode {
	return s.GetTokens(SuricataRuleParserParamSep)
}

func (s *ParamsContext) ParamSep(i int) antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamSep, i)
}

func (s *ParamsContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ParamsContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ParamsContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitParams(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Params() (localctx IParamsContext) {
	this := p
	_ = this

	localctx = NewParamsContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 34, SuricataRuleParserRULE_params)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	var _alt int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(164)
		p.Match(SuricataRuleParserParamStart)
	}
	{
		p.SetState(165)
		p.Param()
	}
	p.SetState(170)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 12, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(166)
				p.Match(SuricataRuleParserParamSep)
			}
			{
				p.SetState(167)
				p.Param()
			}

		}
		p.SetState(172)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 12, p.GetParserRuleContext())
	}
	p.SetState(174)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserParamSep {
		{
			p.SetState(173)
			p.Match(SuricataRuleParserParamSep)
		}

	}
	{
		p.SetState(176)
		p.Match(SuricataRuleParserParamEnd)
	}

	return localctx
}

// IParamContext is an interface to support dynamic dispatch.
type IParamContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ParamValue() antlr.TerminalNode
	String_() IStringContext

	// IsParamContext differentiates from other interfaces.
	IsParamContext()
}

type ParamContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyParamContext() *ParamContext {
	var p = new(ParamContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_param
	return p
}

func (*ParamContext) IsParamContext() {}

func NewParamContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ParamContext {
	var p = new(ParamContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_param

	return p
}

func (s *ParamContext) GetParser() antlr.Parser { return s.parser }

func (s *ParamContext) ParamValue() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamValue, 0)
}

func (s *ParamContext) String_() IStringContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStringContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStringContext)
}

func (s *ParamContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ParamContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ParamContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitParam(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Param() (localctx IParamContext) {
	this := p
	_ = this

	localctx = NewParamContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 36, SuricataRuleParserRULE_param)
	var _la int

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(178)
		p.Match(SuricataRuleParserParamValue)
	}
	p.SetState(180)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserParamQuotedString {
		{
			p.SetState(179)
			p.String_()
		}

	}

	return localctx
}

// IStringContext is an interface to support dynamic dispatch.
type IStringContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ParamQuotedString() antlr.TerminalNode

	// IsStringContext differentiates from other interfaces.
	IsStringContext()
}

type StringContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStringContext() *StringContext {
	var p = new(StringContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_string
	return p
}

func (*StringContext) IsStringContext() {}

func NewStringContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StringContext {
	var p = new(StringContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_string

	return p
}

func (s *StringContext) GetParser() antlr.Parser { return s.parser }

func (s *StringContext) ParamQuotedString() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamQuotedString, 0)
}

func (s *StringContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StringContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StringContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitString(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) String_() (localctx IStringContext) {
	this := p
	_ = this

	localctx = NewStringContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 38, SuricataRuleParserRULE_string)

	defer func() {
		p.ExitRule()
	}()

	defer func() {
		if err := recover(); err != nil {
			if v, ok := err.(antlr.RecognitionException); ok {
				localctx.SetException(v)
				p.GetErrorHandler().ReportError(p, v)
				p.GetErrorHandler().Recover(p, v)
			} else {
				panic(err)
			}
		}
	}()

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(182)
		p.Match(SuricataRuleParserParamQuotedString)
	}

	return localctx
}
