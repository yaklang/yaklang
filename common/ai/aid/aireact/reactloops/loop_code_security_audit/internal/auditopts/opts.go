// Package auditopts holds shared Code Security Audit loop tuning knobs.
//
// Operational note: the dominant runtime cost in long audits is usually the AI
// upstream (TLS/EOF/timeouts). Prefer a stable gateway/model before relying on
// these code-side budgets. The knobs here are audit-layer only: wall-clock
// budgets via the sub-agent job timeout parameter and loop-level auxiliary
// cuts via pre-existing reactloops options — no config-layer switches.
package auditopts

import (
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

const (
	// DefaultCategoryScanTimeout is the wall-clock budget per Phase-2 category
	// sub-agent. Prevents a single long-tail class from holding a concurrency
	// slot for hours.
	DefaultCategoryScanTimeout = 75 * time.Minute
)

// LoopAuxiliaryOpts disables expensive auxiliary AI paths that amplify request
// volume under slow networks (perception, periodic verification). Both are
// pre-existing loop-level options applied to the audit's own loops only.
func LoopAuxiliaryOpts() []reactloops.ReActLoopOption {
	return []reactloops.ReActLoopOption{
		reactloops.WithDisableLoopPerception(true),
		reactloops.WithDisablePeriodicVerification(true),
	}
}
