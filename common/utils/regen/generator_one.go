package regen

import "regexp/syntax"

var (
	generatorOneFactories map[syntax.Op]generatorFactory
)

func createCharClassGeneratorOne(name string, charClass *tCharClass, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{name, func() []string {
		return charClass.GetOneRuneAsString()
	}}, nil
}
