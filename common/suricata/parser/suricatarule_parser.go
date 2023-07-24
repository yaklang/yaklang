// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

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
		"", "'any'", "", "'$'", "'->'", "'<>'", "'*'", "'/'", "'%'", "'&'",
		"'+'", "'-'", "'^'", "'<'", "'>'", "'<='", "'>='", "", "'::'", "'['",
		"']'", "'('", "'{'", "'}'", "", "'='", "'~'", "'.'", "", "", "", "",
		"", "", "", "", "", "", "')'", "", "", "';'",
	}
	staticData.symbolicNames = []string{
		"", "Any", "Negative", "Dollar", "Arrow", "BothDirect", "Mul", "Div",
		"Mod", "Amp", "Plus", "Sub", "Power", "Lt", "Gt", "LtEq", "GtEq", "Colon",
		"DoubleColon", "LBracket", "RBracket", "ParamStart", "LBrace", "RBrace",
		"Comma", "Eq", "NotSymbol", "Dot", "LINE_COMMENT", "NORMALSTRING", "INT",
		"HEX", "ID", "HexDigit", "WS", "NonSemiColon", "SHEBANG", "ParamWS",
		"ParamEnd", "ParamQuotedString", "ParamColon", "ParamSep", "ParamNegative",
		"ParamComma", "ParamCommonString",
	}
	staticData.ruleNames = []string{
		"rules", "rule", "action", "protocol", "src_address", "dest_address",
		"address", "ipv4", "ipv4block", "ipv4mask", "environment_var", "ipv6",
		"ipv6full", "ipv6compact", "ipv6part", "ipv6block", "ipv6mask", "src_port",
		"dest_port", "port", "params", "param", "keyword", "setting", "singleSetting",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 44, 221, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7, 20, 2,
		21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 1, 0, 4, 0, 52, 8,
		0, 11, 0, 12, 0, 53, 1, 0, 1, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 2, 1, 2, 1, 3, 1, 3, 1, 4, 1, 4, 1, 5, 1, 5, 1, 6, 1,
		6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 5, 6, 83, 8, 6, 10, 6, 12, 6, 86,
		9, 6, 1, 6, 1, 6, 1, 6, 1, 6, 3, 6, 92, 8, 6, 1, 7, 1, 7, 1, 7, 1, 7, 1,
		7, 1, 7, 1, 7, 1, 7, 1, 7, 3, 7, 103, 8, 7, 1, 8, 1, 8, 1, 9, 1, 9, 1,
		10, 1, 10, 1, 10, 1, 11, 1, 11, 3, 11, 114, 8, 11, 1, 11, 1, 11, 3, 11,
		118, 8, 11, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1,
		12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 13, 1, 13, 1, 13,
		1, 13, 1, 14, 1, 14, 3, 14, 142, 8, 14, 1, 14, 1, 14, 1, 14, 5, 14, 147,
		8, 14, 10, 14, 12, 14, 150, 9, 14, 1, 15, 1, 15, 1, 16, 1, 16, 1, 17, 1,
		17, 1, 18, 1, 18, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 3, 19, 166,
		8, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 5, 19, 176,
		8, 19, 10, 19, 12, 19, 179, 9, 19, 1, 19, 1, 19, 1, 19, 1, 19, 3, 19, 185,
		8, 19, 1, 20, 1, 20, 1, 20, 1, 20, 5, 20, 191, 8, 20, 10, 20, 12, 20, 194,
		9, 20, 1, 20, 3, 20, 197, 8, 20, 1, 20, 1, 20, 1, 21, 1, 21, 1, 21, 3,
		21, 204, 8, 21, 1, 22, 1, 22, 1, 23, 1, 23, 1, 23, 5, 23, 211, 8, 23, 10,
		23, 12, 23, 214, 9, 23, 1, 24, 3, 24, 217, 8, 24, 1, 24, 1, 24, 1, 24,
		0, 1, 28, 25, 0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30,
		32, 34, 36, 38, 40, 42, 44, 46, 48, 0, 3, 1, 0, 4, 5, 1, 0, 30, 31, 2,
		0, 39, 39, 44, 44, 221, 0, 51, 1, 0, 0, 0, 2, 57, 1, 0, 0, 0, 4, 66, 1,
		0, 0, 0, 6, 68, 1, 0, 0, 0, 8, 70, 1, 0, 0, 0, 10, 72, 1, 0, 0, 0, 12,
		91, 1, 0, 0, 0, 14, 93, 1, 0, 0, 0, 16, 104, 1, 0, 0, 0, 18, 106, 1, 0,
		0, 0, 20, 108, 1, 0, 0, 0, 22, 113, 1, 0, 0, 0, 24, 119, 1, 0, 0, 0, 26,
		135, 1, 0, 0, 0, 28, 139, 1, 0, 0, 0, 30, 151, 1, 0, 0, 0, 32, 153, 1,
		0, 0, 0, 34, 155, 1, 0, 0, 0, 36, 157, 1, 0, 0, 0, 38, 184, 1, 0, 0, 0,
		40, 186, 1, 0, 0, 0, 42, 200, 1, 0, 0, 0, 44, 205, 1, 0, 0, 0, 46, 207,
		1, 0, 0, 0, 48, 216, 1, 0, 0, 0, 50, 52, 3, 2, 1, 0, 51, 50, 1, 0, 0, 0,
		52, 53, 1, 0, 0, 0, 53, 51, 1, 0, 0, 0, 53, 54, 1, 0, 0, 0, 54, 55, 1,
		0, 0, 0, 55, 56, 5, 0, 0, 1, 56, 1, 1, 0, 0, 0, 57, 58, 3, 4, 2, 0, 58,
		59, 3, 6, 3, 0, 59, 60, 3, 8, 4, 0, 60, 61, 3, 34, 17, 0, 61, 62, 7, 0,
		0, 0, 62, 63, 3, 10, 5, 0, 63, 64, 3, 36, 18, 0, 64, 65, 3, 40, 20, 0,
		65, 3, 1, 0, 0, 0, 66, 67, 5, 32, 0, 0, 67, 5, 1, 0, 0, 0, 68, 69, 5, 32,
		0, 0, 69, 7, 1, 0, 0, 0, 70, 71, 3, 12, 6, 0, 71, 9, 1, 0, 0, 0, 72, 73,
		3, 12, 6, 0, 73, 11, 1, 0, 0, 0, 74, 92, 5, 1, 0, 0, 75, 92, 3, 20, 10,
		0, 76, 92, 3, 14, 7, 0, 77, 92, 3, 22, 11, 0, 78, 79, 5, 19, 0, 0, 79,
		84, 3, 12, 6, 0, 80, 81, 5, 24, 0, 0, 81, 83, 3, 12, 6, 0, 82, 80, 1, 0,
		0, 0, 83, 86, 1, 0, 0, 0, 84, 82, 1, 0, 0, 0, 84, 85, 1, 0, 0, 0, 85, 87,
		1, 0, 0, 0, 86, 84, 1, 0, 0, 0, 87, 88, 5, 20, 0, 0, 88, 92, 1, 0, 0, 0,
		89, 90, 5, 2, 0, 0, 90, 92, 3, 12, 6, 0, 91, 74, 1, 0, 0, 0, 91, 75, 1,
		0, 0, 0, 91, 76, 1, 0, 0, 0, 91, 77, 1, 0, 0, 0, 91, 78, 1, 0, 0, 0, 91,
		89, 1, 0, 0, 0, 92, 13, 1, 0, 0, 0, 93, 94, 3, 16, 8, 0, 94, 95, 5, 27,
		0, 0, 95, 96, 3, 16, 8, 0, 96, 97, 5, 27, 0, 0, 97, 98, 3, 16, 8, 0, 98,
		99, 5, 27, 0, 0, 99, 102, 3, 16, 8, 0, 100, 101, 5, 7, 0, 0, 101, 103,
		3, 18, 9, 0, 102, 100, 1, 0, 0, 0, 102, 103, 1, 0, 0, 0, 103, 15, 1, 0,
		0, 0, 104, 105, 5, 30, 0, 0, 105, 17, 1, 0, 0, 0, 106, 107, 5, 30, 0, 0,
		107, 19, 1, 0, 0, 0, 108, 109, 5, 3, 0, 0, 109, 110, 5, 32, 0, 0, 110,
		21, 1, 0, 0, 0, 111, 114, 3, 24, 12, 0, 112, 114, 3, 26, 13, 0, 113, 111,
		1, 0, 0, 0, 113, 112, 1, 0, 0, 0, 114, 117, 1, 0, 0, 0, 115, 116, 5, 7,
		0, 0, 116, 118, 3, 32, 16, 0, 117, 115, 1, 0, 0, 0, 117, 118, 1, 0, 0,
		0, 118, 23, 1, 0, 0, 0, 119, 120, 3, 30, 15, 0, 120, 121, 5, 17, 0, 0,
		121, 122, 3, 30, 15, 0, 122, 123, 5, 17, 0, 0, 123, 124, 3, 30, 15, 0,
		124, 125, 5, 17, 0, 0, 125, 126, 3, 30, 15, 0, 126, 127, 5, 17, 0, 0, 127,
		128, 3, 30, 15, 0, 128, 129, 5, 17, 0, 0, 129, 130, 3, 30, 15, 0, 130,
		131, 5, 17, 0, 0, 131, 132, 3, 30, 15, 0, 132, 133, 5, 17, 0, 0, 133, 134,
		3, 30, 15, 0, 134, 25, 1, 0, 0, 0, 135, 136, 3, 28, 14, 0, 136, 137, 5,
		18, 0, 0, 137, 138, 3, 28, 14, 0, 138, 27, 1, 0, 0, 0, 139, 141, 6, 14,
		-1, 0, 140, 142, 3, 30, 15, 0, 141, 140, 1, 0, 0, 0, 141, 142, 1, 0, 0,
		0, 142, 148, 1, 0, 0, 0, 143, 144, 10, 1, 0, 0, 144, 145, 5, 17, 0, 0,
		145, 147, 3, 30, 15, 0, 146, 143, 1, 0, 0, 0, 147, 150, 1, 0, 0, 0, 148,
		146, 1, 0, 0, 0, 148, 149, 1, 0, 0, 0, 149, 29, 1, 0, 0, 0, 150, 148, 1,
		0, 0, 0, 151, 152, 7, 1, 0, 0, 152, 31, 1, 0, 0, 0, 153, 154, 5, 30, 0,
		0, 154, 33, 1, 0, 0, 0, 155, 156, 3, 38, 19, 0, 156, 35, 1, 0, 0, 0, 157,
		158, 3, 38, 19, 0, 158, 37, 1, 0, 0, 0, 159, 185, 5, 1, 0, 0, 160, 185,
		3, 20, 10, 0, 161, 185, 5, 30, 0, 0, 162, 163, 5, 30, 0, 0, 163, 165, 5,
		17, 0, 0, 164, 166, 5, 30, 0, 0, 165, 164, 1, 0, 0, 0, 165, 166, 1, 0,
		0, 0, 166, 185, 1, 0, 0, 0, 167, 168, 5, 17, 0, 0, 168, 185, 5, 30, 0,
		0, 169, 170, 5, 30, 0, 0, 170, 185, 5, 17, 0, 0, 171, 172, 5, 19, 0, 0,
		172, 177, 3, 38, 19, 0, 173, 174, 5, 24, 0, 0, 174, 176, 3, 38, 19, 0,
		175, 173, 1, 0, 0, 0, 176, 179, 1, 0, 0, 0, 177, 175, 1, 0, 0, 0, 177,
		178, 1, 0, 0, 0, 178, 180, 1, 0, 0, 0, 179, 177, 1, 0, 0, 0, 180, 181,
		5, 20, 0, 0, 181, 185, 1, 0, 0, 0, 182, 183, 5, 2, 0, 0, 183, 185, 3, 38,
		19, 0, 184, 159, 1, 0, 0, 0, 184, 160, 1, 0, 0, 0, 184, 161, 1, 0, 0, 0,
		184, 162, 1, 0, 0, 0, 184, 167, 1, 0, 0, 0, 184, 169, 1, 0, 0, 0, 184,
		171, 1, 0, 0, 0, 184, 182, 1, 0, 0, 0, 185, 39, 1, 0, 0, 0, 186, 187, 5,
		21, 0, 0, 187, 192, 3, 42, 21, 0, 188, 189, 5, 41, 0, 0, 189, 191, 3, 42,
		21, 0, 190, 188, 1, 0, 0, 0, 191, 194, 1, 0, 0, 0, 192, 190, 1, 0, 0, 0,
		192, 193, 1, 0, 0, 0, 193, 196, 1, 0, 0, 0, 194, 192, 1, 0, 0, 0, 195,
		197, 5, 41, 0, 0, 196, 195, 1, 0, 0, 0, 196, 197, 1, 0, 0, 0, 197, 198,
		1, 0, 0, 0, 198, 199, 5, 38, 0, 0, 199, 41, 1, 0, 0, 0, 200, 203, 3, 44,
		22, 0, 201, 202, 5, 40, 0, 0, 202, 204, 3, 46, 23, 0, 203, 201, 1, 0, 0,
		0, 203, 204, 1, 0, 0, 0, 204, 43, 1, 0, 0, 0, 205, 206, 5, 44, 0, 0, 206,
		45, 1, 0, 0, 0, 207, 212, 3, 48, 24, 0, 208, 209, 5, 43, 0, 0, 209, 211,
		3, 48, 24, 0, 210, 208, 1, 0, 0, 0, 211, 214, 1, 0, 0, 0, 212, 210, 1,
		0, 0, 0, 212, 213, 1, 0, 0, 0, 213, 47, 1, 0, 0, 0, 214, 212, 1, 0, 0,
		0, 215, 217, 5, 42, 0, 0, 216, 215, 1, 0, 0, 0, 216, 217, 1, 0, 0, 0, 217,
		218, 1, 0, 0, 0, 218, 219, 7, 2, 0, 0, 219, 49, 1, 0, 0, 0, 16, 53, 84,
		91, 102, 113, 117, 141, 148, 165, 177, 184, 192, 196, 203, 212, 216,
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
	this.GrammarFileName = "java-escape"

	return this
}

// SuricataRuleParser tokens.
const (
	SuricataRuleParserEOF               = antlr.TokenEOF
	SuricataRuleParserAny               = 1
	SuricataRuleParserNegative          = 2
	SuricataRuleParserDollar            = 3
	SuricataRuleParserArrow             = 4
	SuricataRuleParserBothDirect        = 5
	SuricataRuleParserMul               = 6
	SuricataRuleParserDiv               = 7
	SuricataRuleParserMod               = 8
	SuricataRuleParserAmp               = 9
	SuricataRuleParserPlus              = 10
	SuricataRuleParserSub               = 11
	SuricataRuleParserPower             = 12
	SuricataRuleParserLt                = 13
	SuricataRuleParserGt                = 14
	SuricataRuleParserLtEq              = 15
	SuricataRuleParserGtEq              = 16
	SuricataRuleParserColon             = 17
	SuricataRuleParserDoubleColon       = 18
	SuricataRuleParserLBracket          = 19
	SuricataRuleParserRBracket          = 20
	SuricataRuleParserParamStart        = 21
	SuricataRuleParserLBrace            = 22
	SuricataRuleParserRBrace            = 23
	SuricataRuleParserComma             = 24
	SuricataRuleParserEq                = 25
	SuricataRuleParserNotSymbol         = 26
	SuricataRuleParserDot               = 27
	SuricataRuleParserLINE_COMMENT      = 28
	SuricataRuleParserNORMALSTRING      = 29
	SuricataRuleParserINT               = 30
	SuricataRuleParserHEX               = 31
	SuricataRuleParserID                = 32
	SuricataRuleParserHexDigit          = 33
	SuricataRuleParserWS                = 34
	SuricataRuleParserNonSemiColon      = 35
	SuricataRuleParserSHEBANG           = 36
	SuricataRuleParserParamWS           = 37
	SuricataRuleParserParamEnd          = 38
	SuricataRuleParserParamQuotedString = 39
	SuricataRuleParserParamColon        = 40
	SuricataRuleParserParamSep          = 41
	SuricataRuleParserParamNegative     = 42
	SuricataRuleParserParamComma        = 43
	SuricataRuleParserParamCommonString = 44
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
	SuricataRuleParserRULE_ipv6full        = 12
	SuricataRuleParserRULE_ipv6compact     = 13
	SuricataRuleParserRULE_ipv6part        = 14
	SuricataRuleParserRULE_ipv6block       = 15
	SuricataRuleParserRULE_ipv6mask        = 16
	SuricataRuleParserRULE_src_port        = 17
	SuricataRuleParserRULE_dest_port       = 18
	SuricataRuleParserRULE_port            = 19
	SuricataRuleParserRULE_params          = 20
	SuricataRuleParserRULE_param           = 21
	SuricataRuleParserRULE_keyword         = 22
	SuricataRuleParserRULE_setting         = 23
	SuricataRuleParserRULE_singleSetting   = 24
)

// IRulesContext is an interface to support dynamic dispatch.
type IRulesContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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
	p.SetState(51)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == SuricataRuleParserID {
		{
			p.SetState(50)
			p.Rule_()
		}

		p.SetState(53)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(55)
		p.Match(SuricataRuleParserEOF)
	}

	return localctx
}

// IRuleContext is an interface to support dynamic dispatch.
type IRuleContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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

func (s *RuleContext) Arrow() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserArrow, 0)
}

func (s *RuleContext) BothDirect() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserBothDirect, 0)
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
		p.SetState(57)
		p.Action_()
	}
	{
		p.SetState(58)
		p.Protocol()
	}
	{
		p.SetState(59)
		p.Src_address()
	}
	{
		p.SetState(60)
		p.Src_port()
	}
	{
		p.SetState(61)
		_la = p.GetTokenStream().LA(1)

		if !(_la == SuricataRuleParserArrow || _la == SuricataRuleParserBothDirect) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}
	{
		p.SetState(62)
		p.Dest_address()
	}
	{
		p.SetState(63)
		p.Dest_port()
	}
	{
		p.SetState(64)
		p.Params()
	}

	return localctx
}

// IActionContext is an interface to support dynamic dispatch.
type IActionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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
		p.SetState(66)
		p.Match(SuricataRuleParserID)
	}

	return localctx
}

// IProtocolContext is an interface to support dynamic dispatch.
type IProtocolContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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
		p.SetState(68)
		p.Match(SuricataRuleParserID)
	}

	return localctx
}

// ISrc_addressContext is an interface to support dynamic dispatch.
type ISrc_addressContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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
		p.SetState(70)
		p.Address()
	}

	return localctx
}

// IDest_addressContext is an interface to support dynamic dispatch.
type IDest_addressContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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
		p.SetState(72)
		p.Address()
	}

	return localctx
}

// IAddressContext is an interface to support dynamic dispatch.
type IAddressContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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

func (s *AddressContext) RBracket() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserRBracket, 0)
}

func (s *AddressContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(SuricataRuleParserComma)
}

func (s *AddressContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserComma, i)
}

func (s *AddressContext) Negative() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserNegative, 0)
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

	p.SetState(91)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 2, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(74)
			p.Match(SuricataRuleParserAny)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(75)
			p.Environment_var()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(76)
			p.Ipv4()
		}

	case 4:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(77)
			p.Ipv6()
		}

	case 5:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(78)
			p.Match(SuricataRuleParserLBracket)
		}
		{
			p.SetState(79)
			p.Address()
		}
		p.SetState(84)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == SuricataRuleParserComma {
			{
				p.SetState(80)
				p.Match(SuricataRuleParserComma)
			}
			{
				p.SetState(81)
				p.Address()
			}

			p.SetState(86)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(87)
			p.Match(SuricataRuleParserRBracket)
		}

	case 6:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(89)
			p.Match(SuricataRuleParserNegative)
		}
		{
			p.SetState(90)
			p.Address()
		}

	}

	return localctx
}

// IIpv4Context is an interface to support dynamic dispatch.
type IIpv4Context interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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
		p.SetState(93)
		p.Ipv4block()
	}
	{
		p.SetState(94)
		p.Match(SuricataRuleParserDot)
	}
	{
		p.SetState(95)
		p.Ipv4block()
	}
	{
		p.SetState(96)
		p.Match(SuricataRuleParserDot)
	}
	{
		p.SetState(97)
		p.Ipv4block()
	}
	{
		p.SetState(98)
		p.Match(SuricataRuleParserDot)
	}
	{
		p.SetState(99)
		p.Ipv4block()
	}
	p.SetState(102)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserDiv {
		{
			p.SetState(100)
			p.Match(SuricataRuleParserDiv)
		}
		{
			p.SetState(101)
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
		p.SetState(104)
		p.Match(SuricataRuleParserINT)
	}

	return localctx
}

// IIpv4maskContext is an interface to support dynamic dispatch.
type IIpv4maskContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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
		p.SetState(106)
		p.Match(SuricataRuleParserINT)
	}

	return localctx
}

// IEnvironment_varContext is an interface to support dynamic dispatch.
type IEnvironment_varContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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
		p.SetState(108)
		p.Match(SuricataRuleParserDollar)
	}
	{
		p.SetState(109)
		p.Match(SuricataRuleParserID)
	}

	return localctx
}

// IIpv6Context is an interface to support dynamic dispatch.
type IIpv6Context interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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

func (s *Ipv6Context) Ipv6full() IIpv6fullContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIpv6fullContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIpv6fullContext)
}

func (s *Ipv6Context) Ipv6compact() IIpv6compactContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIpv6compactContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIpv6compactContext)
}

func (s *Ipv6Context) Div() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserDiv, 0)
}

func (s *Ipv6Context) Ipv6mask() IIpv6maskContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIpv6maskContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIpv6maskContext)
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

	p.EnterOuterAlt(localctx, 1)
	p.SetState(113)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 4, p.GetParserRuleContext()) {
	case 1:
		{
			p.SetState(111)
			p.Ipv6full()
		}

	case 2:
		{
			p.SetState(112)
			p.Ipv6compact()
		}

	}
	p.SetState(117)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserDiv {
		{
			p.SetState(115)
			p.Match(SuricataRuleParserDiv)
		}
		{
			p.SetState(116)
			p.Ipv6mask()
		}

	}

	return localctx
}

// IIpv6fullContext is an interface to support dynamic dispatch.
type IIpv6fullContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsIpv6fullContext differentiates from other interfaces.
	IsIpv6fullContext()
}

type Ipv6fullContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyIpv6fullContext() *Ipv6fullContext {
	var p = new(Ipv6fullContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_ipv6full
	return p
}

func (*Ipv6fullContext) IsIpv6fullContext() {}

func NewIpv6fullContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Ipv6fullContext {
	var p = new(Ipv6fullContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_ipv6full

	return p
}

func (s *Ipv6fullContext) GetParser() antlr.Parser { return s.parser }

func (s *Ipv6fullContext) AllIpv6block() []IIpv6blockContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IIpv6blockContext); ok {
			len++
		}
	}

	tst := make([]IIpv6blockContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IIpv6blockContext); ok {
			tst[i] = t.(IIpv6blockContext)
			i++
		}
	}

	return tst
}

func (s *Ipv6fullContext) Ipv6block(i int) IIpv6blockContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIpv6blockContext); ok {
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

	return t.(IIpv6blockContext)
}

func (s *Ipv6fullContext) AllColon() []antlr.TerminalNode {
	return s.GetTokens(SuricataRuleParserColon)
}

func (s *Ipv6fullContext) Colon(i int) antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserColon, i)
}

func (s *Ipv6fullContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Ipv6fullContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Ipv6fullContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitIpv6full(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Ipv6full() (localctx IIpv6fullContext) {
	this := p
	_ = this

	localctx = NewIpv6fullContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 24, SuricataRuleParserRULE_ipv6full)

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
		p.SetState(119)
		p.Ipv6block()
	}
	{
		p.SetState(120)
		p.Match(SuricataRuleParserColon)
	}
	{
		p.SetState(121)
		p.Ipv6block()
	}
	{
		p.SetState(122)
		p.Match(SuricataRuleParserColon)
	}
	{
		p.SetState(123)
		p.Ipv6block()
	}
	{
		p.SetState(124)
		p.Match(SuricataRuleParserColon)
	}
	{
		p.SetState(125)
		p.Ipv6block()
	}
	{
		p.SetState(126)
		p.Match(SuricataRuleParserColon)
	}
	{
		p.SetState(127)
		p.Ipv6block()
	}
	{
		p.SetState(128)
		p.Match(SuricataRuleParserColon)
	}
	{
		p.SetState(129)
		p.Ipv6block()
	}
	{
		p.SetState(130)
		p.Match(SuricataRuleParserColon)
	}
	{
		p.SetState(131)
		p.Ipv6block()
	}
	{
		p.SetState(132)
		p.Match(SuricataRuleParserColon)
	}
	{
		p.SetState(133)
		p.Ipv6block()
	}

	return localctx
}

// IIpv6compactContext is an interface to support dynamic dispatch.
type IIpv6compactContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsIpv6compactContext differentiates from other interfaces.
	IsIpv6compactContext()
}

type Ipv6compactContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyIpv6compactContext() *Ipv6compactContext {
	var p = new(Ipv6compactContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_ipv6compact
	return p
}

func (*Ipv6compactContext) IsIpv6compactContext() {}

func NewIpv6compactContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Ipv6compactContext {
	var p = new(Ipv6compactContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_ipv6compact

	return p
}

func (s *Ipv6compactContext) GetParser() antlr.Parser { return s.parser }

func (s *Ipv6compactContext) AllIpv6part() []IIpv6partContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IIpv6partContext); ok {
			len++
		}
	}

	tst := make([]IIpv6partContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IIpv6partContext); ok {
			tst[i] = t.(IIpv6partContext)
			i++
		}
	}

	return tst
}

func (s *Ipv6compactContext) Ipv6part(i int) IIpv6partContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIpv6partContext); ok {
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

	return t.(IIpv6partContext)
}

func (s *Ipv6compactContext) DoubleColon() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserDoubleColon, 0)
}

func (s *Ipv6compactContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Ipv6compactContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Ipv6compactContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitIpv6compact(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Ipv6compact() (localctx IIpv6compactContext) {
	this := p
	_ = this

	localctx = NewIpv6compactContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 26, SuricataRuleParserRULE_ipv6compact)

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
		p.ipv6part(0)
	}
	{
		p.SetState(136)
		p.Match(SuricataRuleParserDoubleColon)
	}
	{
		p.SetState(137)
		p.ipv6part(0)
	}

	return localctx
}

// IIpv6partContext is an interface to support dynamic dispatch.
type IIpv6partContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsIpv6partContext differentiates from other interfaces.
	IsIpv6partContext()
}

type Ipv6partContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyIpv6partContext() *Ipv6partContext {
	var p = new(Ipv6partContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_ipv6part
	return p
}

func (*Ipv6partContext) IsIpv6partContext() {}

func NewIpv6partContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Ipv6partContext {
	var p = new(Ipv6partContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_ipv6part

	return p
}

func (s *Ipv6partContext) GetParser() antlr.Parser { return s.parser }

func (s *Ipv6partContext) Ipv6block() IIpv6blockContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIpv6blockContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIpv6blockContext)
}

func (s *Ipv6partContext) Ipv6part() IIpv6partContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIpv6partContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIpv6partContext)
}

func (s *Ipv6partContext) Colon() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserColon, 0)
}

func (s *Ipv6partContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Ipv6partContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Ipv6partContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitIpv6part(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Ipv6part() (localctx IIpv6partContext) {
	return p.ipv6part(0)
}

func (p *SuricataRuleParser) ipv6part(_p int) (localctx IIpv6partContext) {
	this := p
	_ = this

	var _parentctx antlr.ParserRuleContext = p.GetParserRuleContext()
	_parentState := p.GetState()
	localctx = NewIpv6partContext(p, p.GetParserRuleContext(), _parentState)
	var _prevctx IIpv6partContext = localctx
	var _ antlr.ParserRuleContext = _prevctx // TODO: To prevent unused variable warning.
	_startState := 28
	p.EnterRecursionRule(localctx, 28, SuricataRuleParserRULE_ipv6part, _p)

	defer func() {
		p.UnrollRecursionContexts(_parentctx)
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
	p.SetState(141)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 6, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(140)
			p.Ipv6block()
		}

	}

	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(148)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 7, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			if p.GetParseListeners() != nil {
				p.TriggerExitRuleEvent()
			}
			_prevctx = localctx
			localctx = NewIpv6partContext(p, _parentctx, _parentState)
			p.PushNewRecursionContext(localctx, _startState, SuricataRuleParserRULE_ipv6part)
			p.SetState(143)

			if !(p.Precpred(p.GetParserRuleContext(), 1)) {
				panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 1)", ""))
			}
			{
				p.SetState(144)
				p.Match(SuricataRuleParserColon)
			}
			{
				p.SetState(145)
				p.Ipv6block()
			}

		}
		p.SetState(150)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 7, p.GetParserRuleContext())
	}

	return localctx
}

// IIpv6blockContext is an interface to support dynamic dispatch.
type IIpv6blockContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsIpv6blockContext differentiates from other interfaces.
	IsIpv6blockContext()
}

type Ipv6blockContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyIpv6blockContext() *Ipv6blockContext {
	var p = new(Ipv6blockContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_ipv6block
	return p
}

func (*Ipv6blockContext) IsIpv6blockContext() {}

func NewIpv6blockContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Ipv6blockContext {
	var p = new(Ipv6blockContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_ipv6block

	return p
}

func (s *Ipv6blockContext) GetParser() antlr.Parser { return s.parser }

func (s *Ipv6blockContext) HEX() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserHEX, 0)
}

func (s *Ipv6blockContext) INT() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserINT, 0)
}

func (s *Ipv6blockContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Ipv6blockContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Ipv6blockContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitIpv6block(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Ipv6block() (localctx IIpv6blockContext) {
	this := p
	_ = this

	localctx = NewIpv6blockContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 30, SuricataRuleParserRULE_ipv6block)
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
		p.SetState(151)
		_la = p.GetTokenStream().LA(1)

		if !(_la == SuricataRuleParserINT || _la == SuricataRuleParserHEX) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

// IIpv6maskContext is an interface to support dynamic dispatch.
type IIpv6maskContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsIpv6maskContext differentiates from other interfaces.
	IsIpv6maskContext()
}

type Ipv6maskContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyIpv6maskContext() *Ipv6maskContext {
	var p = new(Ipv6maskContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_ipv6mask
	return p
}

func (*Ipv6maskContext) IsIpv6maskContext() {}

func NewIpv6maskContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *Ipv6maskContext {
	var p = new(Ipv6maskContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_ipv6mask

	return p
}

func (s *Ipv6maskContext) GetParser() antlr.Parser { return s.parser }

func (s *Ipv6maskContext) INT() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserINT, 0)
}

func (s *Ipv6maskContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *Ipv6maskContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *Ipv6maskContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitIpv6mask(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Ipv6mask() (localctx IIpv6maskContext) {
	this := p
	_ = this

	localctx = NewIpv6maskContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 32, SuricataRuleParserRULE_ipv6mask)

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
		p.SetState(153)
		p.Match(SuricataRuleParserINT)
	}

	return localctx
}

// ISrc_portContext is an interface to support dynamic dispatch.
type ISrc_portContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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
	p.EnterRule(localctx, 34, SuricataRuleParserRULE_src_port)

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
		p.SetState(155)
		p.Port()
	}

	return localctx
}

// IDest_portContext is an interface to support dynamic dispatch.
type IDest_portContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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
	p.EnterRule(localctx, 36, SuricataRuleParserRULE_dest_port)

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
		p.SetState(157)
		p.Port()
	}

	return localctx
}

// IPortContext is an interface to support dynamic dispatch.
type IPortContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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

func (s *PortContext) AllINT() []antlr.TerminalNode {
	return s.GetTokens(SuricataRuleParserINT)
}

func (s *PortContext) INT(i int) antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserINT, i)
}

func (s *PortContext) Colon() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserColon, 0)
}

func (s *PortContext) LBracket() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserLBracket, 0)
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

func (s *PortContext) RBracket() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserRBracket, 0)
}

func (s *PortContext) AllComma() []antlr.TerminalNode {
	return s.GetTokens(SuricataRuleParserComma)
}

func (s *PortContext) Comma(i int) antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserComma, i)
}

func (s *PortContext) Negative() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserNegative, 0)
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
	p.EnterRule(localctx, 38, SuricataRuleParserRULE_port)
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

	p.SetState(184)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 10, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(159)
			p.Match(SuricataRuleParserAny)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(160)
			p.Environment_var()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(161)
			p.Match(SuricataRuleParserINT)
		}

	case 4:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(162)
			p.Match(SuricataRuleParserINT)
		}
		{
			p.SetState(163)
			p.Match(SuricataRuleParserColon)
		}
		p.SetState(165)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SuricataRuleParserINT {
			{
				p.SetState(164)
				p.Match(SuricataRuleParserINT)
			}

		}

	case 5:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(167)
			p.Match(SuricataRuleParserColon)
		}
		{
			p.SetState(168)
			p.Match(SuricataRuleParserINT)
		}

	case 6:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(169)
			p.Match(SuricataRuleParserINT)
		}
		{
			p.SetState(170)
			p.Match(SuricataRuleParserColon)
		}

	case 7:
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(171)
			p.Match(SuricataRuleParserLBracket)
		}
		{
			p.SetState(172)
			p.Port()
		}
		p.SetState(177)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == SuricataRuleParserComma {
			{
				p.SetState(173)
				p.Match(SuricataRuleParserComma)
			}
			{
				p.SetState(174)
				p.Port()
			}

			p.SetState(179)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(180)
			p.Match(SuricataRuleParserRBracket)
		}

	case 8:
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(182)
			p.Match(SuricataRuleParserNegative)
		}
		{
			p.SetState(183)
			p.Port()
		}

	}

	return localctx
}

// IParamsContext is an interface to support dynamic dispatch.
type IParamsContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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
	p.EnterRule(localctx, 40, SuricataRuleParserRULE_params)
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
		p.SetState(186)
		p.Match(SuricataRuleParserParamStart)
	}
	{
		p.SetState(187)
		p.Param()
	}
	p.SetState(192)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 11, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(188)
				p.Match(SuricataRuleParserParamSep)
			}
			{
				p.SetState(189)
				p.Param()
			}

		}
		p.SetState(194)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 11, p.GetParserRuleContext())
	}
	p.SetState(196)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserParamSep {
		{
			p.SetState(195)
			p.Match(SuricataRuleParserParamSep)
		}

	}
	{
		p.SetState(198)
		p.Match(SuricataRuleParserParamEnd)
	}

	return localctx
}

// IParamContext is an interface to support dynamic dispatch.
type IParamContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

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

func (s *ParamContext) Keyword() IKeywordContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IKeywordContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IKeywordContext)
}

func (s *ParamContext) ParamColon() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamColon, 0)
}

func (s *ParamContext) Setting() ISettingContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISettingContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISettingContext)
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
	p.EnterRule(localctx, 42, SuricataRuleParserRULE_param)
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
		p.SetState(200)
		p.Keyword()
	}
	p.SetState(203)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserParamColon {
		{
			p.SetState(201)
			p.Match(SuricataRuleParserParamColon)
		}
		{
			p.SetState(202)
			p.Setting()
		}

	}

	return localctx
}

// IKeywordContext is an interface to support dynamic dispatch.
type IKeywordContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsKeywordContext differentiates from other interfaces.
	IsKeywordContext()
}

type KeywordContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyKeywordContext() *KeywordContext {
	var p = new(KeywordContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_keyword
	return p
}

func (*KeywordContext) IsKeywordContext() {}

func NewKeywordContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *KeywordContext {
	var p = new(KeywordContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_keyword

	return p
}

func (s *KeywordContext) GetParser() antlr.Parser { return s.parser }

func (s *KeywordContext) ParamCommonString() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamCommonString, 0)
}

func (s *KeywordContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *KeywordContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *KeywordContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitKeyword(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Keyword() (localctx IKeywordContext) {
	this := p
	_ = this

	localctx = NewKeywordContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 44, SuricataRuleParserRULE_keyword)

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
		p.SetState(205)
		p.Match(SuricataRuleParserParamCommonString)
	}

	return localctx
}

// ISettingContext is an interface to support dynamic dispatch.
type ISettingContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsSettingContext differentiates from other interfaces.
	IsSettingContext()
}

type SettingContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySettingContext() *SettingContext {
	var p = new(SettingContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_setting
	return p
}

func (*SettingContext) IsSettingContext() {}

func NewSettingContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SettingContext {
	var p = new(SettingContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_setting

	return p
}

func (s *SettingContext) GetParser() antlr.Parser { return s.parser }

func (s *SettingContext) AllSingleSetting() []ISingleSettingContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ISingleSettingContext); ok {
			len++
		}
	}

	tst := make([]ISingleSettingContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ISingleSettingContext); ok {
			tst[i] = t.(ISingleSettingContext)
			i++
		}
	}

	return tst
}

func (s *SettingContext) SingleSetting(i int) ISingleSettingContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISingleSettingContext); ok {
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

	return t.(ISingleSettingContext)
}

func (s *SettingContext) AllParamComma() []antlr.TerminalNode {
	return s.GetTokens(SuricataRuleParserParamComma)
}

func (s *SettingContext) ParamComma(i int) antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamComma, i)
}

func (s *SettingContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SettingContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *SettingContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitSetting(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Setting() (localctx ISettingContext) {
	this := p
	_ = this

	localctx = NewSettingContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 46, SuricataRuleParserRULE_setting)
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
		p.SetState(207)
		p.SingleSetting()
	}
	p.SetState(212)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == SuricataRuleParserParamComma {
		{
			p.SetState(208)
			p.Match(SuricataRuleParserParamComma)
		}
		{
			p.SetState(209)
			p.SingleSetting()
		}

		p.SetState(214)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}

	return localctx
}

// ISingleSettingContext is an interface to support dynamic dispatch.
type ISingleSettingContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsSingleSettingContext differentiates from other interfaces.
	IsSingleSettingContext()
}

type SingleSettingContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySingleSettingContext() *SingleSettingContext {
	var p = new(SingleSettingContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_singleSetting
	return p
}

func (*SingleSettingContext) IsSingleSettingContext() {}

func NewSingleSettingContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SingleSettingContext {
	var p = new(SingleSettingContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_singleSetting

	return p
}

func (s *SingleSettingContext) GetParser() antlr.Parser { return s.parser }

func (s *SingleSettingContext) ParamCommonString() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamCommonString, 0)
}

func (s *SingleSettingContext) ParamQuotedString() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamQuotedString, 0)
}

func (s *SingleSettingContext) ParamNegative() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamNegative, 0)
}

func (s *SingleSettingContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SingleSettingContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *SingleSettingContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitSingleSetting(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) SingleSetting() (localctx ISingleSettingContext) {
	this := p
	_ = this

	localctx = NewSingleSettingContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 48, SuricataRuleParserRULE_singleSetting)
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
	p.SetState(216)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserParamNegative {
		{
			p.SetState(215)
			p.Match(SuricataRuleParserParamNegative)
		}

	}
	{
		p.SetState(218)
		_la = p.GetTokenStream().LA(1)

		if !(_la == SuricataRuleParserParamQuotedString || _la == SuricataRuleParserParamCommonString) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}

	return localctx
}

func (p *SuricataRuleParser) Sempred(localctx antlr.RuleContext, ruleIndex, predIndex int) bool {
	switch ruleIndex {
	case 14:
		var t *Ipv6partContext = nil
		if localctx != nil {
			t = localctx.(*Ipv6partContext)
		}
		return p.Ipv6part_Sempred(t, predIndex)

	default:
		panic("No predicate with index: " + fmt.Sprint(ruleIndex))
	}
}

func (p *SuricataRuleParser) Ipv6part_Sempred(localctx antlr.RuleContext, predIndex int) bool {
	this := p
	_ = this

	switch predIndex {
	case 0:
		return p.Precpred(p.GetParserRuleContext(), 1)

	default:
		panic("No predicate with index: " + fmt.Sprint(predIndex))
	}
}
