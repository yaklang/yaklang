// Package c is the default language layer. Every native artifact has a
// language; when no compiler metadata identifies another one, the artifact is
// treated as C.
//
// It adds no ABI and no string rules. The calling convention of C and of
// hand-written assembly is the platform convention from the architecture
// profile, so this layer only states that no compiler overrides exist.
package c

import (
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// C is the fallback language layer. Detection always succeeds with the lowest
// confidence, so a report always exists.
type C struct{}

func (C) Kind() core.Language { return core.LanguageC }

func (C) Detect(*core.Artifact) *core.LanguageReport {
	return &core.LanguageReport{
		Language:   core.LanguageC,
		Confidence: core.ConfidenceNone,
	}
}

// Recover reports no names: C symbols come from the container's own tables.
func (C) Recover(*core.Artifact) ([]symbol.Symbol, error) { return nil, nil }

// Strings reports a NUL-terminated byte string reached through a pointer.
func (C) Strings() core.Strings {
	return core.Strings{Layout: core.StringLayoutNulTerminated, Encoding: core.StringEncodingBytes}
}
