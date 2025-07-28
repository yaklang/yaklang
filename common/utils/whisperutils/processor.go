package whisperutils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// TranscriptionProcessor wraps the response and provides useful methods.
type TranscriptionProcessor struct {
	Response WhisperResponse
}

// NewProcessor creates a new processor instance from raw JSON data.
func NewProcessor(jsonData []byte) (*TranscriptionProcessor, error) {
	var resp WhisperResponse
	// Use a Decoder to be stricter about unknown fields if needed
	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.DisallowUnknownFields() // Good practice for robust parsing

	if err := decoder.Decode(&resp); err != nil {
		// If it fails, retry without disallowing unknown fields, for flexibility
		log.Warnf("Strict JSON parsing failed: %v. Retrying with flexible parsing...", err)
		var respFlexible WhisperResponse
		if err := json.Unmarshal(jsonData, &respFlexible); err != nil {
			return nil, fmt.Errorf("failed to unmarshal json data: %w", err)
		}
		return &TranscriptionProcessor{Response: respFlexible}, nil
	}

	return &TranscriptionProcessor{Response: resp}, nil
}

// ToSRT converts the transcription into the SubRip (.srt) subtitle format.
func (p *TranscriptionProcessor) ToSRT() string {
	var b strings.Builder

	for i, segment := range p.Response.Segments {
		b.WriteString(fmt.Sprintf("%d\n", i+1))

		startTime := formatSRTTime(segment.Start)
		endTime := formatSRTTime(segment.End)
		b.WriteString(fmt.Sprintf("%s --> %s\n", startTime, endTime))

		b.WriteString(strings.TrimSpace(segment.Text))
		b.WriteString("\n\n")
	}

	return b.String()
}

// aggregateWordsByInterval groups all transcribed words by a specified time interval.
// This is the core helper for creating block-based subtitles.
func (p *TranscriptionProcessor) aggregateWordsByInterval(intervalSeconds int) map[int]string {
	if intervalSeconds <= 0 {
		intervalSeconds = 1 // Default to 1 second if invalid to avoid division by zero
	}
	// Use a map where the key is the start of the interval (e.g., 0, 30, 60)
	// and the value is a list of words.
	wordsByInterval := make(map[int][]string)

	// Iterate through all segments and all words within them.
	for _, segment := range p.Response.Segments {
		for _, word := range segment.Words {
			// group by the interval start time
			intervalStart := int(word.Start) / intervalSeconds * intervalSeconds
			// whisper.cpp output includes leading spaces, so we trim them.
			wordsByInterval[intervalStart] = append(wordsByInterval[intervalStart], strings.TrimSpace(word.Word))
		}
	}

	// Now, join the words for each interval into a single string.
	result := make(map[int]string)
	for intervalStart, words := range wordsByInterval {
		result[intervalStart] = strings.Join(words, " ")
	}

	return result
}

// ToSRTTeleprompter generates a three-line, scrolling SRT format
// with labels for the previous, current, and next block of text.
// This creates a professional teleprompter-style effect.
// aggregationSeconds defines the time window in seconds for each text block.
func (p *TranscriptionProcessor) ToSRTTeleprompter(aggregationSeconds int) string {
	if aggregationSeconds <= 0 {
		aggregationSeconds = 30 // Default to 30 seconds if an invalid value is provided.
	}
	// First, get the text aggregated by the specified interval.
	agg := p.aggregateWordsByInterval(aggregationSeconds)

	if len(agg) == 0 {
		return ""
	}

	var b strings.Builder

	// We need to process the intervals in chronological order.
	var sortedIntervalStarts []int
	for k := range agg {
		sortedIntervalStarts = append(sortedIntervalStarts, k)
	}
	sort.Ints(sortedIntervalStarts)

	// Determine the full range of intervals to generate subtitles for,
	// from the first spoken word to the last, including silent intervals.
	if len(sortedIntervalStarts) == 0 {
		return ""
	}
	firstIntervalStart := sortedIntervalStarts[0]
	lastIntervalStart := sortedIntervalStarts[len(sortedIntervalStarts)-1]

	srtIndex := 1
	// Iterate through each interval period from the start to the end.
	for currentIntervalStart := firstIntervalStart; currentIntervalStart <= lastIntervalStart; currentIntervalStart += aggregationSeconds {
		// Line 1: Text from the previous interval.
		prevText := agg[currentIntervalStart-aggregationSeconds]
		line1 := fmt.Sprintf("%10s: %s", "[PREV]", prevText)

		// Line 2: Text from the current interval.
		currentText := agg[currentIntervalStart]
		line2 := fmt.Sprintf("%10s: %s", "[CURRENT]", currentText)

		// Line 3: Text from the next interval.
		nextText := agg[currentIntervalStart+aggregationSeconds]
		line3 := fmt.Sprintf("%10s: %s", "[NEXT]", nextText)

		// 1. Subtitle Number
		b.WriteString(fmt.Sprintf("%d\n", srtIndex))
		srtIndex++

		// 2. Timestamp: Each entry lasts for the duration of the aggregation window.
		startTime := formatSRTTime(float64(currentIntervalStart))
		endTime := formatSRTTime(float64(currentIntervalStart + aggregationSeconds))
		b.WriteString(fmt.Sprintf("%s --> %s\n", startTime, endTime))

		// 3. Subtitle Text (three lines with labels)
		b.WriteString(fmt.Sprintf("%s\n%s\n%s", line1, line2, line3))
		b.WriteString("\n\n")
	}

	return b.String()
}

func formatSRTTime(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	ms := (d - s*time.Second) / time.Millisecond

	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}
