package aiforge

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/utils/ffmpegutils"
)

type VideoAnalysisResult struct {
	CumulativeSummary string               `json:"cumulative_summary"`
	ImageSegment      *ImageAnalysisResult `json:"segments"`
}

func (v VideoAnalysisResult) GetCumulativeSummary() string {
	return v.CumulativeSummary
}

func (v VideoAnalysisResult) Dump() string {
	return v.ImageSegment.Dump()
}

func AnalyzeVideo(video string, options ...any) (<-chan AnalysisResult, error) {
	analyzeConfig := NewAnalysisConfig(options...)

	analyzeConfig.AnalyzeStatusCard("Analysis", "start analyzing audio")
	audioResults, err := AnalyzeAudioFile(video, options...)
	if err != nil {
		return nil, err
	}

	type InterimData struct {
		AudioResults *AudioAnalysisResult
		ImageData    *ffmpegutils.FfmpegStreamResult
	}

	var ffmpegChan = chanx.NewUnlimitedChan[*InterimData](analyzeConfig.Ctx, 100)
	go func() {
		defer ffmpegChan.Close()
		var ffmpegOptions = make([]ffmpegutils.Option, 0)
		ffmpegOptions = append(ffmpegOptions, ffmpegutils.WithTimestampOverlay(true))
		ffmpegOptions = append(ffmpegOptions, ffmpegutils.WithIgnoreBottomPaddingInSceneDetection(true)) // 默认启用智能场景检测

		analyzeConfig.AnalyzeStatusCard("Analysis", "ffmpeg extract image")
		totalCount := 0
		ffmpegOptions = append(ffmpegOptions, ffmpegutils.WithSceneThreshold(0.05))
		for audioRes := range audioResults {
			segment := audioRes.TimelineSegment
			analyzeConfig.AnalyzeLog("Audio segments found in video, use fine grained to analyze video frames")

			if segment.Ignored() {
				analyzeConfig.AnalyzeLog("Skip ignored segment: %s", segment.String())
				continue
			}

			analyzeConfig.AnalyzeLog("start extracting video frames for fine segment: %s", segment.String())
			subOptions := append(ffmpegOptions, ffmpegutils.WithStartEndSeconds(segment.StartSeconds, segment.EndSeconds), ffmpegutils.WithDebug(true))
			ffmpegResult, err := ffmpegutils.ExtractImageFramesFromVideo(video, subOptions...)
			if err != nil {
				log.Errorf("Failed to extract audio frames: %v", err)
				return
			}
			count := 0
			for fResult := range ffmpegResult {
				ffmpegChan.SafeFeed(&InterimData{
					AudioResults: audioRes,
					ImageData:    fResult,
				})
				count++
				totalCount++
				analyzeConfig.AnalyzeStatusCard("[video]:extract frames", totalCount)
			}
			analyzeConfig.AnalyzeLog("Extracted %d video frames for segment: %s", count, segment.String())
		}

		if totalCount == 0 {
			analyzeConfig.AnalyzeLog("No audio segments found in video, use broad grained to analyze video frames")
			ffmpegOptions = append(ffmpegOptions, ffmpegutils.WithSceneThreshold(0.2))
			ffmpegResult, err := ffmpegutils.ExtractImageFramesFromVideo(video, ffmpegOptions...)
			if err != nil {
				log.Errorf("Failed to extract audio frames: %v", err)
				return
			}
			for fResult := range ffmpegResult {
				ffmpegChan.SafeFeed(&InterimData{
					ImageData: fResult,
				})
				totalCount++
				analyzeConfig.AnalyzeStatusCard("[video]:extract frames", totalCount)
			}
		}
	}()

	extraPromptFormat := "**Supplementary Information**: %s\n cumulative summary: %s\n %s"
	frameCount := 0
	cumulativeSummary := ""
	return utils.OrderedParallelProcessSkipError(analyzeConfig.Ctx, ffmpegChan.OutputChannel(), func(data *InterimData) (AnalysisResult, error) {
		analyzeConfig.AnalyzeLog("Start to analyze video frame %d, Supplementary Information: %s", frameCount, data.AudioResults.TimelineSegment.Dump())
		imageResult, err := AnalyzeImage(data.ImageData.RawData, WithExtraPrompt(fmt.Sprintf(extraPromptFormat, data.AudioResults.TimelineSegment.Dump(), cumulativeSummary, analyzeConfig.ExtraPrompt)))
		if err != nil {
			return nil, err
		}

		cumulativeSummary = imageResult.CumulativeSummary

		analyzeConfig.AnalyzeLog("Finish to analyze video frame %d, current CumulativeSummary is [%s] ", frameCount, utils.ShrinkString(cumulativeSummary, 100))
		frameCount++
		analyzeConfig.AnalyzeStatusCard("[video]:analysed frames", frameCount)
		return imageResult, nil
	},
		utils.WithParallelProcessConcurrency(analyzeConfig.AnalyzeConcurrency), utils.WithParallelProcessFinishCallback(func() {
			analyzeConfig.AnalyzeLog("Finish to analyze video frame %d", frameCount)
			analyzeConfig.AnalyzeStatusCard("Analysis", "Video finish")
		}),
		utils.WithParallelProcessStartCallback(func() {
			analyzeConfig.AnalyzeLog("Finish to analyze video frames; total %d", frameCount)
			analyzeConfig.AnalyzeStatusCard("Analysis", "analyzing video frame")
		}),
	), nil
}
