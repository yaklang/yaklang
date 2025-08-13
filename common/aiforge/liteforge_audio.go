package aiforge

import (
	"bytes"
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mediautils"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/whisperutils"
)

//go:embed liteforge_audio.schema.json
var AUDIO_OUTPUT_SCHEMA string

type TimelineSegment struct {
	StartSeconds   int64  `json:"start_seconds"`
	EndSeconds     int64  `json:"end_seconds"`
	ProcessingType string `json:"processing_type"`
}

func (t *TimelineSegment) Ignored() bool {
	return t.ProcessingType == "ignore"
}

func (t *TimelineSegment) FineGrained() bool {
	return t.ProcessingType == "fine"
}

type AudioAnalysisResult struct {
	CumulativeSummary string             `json:"cumulative_summary"`
	TimelineSegments  []*TimelineSegment `json:"timeline_segments"`
}

func AnalyzeAudioFile(video string, opts ...any) (*AudioAnalysisResult, error) {
	if !utils.FileExists(video) {
		return nil, fmt.Errorf("video file not found: %s", video)
	}

	log.Infof("covert audio[%s] to srt string", video)
	srtContent, err := mediautils.ConvertMediaToSRTString(video)
	if err != nil {
		return nil, err
	}
	log.Infof("srt content size: %d", len(srtContent))
	srtManager, err := whisperutils.NewSRTManagerFromContent(srtContent)
	if err != nil {
		return nil, err
	}

	var config = &imageAnalysisConfig{}
	for _, opt := range opts {
		if optFunc, ok := opt.(imageAnalysisOption); ok {
			optFunc(config)
		} else {
			config.fallbackOptions = append(config.fallbackOptions, opt)
		}
	}
	config.fallbackOptions = append(config.fallbackOptions, _withOutputJSONSchema(AUDIO_OUTPUT_SCHEMA))

	prompt := `# Role: Expert Content Analyst

You are an expert AI assistant specializing in analyzing transcribed video content to identify its core message and structure. Your task is to process the text from an SRT file, segment it based on information density, and generate a structured JSON output.

## Task

Analyze the provided SRT transcript content. Based on the substance and informational value of the text, classify time segments as either "fine" (high-value, key information) or "ignore" (low-value, filler content). Then, generate a JSON object that includes a summary and a complete timeline of these segments.

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

Your output **MUST** be a single, valid JSON object. Do not add any explanatory text before or after the JSON.

The JSON object must contain exactly two top-level keys: "cumulative_summary" and "timeline_segments".

1.  **"cumulative_summary" (string):**
    *   A concise, high-level summary.
    *   This summary must be distilled **exclusively** from the text content of the segments you marked as "fine".

2.  **"timeline_segments" (array of objects):**
    *   This must be a seamless and continuous timeline from "0.0" seconds to the end of the last subtitle.
    *   The "end_seconds" of one segment **must** equal the "start_seconds" of the next segment.
    *   The first segment **must** start at "start_seconds: 0.0".
    *   Each object in the array must contain three keys:
        *   "start_seconds" (number): The start time of the segment.
        *   "end_seconds" (number): The end time of the segment.
        *   "processing_type" (string): The value can **ONLY** be ""fine"" or ""ignore"".

**IMPORTANT:** **DO NOT** include the "processing_stats" object in your output. I will calculate those statistics myself.


` + config.ExtraPrompt

	basicRSTCache := bytes.NewBuffer(make([]byte, 0))
	cacheCount := 0
	var result = &AudioAnalysisResult{}
	for _, entry := range srtManager.GetEntries() {
		if cacheCount < 10 {
			basicRSTCache.WriteString(entry.String() + "\n")
			cacheCount++
			continue
		}
		log.Infof("current cache count: %d, processing %d entries", cacheCount, len(srtManager.GetEntries()))
		log.Infof("analyzing %d entries in srt: %s", cacheCount, utils.ShrinkString(basicRSTCache.String(), 200))
		forgeResult, err := _executeLiteForgeTemp(prompt+"\n"+basicRSTCache.String()+"\n"+result.CumulativeSummary, config.fallbackOptions...)
		if err != nil {
			return nil, err
		}
		if forgeResult == nil || forgeResult.Action == nil {
			return nil, fmt.Errorf("invalid forge result")
		}
		result.CumulativeSummary = forgeResult.GetString("cumulative_summary")
		for _, params := range forgeResult.GetInvokeParamsArray("timeline_segments") {
			if result.TimelineSegments == nil {
				result.TimelineSegments = make([]*TimelineSegment, 0)
			}
			result.TimelineSegments = append(result.TimelineSegments, &TimelineSegment{
				StartSeconds:   params.GetInt("start_seconds"),
				EndSeconds:     params.GetInt("end_seconds"),
				ProcessingType: params.GetString("processing_type"),
			})
		}
		basicRSTCache.Reset()
		cacheCount = 0
	}

	return result, nil
}
