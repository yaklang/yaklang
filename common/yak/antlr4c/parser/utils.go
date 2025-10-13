package c

func GetParserSerializedATN() []int32 {
	staticData := &cparserParserStaticData
	staticData.once.Do(cparserParserInit)
	return staticData.serializedATN
}

func GetLexerSerializedATN() []int32 {
	staticData := &clexerLexerStaticData
	staticData.once.Do(clexerLexerInit)
	return staticData.serializedATN
}
