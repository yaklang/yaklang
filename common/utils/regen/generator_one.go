package regen

import (
	"context"
	"regexp/syntax"
	"unicode"
	"unicode/utf8"
)

var (
	generatorOneFactories        map[syntax.Op]generatorFactory
	generatorVisibleOneFactories map[syntax.Op]generatorFactory
)

func createCharClassGeneratorOne(name string, charClass *tCharClass, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{
		Name: name,
		GenerateFunc: func() []string {
			return charClass.GetOneRuneAsString()
		},
		GenerateStreamFunc: func(ctx context.Context, c chan string) error {
			defer close(c)

			for _, s := range charClass.GetOneRuneAsString() {
				tryPutChannel(ctx, c, s)
			}
			return nil
		},
	}, nil
}

func createCharClassGeneratorVisibleOne(name string, charClass *tCharClass, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{
		Name: name,
		GenerateFunc: func() []string {
			return charClass.GetVisibleOneRuneAsString()
		},
		GenerateStreamFunc: func(ctx context.Context, c chan string) error {
			defer close(c)

			for _, s := range charClass.GetVisibleOneRuneAsString() {
				tryPutChannel(ctx, c, s)
			}
			return nil
		},
	}, nil
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
