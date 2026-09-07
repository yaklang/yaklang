package sfpattern

import (
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// ExecRule runs a SyntaxFlow rule against source files only (no SSA compile).
// The rule should be mode=source and use ${...}.re / .pattern_regex statements.
func ExecRule(rule *schema.SyntaxFlowRule, files map[string]string, opts ...sfvm.Option) (*sfvm.SFFrameResult, error) {
	if rule == nil {
		return nil, utils.Error("sfpattern: nil rule")
	}
	content := strings.TrimSpace(rule.Content)
	if content == "" {
		return nil, utils.Error("sfpattern: empty rule content")
	}
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(content)
	if err != nil {
		return nil, err
	}
	return ExecFrame(frame, files, opts...)
}

// ExecFrame feeds a compiled frame with a PatternRoot over files.
func ExecFrame(frame *sfvm.SFFrame, files map[string]string, opts ...sfvm.Option) (*sfvm.SFFrameResult, error) {
	if frame == nil {
		return nil, utils.Error("sfpattern: nil frame")
	}
	root := NewRoot(files)
	return frame.Feed(sfvm.ValuesOf(root), opts...)
}

// ExecRuleOnFS loads fs then ExecRule.
func ExecRuleOnFS(rule *schema.SyntaxFlowRule, fs fi.FileSystem, opts ...sfvm.Option) (*sfvm.SFFrameResult, error) {
	files, err := LoadFilesFromFS(fs)
	if err != nil {
		return nil, err
	}
	return ExecRule(rule, files, opts...)
}

// ExecFrameOnFS loads fs then ExecFrame.
func ExecFrameOnFS(frame *sfvm.SFFrame, fs fi.FileSystem, opts ...sfvm.Option) (*sfvm.SFFrameResult, error) {
	files, err := LoadFilesFromFS(fs)
	if err != nil {
		return nil, err
	}
	return ExecFrame(frame, files, opts...)
}

// AlertCount returns total values in the alert symbol table.
func AlertCount(result *sfvm.SFFrameResult) int {
	if result == nil || result.AlertSymbolTable == nil {
		return 0
	}
	n := 0
	result.AlertSymbolTable.ForEach(func(_ string, vals sfvm.Values) bool {
		n += sfvm.ValuesLen(vals)
		return true
	})
	return n
}

// HasAlert reports whether any alert variable is non-empty.
func HasAlert(result *sfvm.SFFrameResult) bool {
	return AlertCount(result) > 0
}
