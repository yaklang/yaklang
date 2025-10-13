package parser

func GetLexerSerializedATN() []int32 {
	YaklangLexerInit()
	return yaklanglexerLexerStaticData.serializedATN
}

func GetParserSerializedATN() []int32 {
	YaklangParserInit()
	return yaklangparserParserStaticData.serializedATN
}
