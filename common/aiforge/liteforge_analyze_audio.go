package aiforge

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/mediautils"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

var AUDIO_OUTPUT_SCHEMA = promptloader.MustLoad("aiforge/liteforge_schema/liteforge_audio.schema.json")

type TimelineSegment struct {
	StartSeconds   float64 `json:"start_seconds"`
	EndSeconds     float64 `json:"end_seconds"`
	ProcessingType string  `json:"processing_type"`
	Text           string  `json:"text"`
}

func (t *TimelineSegment) String() string {
	return fmt.Sprintf("start_seconds: %f, end_seconds: %f, processing_type: %s, text: %s", t.StartSeconds, t.EndSeconds, t.ProcessingType, utils.ShrinkString(t.Text, 100))
}

func (t *TimelineSegment) Dump() string {
	return t.String()
}

func (t *TimelineSegment) Ignored() bool {
	return t.ProcessingType == "ignore"
}

func (t *TimelineSegment) FineGrained() bool {
	return t.ProcessingType == "fine"
}

type AudioProcessingStats struct {
	FineDuration   float64 `json:"fine_duration"`   // Total duration of "fine" segments in seconds
	IgnoreDuration float64 `json:"ignore_duration"` // Total duration of "ignore" segments in seconds
	FinePercentage float64 `json:"fine_percentage"`
}
type AudioAnalysisResultList []*AudioAnalysisResult

type AudioAnalysisResult struct {
	CumulativeSummary string           `json:"cumulative_summary"`
	TimelineSegment   *TimelineSegment `json:"timeline_segment"`
}

func (a AudioAnalysisResultList) GetProcessingStats() *AudioProcessingStats {
	var fineDuration float64
	var ignoreDuration float64
	for _, item := range a {
		segment := item.TimelineSegment
		if segment.FineGrained() {
			fineDuration += segment.EndSeconds - segment.StartSeconds
		} else if segment.Ignored() {
			ignoreDuration += segment.EndSeconds - segment.StartSeconds
		}
	}
	return &AudioProcessingStats{
		FineDuration:   fineDuration,
		IgnoreDuration: ignoreDuration,
		FinePercentage: fineDuration / (fineDuration + ignoreDuration),
	}
}

func AnalyzeAudioFile(audio string, opts ...any) (<-chan *AudioAnalysisResult, error) {
	if !utils.FileExists(audio) {
		return nil, fmt.Errorf("video file not found: %s", audio)
	}

	var analyzeConfig = NewAnalysisConfig(opts...)
	analyzeConfig.fallbackOptions = append(analyzeConfig.fallbackOptions, WithOutputJSONSchema(AUDIO_OUTPUT_SCHEMA))

	analyzeConfig.AnalyzeStatusCard("Analysis", "cover audio file to srt")
	analyzeConfig.AnalyzeLog("start to analyze audio file: %s", audio)
	srtPath, err := mediautils.ConvertMediaToSRT(audio)
	if err != nil {
		// Log detailed info about the failure - could be no audio stream, codec issues, etc.
		analyzeConfig.AnalyzeLog("Cannot extract audio from file: %v", err)
		analyzeConfig.AnalyzeLog("Possible reasons: no audio stream, unsupported codec, corrupted file, or ffmpeg/whisper configuration issues")
		return nil, err
	}
	analyzeConfig.AnalyzeLog("srt file generated: %s", srtPath)
	fp, err := os.Open(srtPath)
	if err != nil {
		return nil, utils.Errorf("failed to open srt: %s", err)
	}
	srtReader := utils.NewCRLFtoLFReader(fp)

	prompt := promptloader.MustLoad("inline/aiforge/liteforge_analyze_audio/prompt.txt") + analyzeConfig.ExtraPrompt

	allResult := make([]*AudioAnalysisResult, 0)
	resultChan := chanx.NewUnlimitedChan[*AudioAnalysisResult](analyzeConfig.Ctx, 100)

	cumulativeSummary := ""

	analyze := func(query string) error {
		forgeResult, err := _executeLiteForgeTemp(prompt+"\n"+query+"\n"+cumulativeSummary, analyzeConfig.ForgeExecOption(AUDIO_OUTPUT_SCHEMA)...)
		if err != nil {
			return err
		}
		if forgeResult == nil || forgeResult.Action == nil {
			return fmt.Errorf("invalid forge result")
		}
		cumulativeSummary = forgeResult.GetString("cumulative_summary")
		for _, params := range forgeResult.GetInvokeParamsArray("timeline_segments") {
			segment := &TimelineSegment{
				StartSeconds:   params.GetFloat("start_seconds"),
				EndSeconds:     params.GetFloat("end_seconds"),
				ProcessingType: params.GetString("processing_type"),
				Text:           params.GetString("text_content"),
			}
			item := &AudioAnalysisResult{
				CumulativeSummary: cumulativeSummary,
				TimelineSegment:   segment,
			}
			resultChan.SafeFeed(item)
			allResult = append(allResult, item)
		}
		return nil
	}

	processedCount := 0
	legacyData := ""

	reducerOpts := append(analyzeConfig.ReducerOptions(),
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			srtData := string(chunk.Data())
			index := strings.LastIndex(srtData, "\n\n")
			if index != -1 {
				srtData = srtData[:index]
				legacyData = srtData[index:]
			}
			overlap, ok := chunk.PrevNBytesUntil([]byte("\n\n"), 200)
			if ok {
				srtData = string(overlap) + srtData
			}
			err := analyze(srtData)
			if err != nil {
				return err
			}
			processedCount++
			analyzeConfig.AnalyzeLog("audio analysis processed chunk %d, cumulative summary length: %d, timeline segments: %d", processedCount, len(cumulativeSummary), len(allResult))
			return nil
		}),
		aireducer.WithFinishCallback(func(config *aireducer.Config, memory *aid.PromptContextProvider) error {
			if !funk.IsEmpty(legacyData) {
				err := analyze(legacyData)
				if err != nil {
					return err
				}
			}
			return nil
		}),
	)

	srtReducer, err := aireducer.NewReducerFromReader(srtReader, reducerOpts...)
	if err != nil {
		return nil, utils.Errorf("build srt reducer fail: %s", err.Error())
	}

	go func() {
		defer resultChan.Close()
		analyzeConfig.AnalyzeStatusCard("Analysis", "analyzing rst file")
		analyzeConfig.AnalyzeLog("start analyzing srt file: %s", srtPath)
		err = srtReducer.Run()
		if err != nil {
			analyzeConfig.AnalyzeLog("analyze srt file error: %s", err.Error())
			return
		}
		analyzeConfig.AnalyzeStatusCard("Analysis", "Audio finish")
		analyzeConfig.AnalyzeLog("analyzing srt file finish")
	}()
	return resultChan.OutputChannel(), nil
}
