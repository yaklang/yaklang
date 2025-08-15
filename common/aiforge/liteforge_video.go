package aiforge

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/utils/ffmpegutils"
)

type VideoAnalysisResult struct {
	CumulativeSummary string                 `json:"cumulative_summary"`
	AudioResult       *AudioAnalysisResult   `json:"audio_result,omitempty"`
	ImageSegments     []*ImageAnalysisResult `json:"segments"`
}

func AnalyzeVideo(video string, options ...any) (*VideoAnalysisResult, error) {
	analyzeConfig := NewAnalysisConfig(options...)

	analyzeConfig.AnalyzeStatusCard("Video Analysis", "analyzing audio")
	audioResult, err := AnalyzeAudioFile(video, options...)
	if err != nil {
		return nil, err
	}

	analyzeConfig.AnalyzeLog("Audio analysis completed, found %d timeline segments, cumulative summary: %s", len(audioResult.TimelineSegments), utils.ShrinkString(audioResult.CumulativeSummary, 100))

	var ffmpegOptions = make([]ffmpegutils.Option, 0)
	ffmpegOptions = append(ffmpegOptions, ffmpegutils.WithTimestampOverlay(true))
	ffmpegOptions = append(ffmpegOptions, ffmpegutils.WithIgnoreBottomPaddingInSceneDetection(true)) // 默认启用智能场景检测

	analyzeConfig.AnalyzeStatusCard("Video Analysis", "ffmpeg extract image")
	var videoAnalysisChan = chanx.NewUnlimitedChan[chunkmaker.Chunk](analyzeConfig.Ctx, 100)
	if len(audioResult.TimelineSegments) <= 0 {
		analyzeConfig.AnalyzeLog("No audio segments found in video, use broad grained to analyze video frames")
		ffmpegOptions = append(ffmpegOptions, ffmpegutils.WithSceneThreshold(0.2))
		ffmpegResult, err := ffmpegutils.ExtractImageFramesFromVideo(video, ffmpegOptions...)
		if err != nil {
			return nil, err
		}
		go func() {
			defer videoAnalysisChan.Close()
			count := 0
			for fResult := range ffmpegResult {
				count++
				analyzeConfig.AnalyzeStatusCard("extract frames", count)
				data := fResult.RawData
				mimeObj := fResult.MIMETypeObj
				videoAnalysisChan.SafeFeed(chunkmaker.NewBufferChunkEx(data, mimeObj, ""))

			}
		}()
	} else {
		analyzeConfig.AnalyzeLog("Audio segments found in video, use fine grained to analyze video frames")
		ffmpegOptions = append(ffmpegOptions, ffmpegutils.WithSceneThreshold(0.05))
		go func() {
			defer videoAnalysisChan.Close()
			totalCount := 0
			for _, segment := range audioResult.TimelineSegments {
				if segment.Ignored() {
					analyzeConfig.AnalyzeLog("Skip ignored segment: %s", segment.String())
					continue
				}

				analyzeConfig.AnalyzeLog("start extracting video frames for fine segment: %s", segment.String())
				subOptions := append(ffmpegOptions, ffmpegutils.WithStartEndSeconds(segment.StartSeconds, segment.EndSeconds), ffmpegutils.WithDebug(true))
				ffmpegResult, err := ffmpegutils.ExtractImageFramesFromVideo(video, subOptions...)
				if err != nil {
					log.Errorf("Failed to extract audio frames: %v", err)
				}
				count := 0
				for fResult := range ffmpegResult {
					count++
					totalCount++
					analyzeConfig.AnalyzeStatusCard("extract frames", totalCount)
					data := fResult.RawData
					mimeObj := fResult.MIMETypeObj
					verbose := segment.String()
					videoAnalysisChan.SafeFeed(chunkmaker.NewBufferChunkEx(data, mimeObj, verbose))
				}
				analyzeConfig.AnalyzeLog("Extracted %d video frames for segment: %s", count, segment.String())
			}
		}()
	}

	cm, err := chunkmaker.NewSimpleChunkMaker(videoAnalysisChan, chunkmaker.WithCtx(analyzeConfig.Ctx))
	if err != nil {
		return nil, err
	}

	var videoAnalysisResult = &VideoAnalysisResult{
		AudioResult:       audioResult,
		ImageSegments:     make([]*ImageAnalysisResult, 0),
		CumulativeSummary: "",
	}
	frameCount := 0
	extraPromptFormat := "**Supplementary Information**: %s\n cumulative summary: %s\n %s"
	ar, err := aireducer.NewReducerEx(cm, aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.Memory, chunk chunkmaker.Chunk) error {
		analyzeConfig.AnalyzeLog("Start to analyze video frame %d, Supplementary Information: %s", frameCount, chunk.VerboseMessage())
		imageResult, err := AnalyzeImage(chunk.Data(), WithExtraPrompt(fmt.Sprintf(extraPromptFormat, chunk.VerboseMessage(), videoAnalysisResult.CumulativeSummary, analyzeConfig.ExtraPrompt)))
		if err != nil {
			return err
		}
		videoAnalysisResult.ImageSegments = append(videoAnalysisResult.ImageSegments, imageResult)
		if analyzeConfig.AnalyzeStreamChunkCallback != nil {
			analyzeConfig.AnalyzeStreamChunkCallback(chunkmaker.NewBufferChunk([]byte(imageResult.Dump())))
		}
		videoAnalysisResult.CumulativeSummary = imageResult.CumulativeSummary
		analyzeConfig.AnalyzeLog("Finish to analyze video frame %d, current CumulativeSummary is [%s] ", frameCount, utils.ShrinkString(videoAnalysisResult.CumulativeSummary, 100))
		frameCount++
		analyzeConfig.AnalyzeStatusCard("processed frames", frameCount)
		return nil
	}))
	if err != nil {
		return nil, err
	}
	analyzeConfig.AnalyzeLog("Finish to analyze video frames; total %d", frameCount)
	analyzeConfig.AnalyzeStatusCard("Video Analysis", "analyzing video frame")
	err = ar.Run()
	if err != nil {
		return nil, err
	}
	analyzeConfig.AnalyzeStatusCard("Video Analysis", "finish")
	return videoAnalysisResult, nil
}
