package aicommon

import (
	"time"
)

const (
	TimelineCompressionMaxInputTokens   = 512 * 1024
	TimelineCompressionMaxSummaryTokens = 16 * 1024
	timelineCompressionRetryDelay       = time.Minute
	timelineCompressionTargetDivisor    = 2
)
