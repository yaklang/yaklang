package whisperutils

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SRTManager manages SRT subtitle files and provides context-based operations
type SRTManager struct {
	entries []SRTEntry
}

// NewSRTManager creates a new SRT manager instance
func NewSRTManager() *SRTManager {
	return &SRTManager{
		entries: make([]SRTEntry, 0),
	}
}

// NewSRTManagerFromContent creates a new SRT manager from SRT content
func NewSRTManagerFromContent(srtContent string) (*SRTManager, error) {
	manager := NewSRTManager()
	err := manager.ParseSRTContent(srtContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SRT content: %w", err)
	}
	return manager, nil
}

// NewSRTManagerFromFile creates a new SRT manager from an SRT file
func NewSRTManagerFromFile(srtFilePath string) (*SRTManager, error) {
	content, err := os.ReadFile(srtFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SRT file %s: %w", srtFilePath, err)
	}
	return NewSRTManagerFromContent(string(content))
}

// ParseSRTContent parses SRT content and loads it into the manager
func (s *SRTManager) ParseSRTContent(content string) error {
	// Split by double newlines first, then parse each block
	blocks := strings.Split(strings.TrimSpace(content), "\n\n")
	s.entries = make([]SRTEntry, 0, len(blocks))

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		lines := strings.Split(block, "\n")
		if len(lines) < 3 {
			continue // Need at least index, timestamp, and text
		}

		// Parse index
		index, err := strconv.Atoi(strings.TrimSpace(lines[0]))
		if err != nil {
			continue
		}

		// Parse timestamp line
		timestampLine := strings.TrimSpace(lines[1])
		parts := strings.Split(timestampLine, " --> ")
		if len(parts) != 2 {
			continue
		}

		startTime, err := parseSRTTime(strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}

		endTime, err := parseSRTTime(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}

		// Parse text (remaining lines)
		textLines := lines[2:]
		text := strings.TrimSpace(strings.Join(textLines, "\n"))

		s.entries = append(s.entries, SRTEntry{
			Index:     index,
			StartTime: startTime,
			EndTime:   endTime,
			Text:      text,
		})
	}

	// Sort by start time to ensure chronological order
	sort.Slice(s.entries, func(i, j int) bool {
		return s.entries[i].StartTime < s.entries[j].StartTime
	})

	return nil
}

// GetSRTContextByOffsetSeconds returns the context around a specific time point
// offsetSeconds is the target time in seconds, intervalSeconds is the time window around the target in seconds
func (s *SRTManager) GetSRTContextByOffsetSeconds(offsetSeconds float64, intervalSeconds float64) *SRTContext {
	targetTime := time.Duration(offsetSeconds * float64(time.Second))
	interval := time.Duration(intervalSeconds * float64(time.Second))
	return s.GetSRTContextByTime(targetTime, interval)
}

// GetSRTContextByTime returns the context around a specific time point
func (s *SRTManager) GetSRTContextByTime(targetTime, interval time.Duration) *SRTContext {
	startTime := targetTime - interval
	endTime := targetTime + interval

	var contextEntries []SRTEntry
	var contextTexts []string

	for _, entry := range s.entries {
		// Check if the entry overlaps with our time window
		if entry.EndTime >= startTime && entry.StartTime <= endTime {
			contextEntries = append(contextEntries, entry)
			contextTexts = append(contextTexts, entry.Text)
		}
	}

	contextText := strings.Join(contextTexts, " ")

	return &SRTContext{
		TargetTime:     targetTime,
		Interval:       interval,
		ContextText:    contextText,
		ContextEntries: contextEntries,
	}
}

// GetEntries returns all SRT entries
func (s *SRTManager) GetEntries() []SRTEntry {
	return s.entries
}

// GetEntriesInTimeRange returns entries within a specific time range
func (s *SRTManager) GetEntriesInTimeRange(startTime, endTime time.Duration) []SRTEntry {
	var result []SRTEntry

	for _, entry := range s.entries {
		if entry.EndTime >= startTime && entry.StartTime <= endTime {
			result = append(result, entry)
		}
	}

	return result
}

// ToSRT converts the entries back to SRT format
func (s *SRTManager) ToSRT() string {
	var builder strings.Builder

	for _, entry := range s.entries {
		builder.WriteString(fmt.Sprintf("%d\n", entry.Index))
		builder.WriteString(fmt.Sprintf("%s --> %s\n",
			formatSRTTimeFromDuration(entry.StartTime),
			formatSRTTimeFromDuration(entry.EndTime)))
		builder.WriteString(entry.Text)
		builder.WriteString("\n\n")
	}

	return builder.String()
}

// RewriteWithTimestamp converts the entries to SRT format with timestamp information included in the text
func (s *SRTManager) RewriteWithTimestamp() string {
	var builder strings.Builder

	for _, entry := range s.entries {
		builder.WriteString(fmt.Sprintf("%d\n", entry.Index))
		builder.WriteString(fmt.Sprintf("%s --> %s\n",
			formatSRTTimeFromDuration(entry.StartTime),
			formatSRTTimeFromDuration(entry.EndTime)))

		// Add timestamp info to the text in the same line
		startStr := formatSRTTimeFromDuration(entry.StartTime)
		endStr := formatSRTTimeFromDuration(entry.EndTime)
		timestampInfo := fmt.Sprintf("(start: %s ---> end: %s)", startStr, endStr)

		// Combine original text with timestamp info on the same line
		if strings.TrimSpace(entry.Text) != "" {
			// Remove any existing newlines in the original text and combine with timestamp
			cleanText := strings.ReplaceAll(strings.TrimSpace(entry.Text), "\n", " ")
			builder.WriteString(fmt.Sprintf("%s %s", cleanText, timestampInfo))
		} else {
			builder.WriteString(timestampInfo)
		}
		builder.WriteString("\n\n")
	}

	return builder.String()
}

// CreateTempSRTWithTimestamp creates a temporary SRT file with timestamp information included
func (s *SRTManager) CreateTempSRTWithTimestamp() (string, error) {
	// Generate SRT content with timestamps
	srtContent := s.RewriteWithTimestamp()

	// Create temporary file
	tempFile, err := os.CreateTemp("", "srt_with_timestamp_*.srt")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary SRT file: %w", err)
	}
	defer tempFile.Close()

	// Write content to temporary file
	_, err = tempFile.WriteString(srtContent)
	if err != nil {
		// Clean up the file if writing fails
		os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to write SRT content to temporary file: %w", err)
	}

	return tempFile.Name(), nil
}

// PreviewWithTimestamp returns a preview of how the SRT content will look with timestamps
func (s *SRTManager) PreviewWithTimestamp() string {
	if len(s.entries) == 0 {
		return "No SRT entries to preview"
	}

	var preview strings.Builder
	preview.WriteString("Preview of SRT with timestamps (first 3 entries):\n")
	preview.WriteString("=" + strings.Repeat("=", 60) + "\n")

	maxEntries := len(s.entries)
	if maxEntries > 3 {
		maxEntries = 3
	}

	for i := 0; i < maxEntries; i++ {
		entry := s.entries[i]
		startStr := formatSRTTimeFromDuration(entry.StartTime)
		endStr := formatSRTTimeFromDuration(entry.EndTime)
		timestampInfo := fmt.Sprintf("(start: %s ---> end: %s)", startStr, endStr)

		if strings.TrimSpace(entry.Text) != "" {
			cleanText := strings.ReplaceAll(strings.TrimSpace(entry.Text), "\n", " ")
			preview.WriteString(fmt.Sprintf("%d. %s %s\n", entry.Index, cleanText, timestampInfo))
		} else {
			preview.WriteString(fmt.Sprintf("%d. %s\n", entry.Index, timestampInfo))
		}
	}

	if len(s.entries) > 3 {
		preview.WriteString(fmt.Sprintf("... and %d more entries\n", len(s.entries)-3))
	}

	return preview.String()
}

// UpdateEntry updates a specific SRT entry
func (s *SRTManager) UpdateEntry(index int, newText string) error {
	for i := range s.entries {
		if s.entries[i].Index == index {
			s.entries[i].Text = newText
			return nil
		}
	}
	return fmt.Errorf("entry with index %d not found", index)
}

// AddEntry adds a new SRT entry
func (s *SRTManager) AddEntry(startTime, endTime time.Duration, text string) {
	newIndex := len(s.entries) + 1
	s.entries = append(s.entries, SRTEntry{
		Index:     newIndex,
		StartTime: startTime,
		EndTime:   endTime,
		Text:      text,
	})

	// Sort by start time to maintain chronological order
	sort.Slice(s.entries, func(i, j int) bool {
		return s.entries[i].StartTime < s.entries[j].StartTime
	})

	// Re-index entries after sorting
	for i := range s.entries {
		s.entries[i].Index = i + 1
	}
}

// RemoveEntry removes an SRT entry by index
func (s *SRTManager) RemoveEntry(index int) error {
	for i, entry := range s.entries {
		if entry.Index == index {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			// Re-index remaining entries
			for j := range s.entries {
				s.entries[j].Index = j + 1
			}
			return nil
		}
	}
	return fmt.Errorf("entry with index %d not found", index)
}

// GetDuration returns the total duration of the SRT content
func (s *SRTManager) GetDuration() time.Duration {
	if len(s.entries) == 0 {
		return 0
	}
	return s.entries[len(s.entries)-1].EndTime
}

// parseSRTTime parses SRT time format (HH:MM:SS,mmm) to time.Duration
func parseSRTTime(timeStr string) (time.Duration, error) {
	// Replace comma with dot for parsing
	timeStr = strings.Replace(timeStr, ",", ".", 1)

	parts := strings.Split(timeStr, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid time format: %s", timeStr)
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid hours: %s", parts[0])
	}

	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid minutes: %s", parts[1])
	}

	seconds, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid seconds: %s", parts[2])
	}

	totalSeconds := float64(hours*3600+minutes*60) + seconds
	return time.Duration(totalSeconds * float64(time.Second)), nil
}

// formatSRTTimeFromDuration formats time.Duration to SRT time format (HH:MM:SS,mmm)
func formatSRTTimeFromDuration(d time.Duration) string {
	totalMs := d.Milliseconds()
	hours := totalMs / (1000 * 60 * 60)
	totalMs %= (1000 * 60 * 60)
	minutes := totalMs / (1000 * 60)
	totalMs %= (1000 * 60)
	seconds := totalMs / 1000
	ms := totalMs % 1000

	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, ms)
}
