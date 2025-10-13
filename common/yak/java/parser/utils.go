package javaparser

func GetJavaParserSerializedATN() []int32 {
	javaparserParserInit()
	return javaparserParserStaticData.serializedATN
}

func GetJavaLexerSerializedATN() []int32 {
	JavaLexerInit()
	return javalexerLexerStaticData.serializedATN
}
