package phpparser

func GetPHPParserSerializedATN() []int32 {
	PHPParserInit()
	return phpparserParserStaticData.serializedATN
}

func GetPHPLexerSerializedATN() []int32 {
	PHPLexerInit()
	return phplexerLexerStaticData.serializedATN
}
