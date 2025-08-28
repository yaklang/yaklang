package aiforge

import (
	"fmt"

	"github.com/yaklang/yaklang/common/mimetype"
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
		return AnalyzeVideo(path, option...)
	} else {
		analyzeConfig.AnalyzeLog("file not video, try doc or txt: %s", path)
		return AnalyzeSingleMedia(path, option...)
	}
}
