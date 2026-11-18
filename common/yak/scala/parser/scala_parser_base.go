package scalaparser

import "github.com/yaklang/antlr/v4"

// ScalaParserBase is ported from grammars-v4 scala/scala3/Java/Scala3ParserBase.java.
type ScalaParserBase struct {
	*antlr.BaseParser
}

func (p *ScalaParserBase) migration30() bool {
	// Accept Scala 2 wildcard syntax (`_`) alongside Scala 3 syntax.
	return true
}
