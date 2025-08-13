package whisperutils

import (
	"fmt"
	"strings"
	"time"
)

// SRTEntry represents a single SRT subtitle entry
type SRTEntry struct {
	Index     int           `json:"index"`
	StartTime time.Duration `json:"start_time"`
	EndTime   time.Duration `json:"end_time"`
	Text      string        `json:"text"`
}

// SRTContext represents the context around a specific time point
type SRTContext struct {
	TargetTime     time.Duration `json:"target_time"`
	Interval       time.Duration `json:"interval"`
	ContextText    string        `json:"context_text"`
	ContextEntries []SRTEntry    `json:"context_entries"`
}

func (s *SRTContext) String() string {
	if len(s.ContextEntries) == 0 {
		return ""
	}

	var lines []string
	for _, entry := range s.ContextEntries {
		lines = append(lines, entry.String())
	}

	return strings.Join(lines, "\n")
}

func (s *SRTEntry) String() string {
	startSeconds := s.StartTime.Seconds()
	endSeconds := s.EndTime.Seconds()
	return fmt.Sprintf("[%.2f --> %.2f]: %s", startSeconds, endSeconds, s.Text)
}
