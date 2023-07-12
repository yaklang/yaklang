package regen

import (
	"fmt"
	"math/rand"
	"regexp/syntax"

	"github.com/pkg/errors"
)

type generatorFactory func(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error)

var (
	generatorFactories map[syntax.Op]generatorFactory
	AllRunes           []rune
	AllRunesNL         []rune
	AllRunesAsString   []string
)

const noBound = -1

func init() {
	generatorFactories = map[syntax.Op]generatorFactory{
		syntax.OpEmptyMatch:     opEmptyMatch,
		syntax.OpLiteral:        opLiteral,
		syntax.OpAnyCharNotNL:   opAnyCharNotNl,
		syntax.OpAnyChar:        opAnyChar,
		syntax.OpQuest:          opQuest,
		syntax.OpStar:           opStar,
		syntax.OpPlus:           opPlus,
		syntax.OpRepeat:         opRepeat,
		syntax.OpCharClass:      opCharClass,
		syntax.OpConcat:         opConcat,
		syntax.OpAlternate:      opAlternate,
		syntax.OpCapture:        opCapture,
		syntax.OpBeginLine:      noop,
		syntax.OpEndLine:        noop,
		syntax.OpBeginText:      noop,
		syntax.OpEndText:        noop,
		syntax.OpWordBoundary:   noop,
		syntax.OpNoWordBoundary: noop,
	}
	generatorOneFactories = map[syntax.Op]generatorFactory{
		syntax.OpEmptyMatch:     opEmptyMatch,
		syntax.OpLiteral:        opLiteral,
		syntax.OpAnyCharNotNL:   opAnyCharNotNlOne,
		syntax.OpAnyChar:        opAnyCharOne,
		syntax.OpQuest:          opQuestOne,
		syntax.OpStar:           opStar,
		syntax.OpPlus:           opPlusOne,
		syntax.OpRepeat:         opRepeatOne,
		syntax.OpCharClass:      opCharClassOne,
		syntax.OpConcat:         opConcatOne,
		syntax.OpAlternate:      opAlternateOne,
		syntax.OpCapture:        opCaptureOne,
		syntax.OpBeginLine:      noop,
		syntax.OpEndLine:        noop,
		syntax.OpBeginText:      noop,
		syntax.OpEndText:        noop,
		syntax.OpWordBoundary:   noop,
		syntax.OpNoWordBoundary: noop,
	}
	AllRunes = make([]rune, 128)
	AllRunesNL = make([]rune, 0, 127)
	AllRunesAsString = make([]string, 128)
	for i := 0; i < 128; i++ {
		AllRunes[i] = rune(i)
		AllRunesAsString[i] = string(rune(i))
		if i != 10 {
			AllRunesNL = append(AllRunesNL, rune(i))
		}
	}
}

type internalGenerator struct {
	Name         string
	GenerateFunc func() []string
}

func (gen *internalGenerator) Generate() []string {
	return gen.GenerateFunc()
}

func (gen *internalGenerator) String() string {
	return gen.Name
}

func newGenerators(regexps []*syntax.Regexp, args *GeneratorArgs) ([]*internalGenerator, error) {
	generators := make([]*internalGenerator, len(regexps), len(regexps))
	var err error

	for i, subR := range regexps {
		generators[i], err = newGenerator(subR, args)
		if err != nil {
			return nil, err
		}
	}

	return generators, nil
}

func newGeneratorsOne(regexps []*syntax.Regexp, args *GeneratorArgs) ([]*internalGenerator, error) {
	generators := make([]*internalGenerator, len(regexps), len(regexps))
	var err error

	for i, subR := range regexps {
		generators[i], err = newGeneratorOne(subR, args)
		if err != nil {
			return nil, err
		}
	}

	return generators, nil
}

func newGenerator(regexp *syntax.Regexp, args *GeneratorArgs) (generator *internalGenerator, err error) {
	simplified := regexp.Simplify()

	factory, ok := generatorFactories[simplified.Op]
	if ok {
		return factory(simplified, args)
	}

	return nil, fmt.Errorf("invalid generator pattern: /%s/ as /%s/",
		regexp, simplified)
}

func newGeneratorOne(regexp *syntax.Regexp, args *GeneratorArgs) (generator *internalGenerator, err error) {
	simplified := regexp.Simplify()

	factory, ok := generatorOneFactories[simplified.Op]
	if ok {
		return factory(simplified, args)
	}

	return nil, fmt.Errorf("invalid generator pattern: /%s/ as /%s/",
		regexp, simplified)
}

func noop(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{regexp.String(), func() []string {
		return []string{""}
	}}, nil
}

func opEmptyMatch(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{regexp.String(), func() []string {
		return []string{""}
	}}, nil
}

func opLiteral(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {

	return &internalGenerator{regexp.String(), func() []string {
		return []string{runesToString(regexp.Rune...)}
	}}, nil
}

func opAnyChar(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{regexp.String(), func() []string {
		return AllRunesAsString
	}}, nil
}

func opAnyCharOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{regexp.String(), func() []string {
		if len(AllRunesAsString) > 0 {
			return []string{AllRunesAsString[rand.Intn(len(AllRunesAsString))]}
		}
		return []string{""}
	}}, nil
}

func opAnyCharNotNl(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	charClass, err := parseCharClass(AllRunesNL)
	if err != nil {
		return nil, err
	}
	return createCharClassGenerator(regexp.String(), charClass, args)
}

func opAnyCharNotNlOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	charClass, err := parseCharClass(AllRunesNL)
	if err != nil {
		return nil, err
	}
	return createCharClassGeneratorOne(regexp.String(), charClass, args)
}

func opQuest(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {

	return createRepeatingGenerator(regexp, args, 0, 1)
}

func opQuestOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{regexp.String(), func() []string {
		return []string{""}
	}}, nil
}

func opStar(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {

	return createRepeatingGenerator(regexp, args, noBound, noBound)
}

func opPlus(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {

	return createRepeatingGenerator(regexp, args, 1, noBound)
}

func opPlusOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return createRepeatingGeneratorOne(regexp, args, 1, noBound)
}

func opRepeat(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return createRepeatingGenerator(regexp, args, regexp.Min, regexp.Max)
}

func opRepeatOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return createRepeatingGeneratorOne(regexp, args, regexp.Min, regexp.Min)
}

func opCharClass(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {

	charClass, err := parseCharClass(regexp.Rune)
	if err != nil {
		return nil, err
	}
	return createCharClassGenerator(regexp.String(), charClass, args)
}

func opCharClassOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {

	charClass, err := parseCharClass(regexp.Rune)
	if err != nil {
		return nil, err
	}
	return createCharClassGeneratorOne(regexp.String(), charClass, args)
}

func opConcat(regexp *syntax.Regexp, genArgs *GeneratorArgs) (*internalGenerator, error) {

	generators, err := newGenerators(regexp.Sub, genArgs)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating generators for concat pattern /%s/", regexp)
	}

	return &internalGenerator{regexp.String(), func() []string {
		var sets [][]string
		for _, generator := range generators {
			sets = append(sets, generator.Generate())
		}
		return ProductString(sets...)
	}}, nil
}

func opConcatOne(regexp *syntax.Regexp, genArgs *GeneratorArgs) (*internalGenerator, error) {
	generators, err := newGeneratorsOne(regexp.Sub, genArgs)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating generators for concat pattern /%s/", regexp)
	}

	return &internalGenerator{regexp.String(), func() []string {
		var sets [][]string
		for _, generator := range generators {
			rets := generator.Generate()
			if len(rets) > 0 {
				sets = append(sets, []string{rets[rand.Intn(len(rets))]})
			} else {
				sets = append(sets, []string{""})
			}
		}
		return ProductString(sets...)
	}}, nil
}

func opAlternate(regexp *syntax.Regexp, genArgs *GeneratorArgs) (*internalGenerator, error) {
	generators, err := newGenerators(regexp.Sub, genArgs)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating generators for alternate pattern /%s/", regexp)
	}

	return &internalGenerator{regexp.String(), func() []string {
		var sets []string
		for _, generator := range generators {
			sets = append(sets, generator.Generate()...)
		}
		return sets
	}}, nil
}

func opAlternateOne(regexp *syntax.Regexp, genArgs *GeneratorArgs) (*internalGenerator, error) {
	generators, err := newGeneratorsOne(regexp.Sub, genArgs)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating generators for alternate pattern /%s/", regexp)
	}

	return &internalGenerator{regexp.String(), func() []string {
		var sets []string
		if len(generators) > 0 {
			generator := generators[rand.Intn(len(generators))]
			return generator.Generate()
		}
		for _, generator := range generators {
			sets = append(sets, generator.Generate()...)
		}
		return sets
	}}, nil
}

func opCapture(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {

	if err := enforceSingleSub(regexp); err != nil {
		return nil, err
	}

	groupRegexp := regexp.Sub[0]
	generator, err := newGenerator(groupRegexp, args)
	if err != nil {
		return nil, err
	}

	index := regexp.Cap - 1

	return &internalGenerator{regexp.String(), func() []string {
		return args.CaptureGroupHandler(index, regexp.Name, groupRegexp, generator, args)
	}}, nil
}

func opCaptureOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {

	if err := enforceSingleSub(regexp); err != nil {
		return nil, err
	}

	groupRegexp := regexp.Sub[0]
	generator, err := newGeneratorOne(groupRegexp, args)
	if err != nil {
		return nil, err
	}

	index := regexp.Cap - 1

	return &internalGenerator{regexp.String(), func() []string {
		return args.CaptureGroupHandler(index, regexp.Name, groupRegexp, generator, args)
	}}, nil
}

func defaultCaptureGroupHandler(index int, name string, group *syntax.Regexp, generator Generator, args *GeneratorArgs) []string {
	return generator.Generate()
}

func enforceSingleSub(regexp *syntax.Regexp) error {
	if len(regexp.Sub) != 1 {
		return errors.New(fmt.Sprintf(
			"%s expected 1 sub-expression, but got %d: %s", opToString(regexp.Op), len(regexp.Sub), regexp))
	}
	return nil
}

func createCharClassGenerator(name string, charClass *tCharClass, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{name, func() []string {
		return charClass.GetAllRuneAsString()
	}}, nil
}

func createRepeatingGenerator(regexp *syntax.Regexp, genArgs *GeneratorArgs, min, max int) (*internalGenerator, error) {
	if err := enforceSingleSub(regexp); err != nil {
		return nil, err
	}

	generator, err := newGenerator(regexp.Sub[0], genArgs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create generator for subexpression: /%s/", regexp)
	}

	if min == noBound {
		min = 0
	}

	if max == noBound {
		max = min + 1
	}

	return &internalGenerator{regexp.String(), func() []string {
		var results []string
		gengerated := generator.Generate()

		sets := make([][]string, 0, min)
		for i := 0; i < min; i++ {
			sets = append(sets, gengerated)
		}
		for i := min; i <= max; i++ {
			results = append(results, ProductString(sets...)...)
			sets = append(sets, gengerated)
		}

		return results
	}}, nil
}

func createRepeatingGeneratorOne(regexp *syntax.Regexp, genArgs *GeneratorArgs, min, max int) (*internalGenerator, error) {
	if err := enforceSingleSub(regexp); err != nil {
		return nil, err
	}

	generator, err := newGeneratorOne(regexp.Sub[0], genArgs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create generator for subexpression: /%s/", regexp)
	}

	if min == noBound {
		min = 0
	}

	if max == noBound {
		max = min + 1
	}

	return &internalGenerator{regexp.String(), func() []string {
		genegerated := generator.Generate()
		return []string{genegerated[rand.Intn(len(genegerated))]}
	}}, nil
}
