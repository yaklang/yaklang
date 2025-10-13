package gol

func GetGoParserSerializedATN() []int32 {
	GoParserInit()
	return goparserParserStaticData.serializedATN
}

func GetGoLexerSerializedATN() []int32 {
	GoLexerInit()
	return golexerLexerStaticData.serializedATN
}
