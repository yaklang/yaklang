package aiforge

import (
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/mediautils"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"os"
	"strings"
)

//go:embed liteforge_schema/liteforge_audio.schema.json
var AUDIO_OUTPUT_SCHEMA string

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
		return nil, err
	}
	analyzeConfig.AnalyzeLog("srt file generated: %s", srtPath)
	fp, err := os.Open(srtPath)
	if err != nil {
		return nil, utils.Errorf("failed to open srt: %s", err)
	}
	srtReader := utils.NewCRLFtoLFReader(fp)

	prompt := `# Role: Expert Iterative Content Analyst

You are an expert AI assistant designed to work within an iterative processing loop. Your specialty is analyzing sequential fragments of a transcribed video, progressively building a summary, and identifying the informational value of each time segment.

## Operational Context

You will be invoked repeatedly in a loop. In each iteration, you will receive two inputs:
1.  **"current_srt_chunk"**: A small, continuous fragment of a larger SRT transcript.
2.  **"previous_cumulative_summary"**: The summary generated from all preceding chunks. For the very first chunk, this will be an empty string.

Your task is to analyze the "current_srt_chunk" in the context of the "previous_cumulative_summary" and generate an updated JSON output.

## Task

Analyze the provided "current_srt_chunk". Classify its time segments as either "fine" (high-value) or "ignore" (low-value) based on the substance of the text. Then, generate a JSON object containing an updated cumulative summary and a timeline for **only the current chunk**.

## Rules for Classification

1.  **"fine" (重点区间):** Classify segments as "fine" if they contain:
    *   Core arguments, theses, or main points.
    *   Key conclusions or summaries.
    *   New concepts, definitions, or critical explanations.
    *   Actionable advice, steps, or instructions.
    *   Data, statistics, or strong evidence.
    *   Novel questions or profound insights.

2.  **"ignore" (忽略区间):** Classify segments as "ignore" if they contain:
    *   Filler content (e.g., "um," "ah," "you know," "so," "well").
    *   Greetings, introductions, and closing pleasantries.
    *   Redundant phrases or self-corrections.
    *   Simple transitional sentences (e.g., "Now, let's move on to...", "And another thing is...").
    *   Off-topic remarks or personal anecdotes that don't support the main point.

## Output Format Requirements

1.  **"cumulative_summary" (string):**
    *   This is an **updated** summary.
    *   Synthesize the key information from the "fine" segments of the "srt_chunk" and **integrate it** with the provided "cumulative_summary".
    *   The result should be a single, coherent, and progressively refined summary. Avoid simple concatenation; aim for a true synthesis that merges new insights with existing knowledge without becoming redundant.

2.  **"timeline_segments" (array of objects):**
    *   This array represents **only the segments from the "srt_chunk"**.
    *   **Crucially, the "start_seconds" and "end_seconds" for each segment MUST directly correspond to the literal timestamps found in the provided "srt_chunk". Do not re-normalize, re-index, or start the timeline from 0.0.**
    *   The segments within this chunk's timeline should be continuous and cover the entire duration of the chunk (the "end_seconds" of one segment must equal the "start_seconds" of the next).

` + analyzeConfig.ExtraPrompt

	allResult := make([]*AudioAnalysisResult, 0)
	resultChan := chanx.NewUnlimitedChan[*AudioAnalysisResult](analyzeConfig.Ctx, 100)

	cumulativeSummary := ""

	analyze := func(query string) error {
		forgeResult, err := _executeLiteForgeTemp(prompt+"\n"+query+"\n"+cumulativeSummary, analyzeConfig.fallbackOptions...)
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
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.Memory, chunk chunkmaker.Chunk) error {
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
		aireducer.WithFinishCallback(func(config *aireducer.Config, memory *aid.Memory) error {
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
