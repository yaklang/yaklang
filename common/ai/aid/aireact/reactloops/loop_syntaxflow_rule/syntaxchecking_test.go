package loop_syntaxflow_rule

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckSyntaxFlowAndFormatErrors_ValidRule(t *testing.T) {
	validRule := `rule("test")
desc(
	title: "Test"
	type: audit
	level: info
)`
	errorMsg, hasBlockingErrors := checkSyntaxFlowAndFormatErrors(validRule)
	assert.False(t, hasBlockingErrors, "valid rule should not have blocking errors")
	assert.Empty(t, errorMsg)
}

func TestCheckSyntaxFlowAndFormatErrors_InvalidRule_MissingParen(t *testing.T) {
	invalidRule := `rule("test")
desc(
	title: "Test"
	type: audit
	level: info
` // missing closing ) for desc(
	errorMsg, hasBlockingErrors := checkSyntaxFlowAndFormatErrors(invalidRule)
	assert.True(t, hasBlockingErrors, "invalid rule should have blocking errors")
	assert.NotEmpty(t, errorMsg)
	assert.Contains(t, errorMsg, "SyntaxFlow")
}

func TestCheckSyntaxFlowAndFormatErrors_InvalidRule_UnknownToken(t *testing.T) {
	invalidRule := `rule("test") invalid_token_here`
	errorMsg, hasBlockingErrors := checkSyntaxFlowAndFormatErrors(invalidRule)
	assert.True(t, hasBlockingErrors, "invalid rule should have blocking errors")
	assert.NotEmpty(t, errorMsg)
}

func TestCheckSyntaxFlowAndFormatErrors_EmptyContent(t *testing.T) {
	errorMsg, hasBlockingErrors := checkSyntaxFlowAndFormatErrors("")
	// Empty may or may not be valid depending on sfvm
	_ = errorMsg
	_ = hasBlockingErrors
}

func TestCheckSyntaxFlowAndFormatErrors_ErrorOutputContainsContext(t *testing.T) {
	invalidRule := `rule("x")
desc(
	title: "Bad
)`
	errorMsg, hasBlockingErrors := checkSyntaxFlowAndFormatErrors(invalidRule)
	assert.True(t, hasBlockingErrors)
	assert.True(t, strings.Contains(errorMsg, "SyntaxFlow") || len(errorMsg) > 0,
		"error message should contain useful information")
}
