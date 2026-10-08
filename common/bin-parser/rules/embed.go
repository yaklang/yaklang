package rules

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/utils/embeddedfs"
)

//go:generate go run ./internal/generate

//go:embed rules.tar.zst
var archive string

// RuleFS loads the immutable rule archive only when a rule is first requested.
// YAML remains the source of truth; regenerate the checked-in archive after edits.
var RuleFS = embeddedfs.NewZstd(archive, 32<<20)
