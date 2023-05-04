package parser

// Returns true if the current Token is a closing bracket (")" or "}")
func (p *YaklangParser) closingBracket() bool {
	stream := p.GetTokenStream()
	prevTokenType := stream.LA(1)
	stream.LA(-1)
	//return prevTokenType == GoParserR_PAREN || prevTokenType == GoParserR_CURLY;
	return prevTokenType == YaklangParserRParen || prevTokenType == YaklangParserRBrace || prevTokenType == YaklangParserEOF
}
