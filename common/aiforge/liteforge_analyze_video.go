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

	type InterimData struct {
		AudioResults *AudioAnalysisResult
		ImageData    *ffmpegutils.FfmpegStreamResult
	}

	var ffmpegChan = chanx.NewUnlimitedChan[*InterimData](analyzeConfig.Ctx, 100)

	// Try to analyze audio from video
	analyzeConfig.AnalyzeStatusCard("Analysis", "start analyzing audio")
	audioResults, err := AnalyzeAudioFile(video, options...)

	// If audio analysis fails for any reason, use fallback strategy
	// This handles: no audio stream, audio extraction failure, whisper failure, etc.
	audioAnalysisFailed := false
	if err != nil {
		// Log the error and use fallback strategy instead of failing
		log.Warnf("Audio analysis failed, will use fallback strategy (scene change + fixed interval): %v", err)
		analyzeConfig.AnalyzeLog("Audio analysis failed: %v", err)
		analyzeConfig.AnalyzeLog("Fallback to scene change detection + every 3 seconds frame extraction strategy")
		audioAnalysisFailed = true
	}

	go func() {
		defer ffmpegChan.Close()
		var ffmpegOptions = make([]ffmpegutils.Option, 0)
		ffmpegOptions = append(ffmpegOptions, ffmpegutils.WithTimestampOverlay(true))
		ffmpegOptions = append(ffmpegOptions, ffmpegutils.WithIgnoreBottomPaddingInSceneDetection(true)) // 默认启用智能场景检测

		analyzeConfig.AnalyzeStatusCard("Analysis", "ffmpeg extract image")
		totalCount := 0

		if audioAnalysisFailed {
			// Fallback strategy: scene change detection + every 3 seconds frame extraction
			analyzeConfig.AnalyzeLog("No audio stream available, using fallback strategy: scene change (threshold=0.2) + every 3 seconds")
			analyzeConfig.AnalyzeStatusCard("Analysis", "fallback: scene change + 3s interval")

			// Strategy 1: Scene change detection with moderate threshold
			sceneOptions := append(ffmpegOptions, ffmpegutils.WithSceneThreshold(0.2), ffmpegutils.WithDebug(true))
			sceneResult, err := ffmpegutils.ExtractImageFramesFromVideo(video, sceneOptions...)
			if err != nil {
				log.Errorf("Failed to extract frames using scene change detection: %v", err)
			} else {
				for fResult := range sceneResult {
					if fResult.Error != nil {
						log.Warnf("Error extracting scene change frame: %v", fResult.Error)
						continue
					}
					ffmpegChan.SafeFeed(&InterimData{
						ImageData: fResult,
					})
					totalCount++
					analyzeConfig.AnalyzeStatusCard("[video]:extract frames (scene)", totalCount)
				}
				analyzeConfig.AnalyzeLog("Extracted %d frames using scene change detection", totalCount)
			}

			// Strategy 2: Fixed interval extraction (every 3 seconds)
			intervalOptions := append(ffmpegOptions, ffmpegutils.WithFramesPerSecond(1.0/3.0), ffmpegutils.WithDebug(true))
			intervalResult, err := ffmpegutils.ExtractImageFramesFromVideo(video, intervalOptions...)
			if err != nil {
				log.Errorf("Failed to extract frames using fixed interval: %v", err)
			} else {
				intervalCount := 0
				for fResult := range intervalResult {
					if fResult.Error != nil {
						log.Warnf("Error extracting interval frame: %v", fResult.Error)
						continue
					}
					ffmpegChan.SafeFeed(&InterimData{
						ImageData: fResult,
					})
					intervalCount++
					totalCount++
					analyzeConfig.AnalyzeStatusCard("[video]:extract frames (interval)", totalCount)
				}
				analyzeConfig.AnalyzeLog("Extracted %d frames using 3-second interval", intervalCount)
			}

			analyzeConfig.AnalyzeLog("Fallback strategy completed: total %d frames extracted", totalCount)
		} else {
			// Normal audio-guided frame extraction
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
					log.Errorf("Failed to extract video frames: %v", err)
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
					log.Errorf("Failed to extract video frames: %v", err)
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
		}
	}()

	extraPromptFormat := "**Supplementary Information**: %s\n cumulative summary: %s\n %s"
	frameCount := 0
	cumulativeSummary := ""
	return utils.OrderedParallelProcessSkipError(analyzeConfig.Ctx, ffmpegChan.OutputChannel(), func(data *InterimData) (AnalysisResult, error) {
		// Handle case where AudioResults might be nil (no audio stream scenario)
		var supplementaryInfo string
		if data.AudioResults != nil && data.AudioResults.TimelineSegment != nil {
			supplementaryInfo = data.AudioResults.TimelineSegment.Dump()
		} else {
			supplementaryInfo = "No audio information available (video has no audio stream)"
		}

		analyzeConfig.AnalyzeLog("Start to analyze video frame %d, Supplementary Information: %s", frameCount, supplementaryInfo)
		imageResult, err := AnalyzeImage(data.ImageData.RawData, WithExtraPrompt(fmt.Sprintf(extraPromptFormat, supplementaryInfo, cumulativeSummary, analyzeConfig.ExtraPrompt)))
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
