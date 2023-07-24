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
		"negative", "settingcontent",
	}
	staticData.predictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 44, 229, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7, 20, 2,
		21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25, 2, 26,
		7, 26, 1, 0, 4, 0, 56, 8, 0, 11, 0, 12, 0, 57, 1, 0, 1, 0, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2, 1, 2, 1, 3, 1, 3, 1, 4,
		1, 4, 1, 5, 1, 5, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 1, 6, 5, 6,
		87, 8, 6, 10, 6, 12, 6, 90, 9, 6, 1, 6, 1, 6, 1, 6, 1, 6, 3, 6, 96, 8,
		6, 1, 7, 1, 7, 1, 7, 1, 7, 1, 7, 1, 7, 1, 7, 1, 7, 1, 7, 3, 7, 107, 8,
		7, 1, 8, 1, 8, 1, 9, 1, 9, 1, 10, 1, 10, 1, 10, 1, 11, 1, 11, 3, 11, 118,
		8, 11, 1, 11, 1, 11, 3, 11, 122, 8, 11, 1, 12, 1, 12, 1, 12, 1, 12, 1,
		12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12,
		1, 12, 1, 13, 1, 13, 1, 13, 1, 13, 1, 14, 1, 14, 3, 14, 146, 8, 14, 1,
		14, 1, 14, 1, 14, 5, 14, 151, 8, 14, 10, 14, 12, 14, 154, 9, 14, 1, 15,
		1, 15, 1, 16, 1, 16, 1, 17, 1, 17, 1, 18, 1, 18, 1, 19, 1, 19, 1, 19, 1,
		19, 1, 19, 1, 19, 3, 19, 170, 8, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19,
		1, 19, 1, 19, 1, 19, 5, 19, 180, 8, 19, 10, 19, 12, 19, 183, 9, 19, 1,
		19, 1, 19, 1, 19, 1, 19, 3, 19, 189, 8, 19, 1, 20, 1, 20, 1, 20, 1, 20,
		5, 20, 195, 8, 20, 10, 20, 12, 20, 198, 9, 20, 1, 20, 3, 20, 201, 8, 20,
		1, 20, 1, 20, 1, 21, 1, 21, 1, 21, 3, 21, 208, 8, 21, 1, 22, 1, 22, 1,
		23, 1, 23, 1, 23, 5, 23, 215, 8, 23, 10, 23, 12, 23, 218, 9, 23, 1, 24,
		3, 24, 221, 8, 24, 1, 24, 1, 24, 1, 25, 1, 25, 1, 26, 1, 26, 1, 26, 0,
		1, 28, 27, 0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30, 32,
		34, 36, 38, 40, 42, 44, 46, 48, 50, 52, 0, 3, 1, 0, 4, 5, 1, 0, 30, 31,
		2, 0, 39, 39, 44, 44, 227, 0, 55, 1, 0, 0, 0, 2, 61, 1, 0, 0, 0, 4, 70,
		1, 0, 0, 0, 6, 72, 1, 0, 0, 0, 8, 74, 1, 0, 0, 0, 10, 76, 1, 0, 0, 0, 12,
		95, 1, 0, 0, 0, 14, 97, 1, 0, 0, 0, 16, 108, 1, 0, 0, 0, 18, 110, 1, 0,
		0, 0, 20, 112, 1, 0, 0, 0, 22, 117, 1, 0, 0, 0, 24, 123, 1, 0, 0, 0, 26,
		139, 1, 0, 0, 0, 28, 143, 1, 0, 0, 0, 30, 155, 1, 0, 0, 0, 32, 157, 1,
		0, 0, 0, 34, 159, 1, 0, 0, 0, 36, 161, 1, 0, 0, 0, 38, 188, 1, 0, 0, 0,
		40, 190, 1, 0, 0, 0, 42, 204, 1, 0, 0, 0, 44, 209, 1, 0, 0, 0, 46, 211,
		1, 0, 0, 0, 48, 220, 1, 0, 0, 0, 50, 224, 1, 0, 0, 0, 52, 226, 1, 0, 0,
		0, 54, 56, 3, 2, 1, 0, 55, 54, 1, 0, 0, 0, 56, 57, 1, 0, 0, 0, 57, 55,
		1, 0, 0, 0, 57, 58, 1, 0, 0, 0, 58, 59, 1, 0, 0, 0, 59, 60, 5, 0, 0, 1,
		60, 1, 1, 0, 0, 0, 61, 62, 3, 4, 2, 0, 62, 63, 3, 6, 3, 0, 63, 64, 3, 8,
		4, 0, 64, 65, 3, 34, 17, 0, 65, 66, 7, 0, 0, 0, 66, 67, 3, 10, 5, 0, 67,
		68, 3, 36, 18, 0, 68, 69, 3, 40, 20, 0, 69, 3, 1, 0, 0, 0, 70, 71, 5, 32,
		0, 0, 71, 5, 1, 0, 0, 0, 72, 73, 5, 32, 0, 0, 73, 7, 1, 0, 0, 0, 74, 75,
		3, 12, 6, 0, 75, 9, 1, 0, 0, 0, 76, 77, 3, 12, 6, 0, 77, 11, 1, 0, 0, 0,
		78, 96, 5, 1, 0, 0, 79, 96, 3, 20, 10, 0, 80, 96, 3, 14, 7, 0, 81, 96,
		3, 22, 11, 0, 82, 83, 5, 19, 0, 0, 83, 88, 3, 12, 6, 0, 84, 85, 5, 24,
		0, 0, 85, 87, 3, 12, 6, 0, 86, 84, 1, 0, 0, 0, 87, 90, 1, 0, 0, 0, 88,
		86, 1, 0, 0, 0, 88, 89, 1, 0, 0, 0, 89, 91, 1, 0, 0, 0, 90, 88, 1, 0, 0,
		0, 91, 92, 5, 20, 0, 0, 92, 96, 1, 0, 0, 0, 93, 94, 5, 2, 0, 0, 94, 96,
		3, 12, 6, 0, 95, 78, 1, 0, 0, 0, 95, 79, 1, 0, 0, 0, 95, 80, 1, 0, 0, 0,
		95, 81, 1, 0, 0, 0, 95, 82, 1, 0, 0, 0, 95, 93, 1, 0, 0, 0, 96, 13, 1,
		0, 0, 0, 97, 98, 3, 16, 8, 0, 98, 99, 5, 27, 0, 0, 99, 100, 3, 16, 8, 0,
		100, 101, 5, 27, 0, 0, 101, 102, 3, 16, 8, 0, 102, 103, 5, 27, 0, 0, 103,
		106, 3, 16, 8, 0, 104, 105, 5, 7, 0, 0, 105, 107, 3, 18, 9, 0, 106, 104,
		1, 0, 0, 0, 106, 107, 1, 0, 0, 0, 107, 15, 1, 0, 0, 0, 108, 109, 5, 30,
		0, 0, 109, 17, 1, 0, 0, 0, 110, 111, 5, 30, 0, 0, 111, 19, 1, 0, 0, 0,
		112, 113, 5, 3, 0, 0, 113, 114, 5, 32, 0, 0, 114, 21, 1, 0, 0, 0, 115,
		118, 3, 24, 12, 0, 116, 118, 3, 26, 13, 0, 117, 115, 1, 0, 0, 0, 117, 116,
		1, 0, 0, 0, 118, 121, 1, 0, 0, 0, 119, 120, 5, 7, 0, 0, 120, 122, 3, 32,
		16, 0, 121, 119, 1, 0, 0, 0, 121, 122, 1, 0, 0, 0, 122, 23, 1, 0, 0, 0,
		123, 124, 3, 30, 15, 0, 124, 125, 5, 17, 0, 0, 125, 126, 3, 30, 15, 0,
		126, 127, 5, 17, 0, 0, 127, 128, 3, 30, 15, 0, 128, 129, 5, 17, 0, 0, 129,
		130, 3, 30, 15, 0, 130, 131, 5, 17, 0, 0, 131, 132, 3, 30, 15, 0, 132,
		133, 5, 17, 0, 0, 133, 134, 3, 30, 15, 0, 134, 135, 5, 17, 0, 0, 135, 136,
		3, 30, 15, 0, 136, 137, 5, 17, 0, 0, 137, 138, 3, 30, 15, 0, 138, 25, 1,
		0, 0, 0, 139, 140, 3, 28, 14, 0, 140, 141, 5, 18, 0, 0, 141, 142, 3, 28,
		14, 0, 142, 27, 1, 0, 0, 0, 143, 145, 6, 14, -1, 0, 144, 146, 3, 30, 15,
		0, 145, 144, 1, 0, 0, 0, 145, 146, 1, 0, 0, 0, 146, 152, 1, 0, 0, 0, 147,
		148, 10, 1, 0, 0, 148, 149, 5, 17, 0, 0, 149, 151, 3, 30, 15, 0, 150, 147,
		1, 0, 0, 0, 151, 154, 1, 0, 0, 0, 152, 150, 1, 0, 0, 0, 152, 153, 1, 0,
		0, 0, 153, 29, 1, 0, 0, 0, 154, 152, 1, 0, 0, 0, 155, 156, 7, 1, 0, 0,
		156, 31, 1, 0, 0, 0, 157, 158, 5, 30, 0, 0, 158, 33, 1, 0, 0, 0, 159, 160,
		3, 38, 19, 0, 160, 35, 1, 0, 0, 0, 161, 162, 3, 38, 19, 0, 162, 37, 1,
		0, 0, 0, 163, 189, 5, 1, 0, 0, 164, 189, 3, 20, 10, 0, 165, 189, 5, 30,
		0, 0, 166, 167, 5, 30, 0, 0, 167, 169, 5, 17, 0, 0, 168, 170, 5, 30, 0,
		0, 169, 168, 1, 0, 0, 0, 169, 170, 1, 0, 0, 0, 170, 189, 1, 0, 0, 0, 171,
		172, 5, 17, 0, 0, 172, 189, 5, 30, 0, 0, 173, 174, 5, 30, 0, 0, 174, 189,
		5, 17, 0, 0, 175, 176, 5, 19, 0, 0, 176, 181, 3, 38, 19, 0, 177, 178, 5,
		24, 0, 0, 178, 180, 3, 38, 19, 0, 179, 177, 1, 0, 0, 0, 180, 183, 1, 0,
		0, 0, 181, 179, 1, 0, 0, 0, 181, 182, 1, 0, 0, 0, 182, 184, 1, 0, 0, 0,
		183, 181, 1, 0, 0, 0, 184, 185, 5, 20, 0, 0, 185, 189, 1, 0, 0, 0, 186,
		187, 5, 2, 0, 0, 187, 189, 3, 38, 19, 0, 188, 163, 1, 0, 0, 0, 188, 164,
		1, 0, 0, 0, 188, 165, 1, 0, 0, 0, 188, 166, 1, 0, 0, 0, 188, 171, 1, 0,
		0, 0, 188, 173, 1, 0, 0, 0, 188, 175, 1, 0, 0, 0, 188, 186, 1, 0, 0, 0,
		189, 39, 1, 0, 0, 0, 190, 191, 5, 21, 0, 0, 191, 196, 3, 42, 21, 0, 192,
		193, 5, 41, 0, 0, 193, 195, 3, 42, 21, 0, 194, 192, 1, 0, 0, 0, 195, 198,
		1, 0, 0, 0, 196, 194, 1, 0, 0, 0, 196, 197, 1, 0, 0, 0, 197, 200, 1, 0,
		0, 0, 198, 196, 1, 0, 0, 0, 199, 201, 5, 41, 0, 0, 200, 199, 1, 0, 0, 0,
		200, 201, 1, 0, 0, 0, 201, 202, 1, 0, 0, 0, 202, 203, 5, 38, 0, 0, 203,
		41, 1, 0, 0, 0, 204, 207, 3, 44, 22, 0, 205, 206, 5, 40, 0, 0, 206, 208,
		3, 46, 23, 0, 207, 205, 1, 0, 0, 0, 207, 208, 1, 0, 0, 0, 208, 43, 1, 0,
		0, 0, 209, 210, 5, 44, 0, 0, 210, 45, 1, 0, 0, 0, 211, 216, 3, 48, 24,
		0, 212, 213, 5, 43, 0, 0, 213, 215, 3, 48, 24, 0, 214, 212, 1, 0, 0, 0,
		215, 218, 1, 0, 0, 0, 216, 214, 1, 0, 0, 0, 216, 217, 1, 0, 0, 0, 217,
		47, 1, 0, 0, 0, 218, 216, 1, 0, 0, 0, 219, 221, 3, 50, 25, 0, 220, 219,
		1, 0, 0, 0, 220, 221, 1, 0, 0, 0, 221, 222, 1, 0, 0, 0, 222, 223, 3, 52,
		26, 0, 223, 49, 1, 0, 0, 0, 224, 225, 5, 42, 0, 0, 225, 51, 1, 0, 0, 0,
		226, 227, 7, 2, 0, 0, 227, 53, 1, 0, 0, 0, 16, 57, 88, 95, 106, 117, 121,
		145, 152, 169, 181, 188, 196, 200, 207, 216, 220,
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
	SuricataRuleParserRULE_negative        = 25
	SuricataRuleParserRULE_settingcontent  = 26
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
	p.SetState(55)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for ok := true; ok; ok = _la == SuricataRuleParserID {
		{
			p.SetState(54)
			p.Rule_()
		}

		p.SetState(57)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(59)
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
		p.SetState(61)
		p.Action_()
	}
	{
		p.SetState(62)
		p.Protocol()
	}
	{
		p.SetState(63)
		p.Src_address()
	}
	{
		p.SetState(64)
		p.Src_port()
	}
	{
		p.SetState(65)
		_la = p.GetTokenStream().LA(1)

		if !(_la == SuricataRuleParserArrow || _la == SuricataRuleParserBothDirect) {
			p.GetErrorHandler().RecoverInline(p)
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}
	{
		p.SetState(66)
		p.Dest_address()
	}
	{
		p.SetState(67)
		p.Dest_port()
	}
	{
		p.SetState(68)
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
		p.SetState(70)
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
		p.SetState(72)
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
		p.SetState(74)
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
		p.SetState(76)
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

	p.SetState(95)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 2, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(78)
			p.Match(SuricataRuleParserAny)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(79)
			p.Environment_var()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(80)
			p.Ipv4()
		}

	case 4:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(81)
			p.Ipv6()
		}

	case 5:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(82)
			p.Match(SuricataRuleParserLBracket)
		}
		{
			p.SetState(83)
			p.Address()
		}
		p.SetState(88)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == SuricataRuleParserComma {
			{
				p.SetState(84)
				p.Match(SuricataRuleParserComma)
			}
			{
				p.SetState(85)
				p.Address()
			}

			p.SetState(90)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(91)
			p.Match(SuricataRuleParserRBracket)
		}

	case 6:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(93)
			p.Match(SuricataRuleParserNegative)
		}
		{
			p.SetState(94)
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
	{
		p.SetState(100)
		p.Match(SuricataRuleParserDot)
	}
	{
		p.SetState(101)
		p.Ipv4block()
	}
	{
		p.SetState(102)
		p.Match(SuricataRuleParserDot)
	}
	{
		p.SetState(103)
		p.Ipv4block()
	}
	p.SetState(106)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserDiv {
		{
			p.SetState(104)
			p.Match(SuricataRuleParserDiv)
		}
		{
			p.SetState(105)
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
		p.SetState(108)
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
		p.SetState(110)
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
		p.SetState(112)
		p.Match(SuricataRuleParserDollar)
	}
	{
		p.SetState(113)
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
	p.SetState(117)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 4, p.GetParserRuleContext()) {
	case 1:
		{
			p.SetState(115)
			p.Ipv6full()
		}

	case 2:
		{
			p.SetState(116)
			p.Ipv6compact()
		}

	}
	p.SetState(121)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserDiv {
		{
			p.SetState(119)
			p.Match(SuricataRuleParserDiv)
		}
		{
			p.SetState(120)
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
	{
		p.SetState(134)
		p.Match(SuricataRuleParserColon)
	}
	{
		p.SetState(135)
		p.Ipv6block()
	}
	{
		p.SetState(136)
		p.Match(SuricataRuleParserColon)
	}
	{
		p.SetState(137)
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
		p.SetState(139)
		p.ipv6part(0)
	}
	{
		p.SetState(140)
		p.Match(SuricataRuleParserDoubleColon)
	}
	{
		p.SetState(141)
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
	p.SetState(145)
	p.GetErrorHandler().Sync(p)

	if p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 6, p.GetParserRuleContext()) == 1 {
		{
			p.SetState(144)
			p.Ipv6block()
		}

	}

	p.GetParserRuleContext().SetStop(p.GetTokenStream().LT(-1))
	p.SetState(152)
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
			p.SetState(147)

			if !(p.Precpred(p.GetParserRuleContext(), 1)) {
				panic(antlr.NewFailedPredicateException(p, "p.Precpred(p.GetParserRuleContext(), 1)", ""))
			}
			{
				p.SetState(148)
				p.Match(SuricataRuleParserColon)
			}
			{
				p.SetState(149)
				p.Ipv6block()
			}

		}
		p.SetState(154)
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
		p.SetState(155)
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
		p.SetState(157)
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
		p.SetState(159)
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
		p.SetState(161)
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

	p.SetState(188)
	p.GetErrorHandler().Sync(p)
	switch p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 10, p.GetParserRuleContext()) {
	case 1:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(163)
			p.Match(SuricataRuleParserAny)
		}

	case 2:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(164)
			p.Environment_var()
		}

	case 3:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(165)
			p.Match(SuricataRuleParserINT)
		}

	case 4:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(166)
			p.Match(SuricataRuleParserINT)
		}
		{
			p.SetState(167)
			p.Match(SuricataRuleParserColon)
		}
		p.SetState(169)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		if _la == SuricataRuleParserINT {
			{
				p.SetState(168)
				p.Match(SuricataRuleParserINT)
			}

		}

	case 5:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(171)
			p.Match(SuricataRuleParserColon)
		}
		{
			p.SetState(172)
			p.Match(SuricataRuleParserINT)
		}

	case 6:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(173)
			p.Match(SuricataRuleParserINT)
		}
		{
			p.SetState(174)
			p.Match(SuricataRuleParserColon)
		}

	case 7:
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(175)
			p.Match(SuricataRuleParserLBracket)
		}
		{
			p.SetState(176)
			p.Port()
		}
		p.SetState(181)
		p.GetErrorHandler().Sync(p)
		_la = p.GetTokenStream().LA(1)

		for _la == SuricataRuleParserComma {
			{
				p.SetState(177)
				p.Match(SuricataRuleParserComma)
			}
			{
				p.SetState(178)
				p.Port()
			}

			p.SetState(183)
			p.GetErrorHandler().Sync(p)
			_la = p.GetTokenStream().LA(1)
		}
		{
			p.SetState(184)
			p.Match(SuricataRuleParserRBracket)
		}

	case 8:
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(186)
			p.Match(SuricataRuleParserNegative)
		}
		{
			p.SetState(187)
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
		p.SetState(190)
		p.Match(SuricataRuleParserParamStart)
	}
	{
		p.SetState(191)
		p.Param()
	}
	p.SetState(196)
	p.GetErrorHandler().Sync(p)
	_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 11, p.GetParserRuleContext())

	for _alt != 2 && _alt != antlr.ATNInvalidAltNumber {
		if _alt == 1 {
			{
				p.SetState(192)
				p.Match(SuricataRuleParserParamSep)
			}
			{
				p.SetState(193)
				p.Param()
			}

		}
		p.SetState(198)
		p.GetErrorHandler().Sync(p)
		_alt = p.GetInterpreter().AdaptivePredict(p.GetTokenStream(), 11, p.GetParserRuleContext())
	}
	p.SetState(200)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserParamSep {
		{
			p.SetState(199)
			p.Match(SuricataRuleParserParamSep)
		}

	}
	{
		p.SetState(202)
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
		p.SetState(204)
		p.Keyword()
	}
	p.SetState(207)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserParamColon {
		{
			p.SetState(205)
			p.Match(SuricataRuleParserParamColon)
		}
		{
			p.SetState(206)
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
		p.SetState(209)
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
		p.SetState(211)
		p.SingleSetting()
	}
	p.SetState(216)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	for _la == SuricataRuleParserParamComma {
		{
			p.SetState(212)
			p.Match(SuricataRuleParserParamComma)
		}
		{
			p.SetState(213)
			p.SingleSetting()
		}

		p.SetState(218)
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

func (s *SingleSettingContext) Settingcontent() ISettingcontentContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ISettingcontentContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ISettingcontentContext)
}

func (s *SingleSettingContext) Negative() INegativeContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INegativeContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INegativeContext)
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
	p.SetState(220)
	p.GetErrorHandler().Sync(p)
	_la = p.GetTokenStream().LA(1)

	if _la == SuricataRuleParserParamNegative {
		{
			p.SetState(219)
			p.Negative()
		}

	}
	{
		p.SetState(222)
		p.Settingcontent()
	}

	return localctx
}

// INegativeContext is an interface to support dynamic dispatch.
type INegativeContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsNegativeContext differentiates from other interfaces.
	IsNegativeContext()
}

type NegativeContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNegativeContext() *NegativeContext {
	var p = new(NegativeContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_negative
	return p
}

func (*NegativeContext) IsNegativeContext() {}

func NewNegativeContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NegativeContext {
	var p = new(NegativeContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_negative

	return p
}

func (s *NegativeContext) GetParser() antlr.Parser { return s.parser }

func (s *NegativeContext) ParamNegative() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamNegative, 0)
}

func (s *NegativeContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NegativeContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NegativeContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitNegative(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Negative() (localctx INegativeContext) {
	this := p
	_ = this

	localctx = NewNegativeContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 50, SuricataRuleParserRULE_negative)

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
		p.SetState(224)
		p.Match(SuricataRuleParserParamNegative)
	}

	return localctx
}

// ISettingcontentContext is an interface to support dynamic dispatch.
type ISettingcontentContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// IsSettingcontentContext differentiates from other interfaces.
	IsSettingcontentContext()
}

type SettingcontentContext struct {
	*antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptySettingcontentContext() *SettingcontentContext {
	var p = new(SettingcontentContext)
	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(nil, -1)
	p.RuleIndex = SuricataRuleParserRULE_settingcontent
	return p
}

func (*SettingcontentContext) IsSettingcontentContext() {}

func NewSettingcontentContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *SettingcontentContext {
	var p = new(SettingcontentContext)

	p.BaseParserRuleContext = antlr.NewBaseParserRuleContext(parent, invokingState)

	p.parser = parser
	p.RuleIndex = SuricataRuleParserRULE_settingcontent

	return p
}

func (s *SettingcontentContext) GetParser() antlr.Parser { return s.parser }

func (s *SettingcontentContext) ParamCommonString() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamCommonString, 0)
}

func (s *SettingcontentContext) ParamQuotedString() antlr.TerminalNode {
	return s.GetToken(SuricataRuleParserParamQuotedString, 0)
}

func (s *SettingcontentContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *SettingcontentContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *SettingcontentContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case SuricataRuleParserVisitor:
		return t.VisitSettingcontent(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *SuricataRuleParser) Settingcontent() (localctx ISettingcontentContext) {
	this := p
	_ = this

	localctx = NewSettingcontentContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 52, SuricataRuleParserRULE_settingcontent)
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
		p.SetState(226)
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
