package regen

import (
	"regexp/syntax"

	"github.com/yaklang/yaklang/common/log"

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

// Generate 根据正则表达式生成所有匹配的字符串，返回生成的字符串切片和错误
// 对于一些可能匹配多次的元字符:
// *     : 则只会生成匹配 0 次或 1 次的字符串
// +     : 则只会生成匹配 1 次或 2 次的字符串
// {n,m} : 则会生成匹配 n 次到 m 次的字符串
// {n,}  : 则只会生成匹配 n 次或 n+1 次的字符串
// Example:
// ```
// regen.Generate("[a-z]+") // a-z 单个字母，aa-zz 两个字母
// ```
func Generate(pattern string) ([]string, error) {
	generator, err := NewGenerator(pattern, &GeneratorArgs{
		Flags: syntax.Perl,
	})
	if err != nil {
		return []string{""}, err
	}
	return generator.Generate(), nil
}

// GenerateOne 根据正则表达式生成一个匹配的字符串，返回生成的字符串和错误
// Example:
// ```
// regen.GenerateOne("[a-z]") // a-z 中随机一个字母
// regen.GenerateOne("^(13[0-9]|14[57]|15[0-9]|18[0-9])\d{8}$") // 生成一个手机号
// ```
func GenerateOne(pattern string) (string, error) {
	generator, err := NewGeneratorOne(pattern, &GeneratorArgs{
		Flags: syntax.Perl,
	})
	if err != nil {
		return "", err
	}
	return generator.Generate()[0], nil
}

// GenerateVisibleOne 根据正则表达式生成一个匹配的字符串(都是可见字符)，返回生成的字符串和错误
// Example:
// ```
// regen.GenerateVisibleOne("[a-z]") // a-z 中随机一个字母
// regen.GenerateVisibleOne("^(13[0-9]|14[57]|15[0-9]|18[0-9])\d{8}$") // 生成一个手机号
// ```
func GenerateVisibleOne(pattern string) (string, error) {
	generator, err := NewGeneratorVisibleOne(pattern, &GeneratorArgs{
		Flags: syntax.Perl,
	})
	if err != nil {
		return "", err
	}
	generated := generator.Generate()[0]
	if len(generated) > 0 {
		if !generator.CheckVisible(generated) {
			log.Warnf("pattern %s,res [%s] is not visible one", pattern, generated)
		}
	}
	return generated, nil
}

// MustGenerate 根据正则表达式生成所有匹配的字符串，如果生成失败则会崩溃，返回生成的字符串切片
// 对于一些可能匹配多次的元字符:
// *     : 则只会生成匹配 0 次或 1 次的字符串
// +     : 则只会生成匹配 1 次或 2 次的字符串
// {n,m} : 则会生成匹配 n 次到 m 次的字符串
// {n,}  : 则只会生成匹配 n 次或 n+1 次的字符串
// Example:
// ```
// regen.MustGenerate("[a-z]+") // a-z 单个字母，aa-zz 两个字母
// ```
func MustGenerate(pattern string) []string {
	generator, err := NewGenerator(pattern, &GeneratorArgs{
		Flags: syntax.Perl,
	})
	if err != nil {
		panic(err)
	}
	return generator.Generate()
}

// MustGenerateOne 根据正则表达式生成一个匹配的字符串，如果生成失败则会崩溃，返回生成的字符串
// Example:
// ```
// regen.MustGenerateOne("[a-z]") // a-z 中随机一个字母
// regen.MustGenerateOne("^(13[0-9]|14[57]|15[0-9]|18[0-9])\d{8}$") // 生成一个手机号
// ```
func MustGenerateOne(pattern string) string {
	generator, err := NewGeneratorOne(pattern, &GeneratorArgs{
		Flags: syntax.Perl,
	})
	if err != nil {
		panic(err)
	}
	return generator.Generate()[0]
}

// MustGenerateVisibleOne 根据正则表达式生成一个匹配的字符串(都是可见字符)，如果生成失败则会崩溃，返回生成的字符串
// Example:
// ```
// regen.MustGenerateVisibleOne("[a-z]") // a-z 中随机一个字母
// regen.MustGenerateVisibleOne("^(13[0-9]|14[57]|15[0-9]|18[0-9])\d{8}$") // 生成一个手机号
// ```
func MustGenerateVisibleOne(pattern string) string {
	generator, err := NewGeneratorVisibleOne(pattern, &GeneratorArgs{
		Flags: syntax.Perl,
	})
	if err != nil {
		panic(err)
	}
	generated := generator.Generate()[0]
	if len(generated) > 0 {
		if !generator.CheckVisible(generated) {
			log.Warnf("pattern %s,res [%s] is not visible one", pattern, generated)
		}
	}
	return generated
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
