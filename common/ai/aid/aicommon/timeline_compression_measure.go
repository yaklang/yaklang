package aicommon

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
)

type timelinePromptUsage struct {
	Tokens        int64
	Limit         int64
	ThroughID     int64
	FreezeVersion int64
	Fingerprint   string
}

// Capture rendered material and its policy/watermark together; tokenize after
// unlocking. This is the same projection used by prompt assembly, not a
// compression candidate snapshot.
func (m *Timeline) measureTimelinePrompt(options TimelinePromptOptions) (usage timelinePromptUsage, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("render timeline compression measurement: %v", recovered)
		}
	}()
	text := func() string {
		m.mu.RLock()
		defer m.mu.RUnlock()
		usage.Limit = m.totalDumpContentLimit
		usage.ThroughID = m.getMaxIDLocked()
		if m.freezeState != nil {
			usage.FreezeVersion = m.freezeState.Version
		}
		if usage.Limit <= 0 {
			return ""
		}
		return m.renderTimelinePromptLocked(options).PromptText()
	}()
	usage.Tokens = int64(MeasureTokens(text))
	usage.Fingerprint = fmt.Sprintf("%x/%d", sha256.Sum256([]byte(text)), usage.Limit)
	return usage, nil
}

func (m *Timeline) compressionCoolingDown(usage timelinePromptUsage) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return usage.Fingerprint == m.compressionLastFailure || time.Now().Before(m.compressionRetryAfter)
}

// Automatic retry policy belongs to the scheduler, never to a single round.
func (m *Timeline) recordCompressionOutcome(options TimelineCompressionOptions, initial timelinePromptUsage, runErr error) {
	if errors.Is(runErr, errTimelineCompressionLimitChanged) {
		return // a new configuration should be checked immediately
	}
	fingerprint := ""
	var retryAfter time.Time
	if runErr != nil {
		current, err := m.measureTimelinePrompt(options.PromptOptions)
		if err != nil {
			current = initial
		}
		fingerprint = current.Fingerprint
		retryAfter = time.Now().Add(timelineCompressionRetryDelay)
	}
	m.mu.Lock()
	m.compressionLastFailure, m.compressionRetryAfter = fingerprint, retryAfter
	m.mu.Unlock()
}

// captureCompressionBudgetBlocksLocked renders the main-model projection while
// the snapshot lock is held. Token counts are computed by the caller after
// releasing the lock.
func (m *Timeline) captureCompressionBudgetBlocksLocked(options TimelinePromptOptions) (blocks TimelineFrozenOpenBlocks, limit int64, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("render timeline compression budget: %v", recovered)
		}
	}()
	if m == nil {
		return TimelineFrozenOpenBlocks{}, 0, fmt.Errorf("timeline compression budget requires a timeline")
	}
	return m.renderTimelinePromptLocked(options), m.totalDumpContentLimit, nil
}

func newCompressionPromptBudget(blocks TimelineFrozenOpenBlocks, limit, target int64) timelineCompressionPromptBudget {
	named := []struct{ name, text string }{
		{"frozen_timeline", blocks.Frozen},
		{"skill_content", blocks.SkillsContext},
		{"recent_tools", blocks.PromotedRecentTools},
		{"user_input", blocks.PromotedUserInputHistory},
		{"evidence", blocks.EvidenceSemiDynamic},
		{"open_timeline", blocks.Open},
	}
	budget := timelineCompressionPromptBudget{
		Current: int64(MeasureTokens(blocks.PromptText())),
		Target:  target,
		Limit:   limit,
	}
	for _, section := range named {
		if strings.TrimSpace(section.text) == "" {
			continue
		}
		budget.Sections = append(budget.Sections, timelineCompressionSectionUsage{
			Name: section.name, Tokens: int64(MeasureTokens(section.text)),
		})
	}
	return budget
}
