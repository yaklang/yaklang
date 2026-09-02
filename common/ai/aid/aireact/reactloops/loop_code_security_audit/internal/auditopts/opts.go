// Package auditopts holds shared Code Security Audit loop tuning knobs.
//
// Operational note: the dominant runtime cost in long audits is usually the AI
// upstream (TLS/EOF/timeouts). Prefer a stable gateway/model before relying on
// these code-side budgets. Defaults below reduce wasted retries and long-tail
// category scans when the channel is flaky.
package auditopts

import (
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

const (
	// DefaultCategoryScanTimeout is the wall-clock budget per Phase-2 category
	// sub-agent. Prevents a single long-tail class from holding a concurrency
	// slot for hours.
	DefaultCategoryScanTimeout = 75 * time.Minute

	// DefaultAuditTransactionRetry caps network+format transaction retries for
	// audit sub-agents (parent default is often 5).
	DefaultAuditTransactionRetry int64 = 3

	// DefaultAuditFormatRetry caps action-format retries (think-only / missing
	// @action). Lower than network retry so format failures fail fast.
	DefaultAuditFormatRetry int64 = 2
)

// SubAgentConfigOpts returns config options applied to Phase2/Phase3 forked
// sub-agent invokers.
func SubAgentConfigOpts() []aicommon.ConfigOption {
	return []aicommon.ConfigOption{
		aicommon.WithAITransactionAutoRetry(DefaultAuditTransactionRetry),
		aicommon.WithAIFormatAutoRetry(DefaultAuditFormatRetry),
		aicommon.WithSkipToolCallReasonGeneration(true),
		aicommon.WithDisablePerception(true),
		// Config-level switches propagate to every child invoker at any depth
		// (category loops, verify loops, fast_context sub-loops): the value
		// feedback gate blocks all submission paths (incl. tool review), and
		// the periodic verification entry disables the watchdog even in loops
		// built without explicit per-loop options.
		aicommon.WithDisableValueFeedbackSubmission(true),
		aicommon.WithDisablePeriodicVerification(true),
	}
}

// LoopAuxiliaryOpts disables expensive auxiliary AI paths that amplify request
// volume under slow networks (perception, periodic verification, value feedback).
func LoopAuxiliaryOpts() []reactloops.ReActLoopOption {
	return []reactloops.ReActLoopOption{
		reactloops.WithDisableLoopPerception(true),
		reactloops.WithDisablePeriodicVerification(true),
		reactloops.WithDisableValueFeedback(true),
	}
}
