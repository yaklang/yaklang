package regen

import (
	"github.com/yaklang/yaklang/common/log"
	"regexp/syntax"

	"github.com/pkg/errors"
)

type CaptureGroupHandler func(index int, name string, group *syntax.Regexp, generator Generator, args *GeneratorArgs) []string

type GeneratorArgs struct {
	Flags               syntax.Flags
	CaptureGroupHandler CaptureGroupHandler
}

func (a *GeneratorArgs) initialize() error {
	if (a.Flags&syntax.UnicodeGroups) == syntax.UnicodeGroups && (a.Flags&syntax.Perl) != syntax.Perl {
		return errors.New("UnicodeGroups not supported")
	}

	if a.CaptureGroupHandler == nil {
		a.CaptureGroupHandler = defaultCaptureGroupHandler
	}

	return nil
}

type Generator interface {
	Generate() []string
	String() string
	CheckVisible(str string) bool
}

func Generate(pattern string) ([]string, error) {
	generator, err := NewGenerator(pattern, &GeneratorArgs{
		Flags: syntax.Perl,
	})
	if err != nil {
		return []string{""}, err
	}
	return generator.Generate(), nil
}

func GenerateOne(pattern string) ([]string, error) {
	generator, err := NewGeneratorOne(pattern, &GeneratorArgs{
		Flags: syntax.Perl,
	})
	if err != nil {
		return []string{""}, err
	}
	return generator.Generate(), nil
}

func GenerateVisibleOne(pattern string) ([]string, error) {
	generator, err := NewGeneratorVisibleOne(pattern, &GeneratorArgs{
		Flags: syntax.Perl,
	})
	if err != nil {
		return []string{""}, err
	}
	generated := generator.Generate()
	if len(generated) > 0 {
		if !generator.CheckVisible(generated[0]) {
			log.Warnf("pattern %s,res [%s] is not visible one", pattern, generated[0])
		}
	}
	return generated, nil
}

func MustGenerate(pattern string) []string {
	generator, err := NewGenerator(pattern, &GeneratorArgs{
		Flags: syntax.Perl,
	})
	if err != nil {
		panic(err)
	}
	return generator.Generate()
}

func NewGenerator(pattern string, inputArgs *GeneratorArgs) (generator Generator, err error) {
	args := GeneratorArgs{}

	// Copy inputArgs so the caller can't change them.
	if inputArgs != nil {
		args = *inputArgs
	}
	if err = args.initialize(); err != nil {
		return nil, err
	}

	var regexp *syntax.Regexp
	regexp, err = syntax.Parse(pattern, args.Flags)
	if err != nil {
		return
	}

	var gen *internalGenerator
	gen, err = newGenerator(regexp, &args)
	if err != nil {
		return
	}

	return gen, nil
}

func NewGeneratorOne(pattern string, inputArgs *GeneratorArgs) (geneator Generator, err error) {
	args := GeneratorArgs{}

	// Copy inputArgs so the caller can't change them.
	if inputArgs != nil {
		args = *inputArgs
	}
	if err = args.initialize(); err != nil {
		return nil, err
	}

	var regexp *syntax.Regexp
	regexp, err = syntax.Parse(pattern, args.Flags)
	if err != nil {
		return
	}

	var gen *internalGenerator
	gen, err = newGeneratorOne(regexp, &args)
	if err != nil {
		return
	}

	return gen, nil
}

func NewGeneratorVisibleOne(pattern string, inputArgs *GeneratorArgs) (geneator Generator, err error) {
	args := GeneratorArgs{}

	// Copy inputArgs so the caller can't change them.
	if inputArgs != nil {
		args = *inputArgs
	}
	if err = args.initialize(); err != nil {
		return nil, err
	}

	var regexp *syntax.Regexp
	regexp, err = syntax.Parse(pattern, args.Flags)
	if err != nil {
		return
	}

	var gen *internalGenerator
	gen, err = newGeneratorVisibleOne(regexp, &args)
	if err != nil {
		return
	}

	return gen, nil
}
