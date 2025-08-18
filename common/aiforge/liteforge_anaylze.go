package aiforge

import (
	"fmt"
	"github.com/yaklang/yaklang/common/mimetype"
	"github.com/yaklang/yaklang/common/utils/pipeline"
)

func AnalyzeFile(path string, option ...any) (<-chan AnalysisResult, error) {
	analyzeConfig := NewAnalysisConfig(option...)
	analyzeConfig.AnalyzeLog("analyze file: %s", path)

	mime, err := mimetype.DetectFile(path)
	if err != nil {
		return nil, fmt.Errorf("connot detect mime type '%s': %w", path, err)
	}

	analyzeConfig.AnalyzeStatusCard("Auto Analysis file type", mime.String())
	if mime.IsVideo() {
		analyzeConfig.AnalyzeLog("file is video: %s", path)
		videoResult, err := AnalyzeVideo(path, option...)
		if err != nil {
			return nil, fmt.Errorf("failed to analyze video file '%s': %w", path, err)
		}
		pl := pipeline.NewSimplePipe(analyzeConfig.Ctx, videoResult, func(item *VideoAnalysisResult) (AnalysisResult, error) {
			analyzeConfig.AnalyzeLog("video analysis result: %s", item.Dump())
			return item, nil
		})
		return pl.Out(), nil
	} else {
		analyzeConfig.AnalyzeLog("file not video: %s", path)
		return AnalyzeSingleMedia(path, option...)
	}
}
