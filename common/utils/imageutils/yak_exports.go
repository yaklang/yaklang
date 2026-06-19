package imageutils

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mimetype"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"os"
)

// ExtractImage 从多种输入（io.Reader、字节、字符串）中提取内嵌的图片
// 适用于从原始数据流中扫描并还原图片，返回一个图片结果的通道
// 参数:
//   - i: 输入数据，可为 io.Reader、字节数组或字符串
//
// 返回值:
//   - 图片提取结果通道，每个元素为一张提取出的图片
//
// Example:
// ```
// // 示意性示例，需要提供包含图片的真实数据
// raw = file.ReadFile("/tmp/with-images.bin")~
//
//	for result in imageutils.ExtractImage(raw) {
//	    println(result.MIMEType)
//	}
//
// ```
func ExtractImage(i any) chan *ImageResult {
	switch ret := i.(type) {
	case io.Reader:
		bytes, _ := io.ReadAll(ret)
		return ExtractWildStringImage(bytes)
	default:
		return ExtractWildStringImage(codec.AnyToBytes(ret))
	}
}

// ExtractImageFromFile 从文件路径提取图片，支持视频抽帧、PDF 与图片文档等
// 内部会先检测文件 MIME 类型，再分别按视频、图片或文档分页方式提取
// 参数:
//   - filePath: 输入文件路径
//   - options: 可选项，如 imageutils.context 设置上下文
//
// 返回值:
//   - 图片提取结果通道
//   - 错误信息
//
// Example:
// ```
// // 示意性示例，需要提供真实的视频/PDF/图片文件
// ch, err = imageutils.ExtractImageFromFile("/tmp/demo.pdf")
// if err != nil { die(err) }
//
//	for result in ch {
//	    println(result.MIMEType)
//	}
//
// ```
func ExtractImageFromFile(filePath string, options ...ImageExtractorOption) (chan *ImageResult, error) {
	config := &ImageExtractorConfig{
		ctx: context.Background(),
	}
	for _, option := range options {
		if option != nil {
			option(config)
		}
	}

	mt, mtErr := mimetype.DetectFile(filePath)
	if mtErr != nil {
		return nil, utils.Errorf("cannot fetch mimetype for file %s: %v", filePath, mtErr)
	}

	var result chan *ImageResult
	var err error

	if mt.IsVideo() {
		result, err = ExtractVideoFrameContext(config.ctx, filePath)
		if err != nil {
			return nil, utils.Errorf("cannot extract video frames for file %s: %v", filePath, err)
		}
		return result, nil
	} else if mt.IsImage() {
		fp, err := os.OpenFile(filePath, os.O_RDONLY, os.ModePerm)
		if err != nil {
			return nil, utils.Errorf("cannot open file %s: %v", filePath, err)
		}
		var ch = make(chan *ImageResult)
		go func() {
			defer close(ch)
			defer fp.Close()
			count := 0
			for i := range ExtractImage(fp) {
				if i == nil {
					continue
				}
				count++
				ch <- i
			}
			if count <= 0 {
				log.Errorf("no images extracted from file %s", filePath)
			}
		}()
		return ch, nil
	} else {
		result, err = ExtractDocumentPagesContext(config.ctx, filePath)
		if err != nil {
			return nil, utils.Errorf("cannot extract document pages for file %s: %v", filePath, err)
		}
		return result, nil
	}
}

type ImageExtractorConfig struct {
	ctx context.Context
}

type ImageExtractorOption func(*ImageExtractorConfig)

// WithCtx 为图片提取设置上下文，用于控制超时与取消（导出名为 imageutils.context）
// 参数:
//   - ctx: 上下文对象
//
// 返回值:
//   - 图片提取可选项
//
// Example:
// ```
// ctx, cancel = context.WithTimeout(context.Background(), 10 * time.Second)
// defer cancel()
// ch, err = imageutils.ExtractImageFromFile("/tmp/demo.mp4", imageutils.context(ctx))
// if err != nil { die(err) }
// ```
func WithCtx(ctx context.Context) ImageExtractorOption {
	return func(o *ImageExtractorConfig) {
		o.ctx = ctx
	}
}

var Exports = map[string]any{
	"ExtractImage":         ExtractImage,
	"ExtractImageFromFile": ExtractImageFromFile,

	"context": WithCtx,
}
