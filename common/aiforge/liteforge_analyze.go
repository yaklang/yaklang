package aiforge

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"

	"github.com/yaklang/yaklang/common/mimetype"
)

func Analyze(input any, option ...any) (<-chan AnalysisResult, error) {
	if input == nil {
		return nil, fmt.Errorf("input is nil")
	}

	switch v := input.(type) {
	case string:
		if utils.FileExists(v) {
			return AnalyzeFile(v, option...)
		} else {
			return AnalyzeReader(strings.NewReader(v), option...)
		}
	case []byte:
		return AnalyzeReader(bytes.NewReader(v), option...)
	case io.Reader:
		return AnalyzeReader(v, option...)
	case *os.File:
		return AnalyzeReader(v, option...)
	default:
		return nil, fmt.Errorf("unknown type %T", v)
	}
}

func AnalyzeFile(path string, option ...any) (<-chan AnalysisResult, error) {
	analyzeConfig := NewAnalysisConfig(option...)
	analyzeConfig.AnalyzeLog("analyze file: %s", path)

	mime, err := mimetype.DetectFile(path)
	if err != nil {
		return nil, fmt.Errorf("connot detect mime type '%s': %w", path, err)
	}

	if mime.IsVideo() {
		analyzeConfig.AnalyzeLog("file is video: %s", path)
		return AnalyzeVideo(path, option...)
	} else {
		analyzeConfig.AnalyzeLog("file not video, try doc or txt: %s", path)
		return AnalyzeSingleMedia(path, option...)
	}
}

func AnalyzeReader(rawReader io.Reader, opts ...any) (<-chan AnalysisResult, error) {
	analyzeConfig := NewAnalysisConfig(opts...)
	analyzeConfig.AnalyzeStatusCard("Analysis", "make chunks from raw data")
	chunkOption := []chunkmaker.Option{chunkmaker.WithCtx(analyzeConfig.Ctx)}
	chunkOption = append(chunkOption, analyzeConfig.chunkOption...)

	cm, err := chunkmaker.NewTextChunkMaker(rawReader, chunkmaker.WithCtx(analyzeConfig.Ctx))
	if err != nil {
		return nil, err
	}

	indexedChannel := chanx.NewUnlimitedChan[chunkmaker.Chunk](analyzeConfig.Ctx, 100)
	count := 0
	ar, err := aireducer.NewReducerEx(cm,
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			analyzeConfig.AnalyzeLog("chunk index[%d] size:%v ", count, utils.ByteSize(uint64(chunk.BytesSize())))
			indexedChannel.SafeFeed(chunk)
			count++
			analyzeConfig.AnalyzeStatusCard("[raw]:extract chunk", count)
			return nil
		}),
		aireducer.WithContext(analyzeConfig.Ctx),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to analyze raw data: %w", err)
	}

	go func() {
		defer indexedChannel.Close()
		err = ar.Run()
		if err != nil {
			log.Errorf("failed to run analyze raw data: %v", err)
		}
	}()

	processedCount := 0

	return utils.OrderedParallelProcessSkipError[chunkmaker.Chunk, AnalysisResult](analyzeConfig.Ctx, indexedChannel.OutputChannel(), func(chunk chunkmaker.Chunk) (AnalysisResult, error) {
		defer func() {
			processedCount++
			analyzeConfig.AnalyzeStatusCard("[analyze raw data]:analysed chunk ", processedCount)
		}()
		if chunk.MIMEType().IsImage() {
			return AnalyzeImage(chunk.Data(), opts...)
		} else {
			return &TextAnalysisResult{Text: string(chunk.Data())}, nil
		}
	},
		utils.WithParallelProcessConcurrency(analyzeConfig.AnalyzeConcurrency),
		utils.WithParallelProcessStartCallback(func() {
			analyzeConfig.AnalyzeStatusCard("Analysis", "processing raw chunk")
		}),
		utils.WithParallelProcessFinishCallback(func() {
			analyzeConfig.AnalyzeStatusCard("Analysis", "finished preliminary analysis")
		})), nil
}
