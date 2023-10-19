package regen

import (
	"regexp/syntax"
	"unicode"
	"unicode/utf8"
)

var (
	generatorOneFactories        map[syntax.Op]generatorFactory
	generatorVisibleOneFactories map[syntax.Op]generatorFactory
)

func createCharClassGeneratorOne(name string, charClass *tCharClass, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{Name: name, GenerateFunc: func() []string {
		return charClass.GetOneRuneAsString()
	}}, nil
}

func createCharClassGeneratorVisibleOne(name string, charClass *tCharClass, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{Name: name, GenerateFunc: func() []string {
		return charClass.GetVisibleOneRuneAsString()
	}}, nil
}

func isWordBoundaryCompatible(c1, c2 rune) bool {
	// Assuming a word character is alphanumeric
	return (unicode.IsLetter(c1) || unicode.IsDigit(c1)) != (unicode.IsLetter(c2) || unicode.IsDigit(c2))
}
func lastChar(s string) rune {
	if len(s) == 0 {
		return 0
	}
	r, _ := utf8.DecodeLastRuneInString(s)
	return r
}
func firstChar(s string) rune {
	if len(s) == 0 {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(s)
	return r
}
