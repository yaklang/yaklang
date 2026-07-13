package pandocutils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// SimpleConvertMarkdownFileToDocxContext 将 Markdown 文件转换为 docx 文件（带上下文）
// 依赖本地安装的 pandoc 工具，转换结果输出到 Yakit 临时目录
// 参数:
//   - ctx: 上下文对象，用于控制取消与超时
//   - md: 输入的 Markdown 文件路径
//
// 返回值:
//   - 生成的 docx 文件路径
//   - 错误信息
//
// Example:
// ```
// // 示意性示例，需要本地安装 pandoc 并提供真实 markdown 文件
// ctx = context.Background()
// result, err = pandoc.SimpleConvertMarkdownFileToDocxContext(ctx, "filename.md")
// if err != nil { die(err) }
// println(result)
// ```
func _simpleConvertMarkdownFileToDocxContext(ctx context.Context, md string) (string, error) {
	filename := fmt.Sprintf("md2docx-%v.docx", ksuid.New().String())
	dirname := filepath.Join(consts.GetDefaultYakitBaseTempDir(), "pandoc-output")
	os.MkdirAll(dirname, os.ModePerm)
	pathName := filepath.Join(dirname, filename)
	err := SimpleCovertMarkdownToDocx(ctx, md, pathName)
	if err != nil {
		return "", fmt.Errorf("failed to convert markdown to docx: %w", err)
	}
	if !utils.FileExists(pathName) {
		return "", utils.Errorf("output file does not exist after conversion: %s", pathName)
	}
	return pathName, nil
}

// SimpleConvertMarkdownFileToDocx 将 Markdown 文件转换为 docx 文件
// 依赖本地安装的 pandoc 工具，使用默认上下文，转换结果输出到 Yakit 临时目录
// 参数:
//   - md: 输入的 Markdown 文件路径
//
// 返回值:
//   - 生成的 docx 文件路径
//   - 错误信息
//
// Example:
// ```
// // 示意性示例，需要本地安装 pandoc 并提供真实 markdown 文件
// outputFile, err = pandoc.SimpleConvertMarkdownFileToDocx("filename.md")
// if err != nil { die(err) }
// println(outputFile)
// ```
func _simpleConvertMarkdownFileToDocx(md string) (string, error) {
	return _simpleConvertMarkdownFileToDocxContext(context.Background(), md)
}

var deprecatedWarning = new(sync.Once)

// simpleCoverMD2Word 将 Markdown 文件转换为 Word(.docx) 文件（导出名为 pandoc.SimpleCoverMD2Word）
// 依赖底层 pandoc 程序完成转换
//
// Deprecated: 这是 alpha 阶段接口，建议改用 pandoc.SimpleConvertMarkdownFileToDocxContext 或
// pandoc.SimpleConvertMarkdownFileToDocx 以获得更好的体验
//
// 参数:
//   - ctx: 上下文，用于控制转换过程的取消与超时
//   - inputFile: 输入的 Markdown 文件路径
//   - outputFile: 输出的 Word(.docx) 文件路径
//
// 返回值:
//   - 错误信息（pandoc 不可用或转换失败时返回）
//
// Example:
// ```
// // 该示例依赖底层 pandoc 程序，仅作用法示意
// dir = os.TempDir()
// md = file.Join(dir, "demo.md")
// out = file.Join(dir, "demo.docx")
// file.Save(md, "# Title\n\nhello pandoc")~
// err = pandoc.SimpleCoverMD2Word(context.Background(), md, out)
// if err != nil { log.error("convert failed: %v", err) }
// ```
func simpleCoverMD2Word(ctx context.Context, inputFile string, outputFile string) error {
	deprecatedWarning.Do(func() {
		log.Warn("pandoc.SimpleCoverMD2Word is an alpha pandoc api, please use pandoc.SimpleConvertMarkdownToDocxContext or SimpleConvertMarkdownTo instead for best experience.")
	})
	return SimpleCovertMarkdownToDocx(ctx, inputFile, outputFile)
}

var Exports = map[string]any{
	"SimpleConvertMarkdownFileToDocxContext": _simpleConvertMarkdownFileToDocxContext,
	"SimpleConvertMarkdownFileToDocx":        _simpleConvertMarkdownFileToDocx,
	"SimpleCoverMD2Word":                     simpleCoverMD2Word,
}
