package match

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/suricata/rule"
)

// RuleCompileError reports every rule feature that the matcher cannot enforce.
type RuleCompileError struct {
	Diagnostics []rule.ValidationDiagnostic
}

func (e *RuleCompileError) Error() string {
	if e == nil || len(e.Diagnostics) == 0 {
		return "suricata rule compilation failed"
	}
	parts := make([]string, 0, len(e.Diagnostics))
	for _, diagnostic := range e.Diagnostics {
		parts = append(parts, fmt.Sprintf("sid=%d stage=%s keyword=%s: %s",
			diagnostic.SID, diagnostic.Stage, diagnostic.Keyword, diagnostic.Reason))
	}
	return "suricata rule compilation failed: " + strings.Join(parts, "; ")
}
