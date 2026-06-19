package regen

import (
	"context"
	"fmt"
	"regexp/syntax"

	"github.com/yaklang/yaklang/common/log"

	"github.com/pkg/errors"
)

// wrapParseErr 对 "invalid repeat count" 等解析错误增加说明：精确 {n} 已支持超大重复
func wrapParseErr(pattern string, err error) error {
	if err == nil {
		return nil
	}
	if e, ok := err.(*syntax.Error); ok && e.Code == syntax.ErrInvalidRepeatSize {
		return fmt.Errorf("%w（regen 基于 Go 正则。已通过内部展开支持 {n} 形式且 n>1000 的精确重复；如果仍出现该错误，通常表示使用了当前尚未支持的大范围重复（例如 {n,m} 且上限值>1000），或模式 %q 触发了内部展开逻辑的缺陷，请尝试简化/拆分该模式并反馈问题。）", err, pattern)
	}
	return err
}

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
	GenerateStream(context.Context, chan string) error
	String() string
	CheckVisible(str string) bool
}

// GenerateStream 根据正则表达式流式生成所有匹配的字符串，返回生成的字符串通道和生成取消函数和错误
// 对于一些可能匹配多次的元字符:
// *     : 则只会生成匹配 0 次或 1 次的字符串
// +     : 则只会生成匹配 1 次或 2 次的字符串
// {n,m} : 则会生成匹配 n 次到 m 次的字符串
// {n,}  : 则只会生成匹配 n 次或 n+1 次的字符串
// 参数:
//   - pattern: 用于生成字符串的正则表达式
//   - ctxs: 可选的 context，用于提前取消生成
//
// 返回值:
//   - 流式输出生成结果的字符串通道
//   - 用于停止生成的取消函数
//   - 编译正则失败时返回的错误
//
// Example:
// ```
// ch, cancel, err = regen.GenerateStream("[a-z]+")
// for s = range ch {
// println(s)
// }
// ```
func GenerateStream(pattern string, ctxs ...context.Context) (chan string, context.CancelFunc, error) {
	generator, err := NewGenerator(pattern, &GeneratorArgs{
		Flags: syntax.Perl,
	})
	ch := make(chan string)
	if err != nil {
		return nil, nil, err
	}
	var rawCtx context.Context = context.Background()
	if len(ctxs) > 0 {
		rawCtx = ctxs[0]
	}

	ctx, cancel := context.WithCancel(rawCtx)
	go generator.GenerateStream(ctx, ch)
	return ch, cancel, nil
}

// GenerateStream 根据正则表达式流式生成一个匹配的字符串，返回生成的字符串和错误
// 对于一些可能匹配多次的元字符:
// *     : 则只会生成匹配 0 次或 1 次的字符串
// +     : 则只会生成匹配 1 次或 2 次的字符串
// {n,m} : 则会生成匹配 n 次到 m 次的字符串
// {n,}  : 则只会生成匹配 n 次或 n+1 次的字符串
// 参数:
//   - pattern: 用于生成字符串的正则表达式
//   - ctxs: 可选的 context，用于提前取消生成
//
// 返回值:
//   - 生成的一个匹配字符串
//   - 编译正则失败时返回的错误
//
// Example:
// ```
// regen.GenerateOneStream("[a-z]+") // a-z 中随机一个字母
// regen.GenerateOneStream("^(13[0-9]|14[57]|15[0-9]|18[0-9])\d{8}$") // 生成一个手机号
// ```
func GenerateOneStream(pattern string, ctxs ...context.Context) (string, error) {
	generator, err := NewGeneratorOne(pattern, &GeneratorArgs{
		Flags: syntax.Perl,
	})
	ch := make(chan string)
	if err != nil {
		return "", err
	}
	var rawCtx context.Context = context.Background()
	if len(ctxs) > 0 {
		rawCtx = ctxs[0]
	}

	ctx, cancel := context.WithCancel(rawCtx)
	go generator.GenerateStream(ctx, ch)
	defer cancel()

	return <-ch, nil
}

// GenerateVisibleOneStream 根据正则表达式流式生成一个匹配的字符串(都是可见字符)，返回生成的字符串和错误
// 对于一些可能匹配多次的元字符:
// *     : 则只会生成匹配 0 次或 1 次的字符串
// +     : 则只会生成匹配 1 次或 2 次的字符串
// {n,m} : 则会生成匹配 n 次到 m 次的字符串
// {n,}  : 则只会生成匹配 n 次或 n+1 次的字符串
// 参数:
//   - pattern: 用于生成字符串的正则表达式
//   - ctxs: 可选的 context，用于提前取消生成
//
// 返回值:
//   - 生成的一个全部为可见字符的匹配字符串
//   - 编译正则失败时返回的错误
//
// Example:
// ```
// regen.GenerateVisibleOneStream("[a-z]") // a-z 中随机一个字母
// regen.GenerateVisibleOneStream("^(13[0-9]|14[57]|15[0-9]|18[0-9])\d{8}$") // 生成一个手机号
// ```
func GenerateVisibleOneStream(pattern string, ctxs ...context.Context) (string, error) {
	generator, err := NewGeneratorVisibleOne(pattern, &GeneratorArgs{
		Flags: syntax.Perl,
	})
	ch := make(chan string)
	if err != nil {
		return "", err
	}
	var rawCtx context.Context = context.Background()
	if len(ctxs) > 0 {
		rawCtx = ctxs[0]
	}

	ctx, cancel := context.WithCancel(rawCtx)
	go generator.GenerateStream(ctx, ch)
	defer cancel()

	return <-ch, nil
}

// Generate 根据正则表达式生成所有匹配的字符串，返回生成的字符串切片和错误
// 对于一些可能匹配多次的元字符:
// *     : 则只会生成匹配 0 次或 1 次的字符串
// +     : 则只会生成匹配 1 次或 2 次的字符串
// {n,m} : 则会生成匹配 n 次到 m 次的字符串
// {n,}  : 则只会生成匹配 n 次或 n+1 次的字符串
// 参数:
//   - pattern: 用于生成字符串的正则表达式
//
// 返回值:
//   - 所有匹配字符串组成的切片
//   - 编译正则失败时返回的错误
//
// Example:
// ```
// // VARS: 字符集会枚举出全部可能
// result = regen.Generate("[ab]c")~
// // STDOUT: 打印生成结果
// println(result)   // OUT: [ac bc]
// // assert: 枚举出两个组合
// assert len(result) == 2, "Generate should enumerate ac and bc"
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
// 参数:
//   - pattern: 用于生成字符串的正则表达式
//
// 返回值:
//   - 生成的一个匹配字符串
//   - 编译正则失败时返回的错误
//
// Example:
// ```
// // VARS: 字面量模式只有唯一结果
// result = regen.GenerateOne("abc")~
// // STDOUT: 打印生成结果
// println(result)   // OUT: abc
// // assert: 锁定结论
// assert result == "abc", "literal pattern should generate itself"
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
// 参数:
//   - pattern: 用于生成字符串的正则表达式
//
// 返回值:
//   - 生成的一个全部为可见字符的匹配字符串
//   - 编译正则失败时返回的错误
//
// Example:
// ```
// // VARS: 字面量模式只有唯一结果
// result = regen.GenerateVisibleOne("abc")~
// // STDOUT: 打印生成结果
// println(result)   // OUT: abc
// // assert: 锁定结论
// assert result == "abc", "literal pattern should generate itself"
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
// 参数:
//   - pattern: 用于生成字符串的正则表达式
//
// 返回值:
//   - 所有匹配字符串组成的切片（编译失败会直接 panic）
//
// Example:
// ```
// // VARS: 字符集会枚举出全部可能
// result = regen.MustGenerate("[ab]c")
// // STDOUT: 打印生成结果
// println(result)   // OUT: [ac bc]
// // assert: 枚举出两个组合
// assert len(result) == 2, "MustGenerate should enumerate ac and bc"
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
// 参数:
//   - pattern: 用于生成字符串的正则表达式
//
// 返回值:
//   - 生成的一个匹配字符串（编译失败会直接 panic）
//
// Example:
// ```
// // VARS: 字面量模式只有唯一结果
// result = regen.MustGenerateOne("abc")
// // STDOUT: 打印生成结果
// println(result)   // OUT: abc
// // assert: 锁定结论
// assert result == "abc", "literal pattern should generate itself"
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
// 参数:
//   - pattern: 用于生成字符串的正则表达式
//
// 返回值:
//   - 生成的一个全部为可见字符的匹配字符串（编译失败会直接 panic）
//
// Example:
// ```
// // VARS: 字面量模式只有唯一结果
// result = regen.MustGenerateVisibleOne("abc")
// // STDOUT: 打印生成结果
// println(result)   // OUT: abc
// // assert: 锁定结论
// assert result == "abc", "literal pattern should generate itself"
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

	pattern = expandBigRepeat(pattern)
	var regexp *syntax.Regexp
	regexp, err = syntax.Parse(pattern, args.Flags)
	if err != nil {
		return nil, wrapParseErr(pattern, err)
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

	pattern = expandBigRepeat(pattern)
	var regexp *syntax.Regexp
	regexp, err = syntax.Parse(pattern, args.Flags)
	if err != nil {
		return nil, wrapParseErr(pattern, err)
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

	pattern = expandBigRepeat(pattern)
	var regexp *syntax.Regexp
	regexp, err = syntax.Parse(pattern, args.Flags)
	if err != nil {
		return nil, wrapParseErr(pattern, err)
	}

	var gen *internalGenerator
	gen, err = newGeneratorVisibleOne(regexp, &args)
	if err != nil {
		return
	}

	return gen, nil
}
