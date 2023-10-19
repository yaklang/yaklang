package regen

import (
	"fmt"
	"github.com/pkg/errors"
	"math/rand"
	"regexp/syntax"
	"unicode"
)

type generatorFactory func(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error)

var (
	generatorFactories   map[syntax.Op]generatorFactory
	AllRunes             []rune
	AllRunesNL           []rune
	AllRunesAsString     []string
	VisibleRunes         []rune
	VisibleRunesAsString []string
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
		syntax.OpStar:           opStarOne,
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

	generatorVisibleOneFactories = map[syntax.Op]generatorFactory{
		syntax.OpEmptyMatch:     opEmptyMatch,
		syntax.OpLiteral:        opLiteral,
		syntax.OpAnyCharNotNL:   opAnyCharNotNlVisibleOne,
		syntax.OpAnyChar:        opAnyCharVisibleOne,
		syntax.OpQuest:          opQuestVisibleOne,
		syntax.OpStar:           opStarVisibleOne,
		syntax.OpPlus:           opPlusVisibleOne,
		syntax.OpRepeat:         opRepeatVisibleOne,
		syntax.OpCharClass:      opCharClassVisibleOne,
		syntax.OpConcat:         opConcatVisibleOne,
		syntax.OpAlternate:      opAlternateVisibleOne,
		syntax.OpCapture:        opCaptureVisibleOne,
		syntax.OpBeginLine:      noop,
		syntax.OpEndLine:        noop,
		syntax.OpBeginText:      noop,
		syntax.OpEndText:        noop,
		syntax.OpWordBoundary:   noop,
		syntax.OpNoWordBoundary: noop,
	} // 初始化可见字符的字符串表示
	for i := 32; i <= 126; i++ {
		VisibleRunes = append(VisibleRunes, rune(i))
		VisibleRunesAsString = append(VisibleRunesAsString, string(rune(i)))
	}
}

type internalGenerator struct {
	Name         string
	GenerateFunc func() []string
	cache        []string
}

func (gen *internalGenerator) Generate() []string {
	if len(gen.cache) > 0 {
		result := gen.cache
		gen.cache = nil
		return result
	}
	return gen.GenerateFunc()
}

func (gen *internalGenerator) String() string {
	return gen.Name
}

func (gen *internalGenerator) Peek() []string {
	if len(gen.cache) == 0 {
		gen.cache = gen.GenerateFunc()
	}
	return gen.cache
}

func (gen *internalGenerator) CheckVisible(str string) bool {
	for _, r := range str {
		if unicode.IsPrint(r) {
			return true
		}
	}
	return false
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

func newGeneratorsVisibleOne(regexps []*syntax.Regexp, args *GeneratorArgs) ([]*internalGenerator, error) {
	generators := make([]*internalGenerator, len(regexps), len(regexps))
	var err error

	for i, subR := range regexps {
		generators[i], err = newGeneratorVisibleOne(subR, args)
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

func newGeneratorVisibleOne(regexp *syntax.Regexp, args *GeneratorArgs) (generator *internalGenerator, err error) {
	simplified := regexp.Simplify()

	factory, ok := generatorVisibleOneFactories[simplified.Op]
	if ok {
		return factory(simplified, args)
	}

	return nil, fmt.Errorf("invalid generator pattern: /%s/ as /%s/",
		regexp, simplified)
}

func noop(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
		return []string{""}
	}}, nil
}

func opEmptyMatch(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
		return []string{""}
	}}, nil
}

func opLiteral(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {

	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
		return []string{runesToString(regexp.Rune...)}
	}}, nil
}

func opAnyChar(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
		return AllRunesAsString
	}}, nil
}

func opAnyCharOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
		if len(AllRunesAsString) > 0 {
			return []string{AllRunesAsString[rand.Intn(len(AllRunesAsString))]}
		}
		return []string{""}
	}}, nil
}

// opAnyCharVisibleOne 函数为 OpAnyChar 操作符返回一个 generator，该 generator 只生成一个可见字符。
func opAnyCharVisibleOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{
		Name: regexp.String(),
		GenerateFunc: func() []string {
			return []string{VisibleRunesAsString[rand.Intn(len(VisibleRunesAsString))]}
		},
	}, nil
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

func opAnyCharNotNlVisibleOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	// 创建一个新的可见字符切片，但不包括换行符
	visibleRunesWithoutNL := make([]rune, 0, len(VisibleRunes))
	for _, r := range VisibleRunes {
		if r != '\n' && r != '\r' {
			visibleRunesWithoutNL = append(visibleRunesWithoutNL, r)
		}
	}

	charClass, err := parseCharClass(visibleRunesWithoutNL)
	if err != nil {
		return nil, err
	}
	return createCharClassGeneratorVisibleOne(regexp.String(), charClass, args)
}

func opQuest(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {

	return createRepeatingGenerator(regexp, args, 0, 1)
}

func opQuestOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
		return []string{""}
	}}, nil
}

func opQuestVisibleOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
		return []string{""}
	}}, nil
}

func opStar(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {

	return createRepeatingGenerator(regexp, args, noBound, noBound)
}

func opStarOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return createRepeatingGeneratorOne(regexp, args, 1, noBound)
}

func opStarVisibleOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return createRepeatingGeneratorVisibleOne(regexp, args, 1, noBound)
}

func opPlus(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {

	return createRepeatingGenerator(regexp, args, 1, noBound)
}

func opPlusOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return createRepeatingGeneratorOne(regexp, args, 1, noBound)
}

func opPlusVisibleOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return createRepeatingGeneratorVisibleOne(regexp, args, 1, noBound)

}

func opRepeat(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return createRepeatingGenerator(regexp, args, regexp.Min, regexp.Max)
}

func opRepeatOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return createRepeatingGeneratorOne(regexp, args, regexp.Min, regexp.Min)
}
func opRepeatVisibleOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	return createRepeatingGeneratorVisibleOne(regexp, args, regexp.Min, noBound)
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

func opCharClassVisibleOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {
	// Parse the character class from the regexp
	charClass, err := parseCharClass(regexp.Rune)
	if err != nil {
		return nil, err
	}

	// Use the filtered visible character class to generate the string
	return createCharClassGeneratorVisibleOne(regexp.String(), charClass, args)
}

func opConcat(regexp *syntax.Regexp, genArgs *GeneratorArgs) (*internalGenerator, error) {

	generators, err := newGenerators(regexp.Sub, genArgs)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating generators for concat pattern /%s/", regexp)
	}

	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
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

	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
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

func opConcatVisibleOne(regexp *syntax.Regexp, genArgs *GeneratorArgs) (*internalGenerator, error) {
	// 1. Create generators for the sub-patterns
	generators, err := newGeneratorsVisibleOne(regexp.Sub, genArgs)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating visible generators for concat pattern /%s/", regexp)
	}

	// 2. Concatenate the generated strings
	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
		var sets [][]string
		for i, generator := range generators {
			if generator.Name == "\\b" && i > 0 && i < len(generators)-1 {
				prevGenerated := sets[len(sets)-1][0]   // Last generated string for previous pattern
				nextString := generators[i+1].Peek()[0] // Fetch a sample string from the next generator without actually generating it

				if !isWordBoundaryCompatible(lastChar(prevGenerated), firstChar(nextString)) {
					if isWordBoundaryCompatible('a', firstChar(nextString)) {
						sets[len(sets)-1][0] += "a"
					} else if isWordBoundaryCompatible(' ', firstChar(nextString)) {
						// If "a" doesn't work, try with " "
						sets[len(sets)-1][0] += " "
					}
				}
			}
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

	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
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

	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
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

func opAlternateVisibleOne(regexp *syntax.Regexp, genArgs *GeneratorArgs) (*internalGenerator, error) {
	generators, err := newGeneratorsVisibleOne(regexp.Sub, genArgs)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating generators for alternate pattern /%s/", regexp)
	}

	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
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

	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
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

	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
		return args.CaptureGroupHandler(index, regexp.Name, groupRegexp, generator, args)
	}}, nil
}

func opCaptureVisibleOne(regexp *syntax.Regexp, args *GeneratorArgs) (*internalGenerator, error) {

	if err := enforceSingleSub(regexp); err != nil {
		return nil, err
	}

	groupRegexp := regexp.Sub[0]
	generator, err := newGeneratorVisibleOne(groupRegexp, args)
	if err != nil {
		return nil, err
	}

	index := regexp.Cap - 1

	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
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
	return &internalGenerator{Name: name, GenerateFunc: func() []string {
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

	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
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

	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
		genegerated := generator.Generate()
		return []string{genegerated[rand.Intn(len(genegerated))]}
	}}, nil
}

func createRepeatingGeneratorVisibleOne(regexp *syntax.Regexp, genArgs *GeneratorArgs, min, max int) (*internalGenerator, error) {
	if err := enforceSingleSub(regexp); err != nil {
		return nil, err
	}

	generator, err := newGeneratorVisibleOne(regexp.Sub[0], genArgs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create generator for subexpression: /%s/", regexp)
	}

	if min == noBound {
		min = 0
	}

	if max == noBound {
		max = min + 1
	}

	return &internalGenerator{Name: regexp.String(), GenerateFunc: func() []string {
		genegerated := generator.Generate()
		return []string{genegerated[rand.Intn(len(genegerated))]}
	}}, nil
}
