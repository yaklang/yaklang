//go:build hids

package rule

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak"
)

func NewSandbox() *yak.Sandbox {
	return yak.NewSandbox(yak.WithSandbox_ExternalLib(allowedHelpers()))
}

func ValidateBooleanExpression(
	sandbox *yak.Sandbox,
	expression string,
	vars map[string]any,
) error {
	expression = strings.TrimSpace(expression)
	if sandbox == nil || expression == "" {
		return nil
	}
	_, err := sandbox.ExecuteAsBoolean(expression, vars)
	return err
}

func EvaluateBooleanExpression(
	sandbox *yak.Sandbox,
	expression string,
	vars map[string]any,
) (bool, error) {
	expression = strings.TrimSpace(expression)
	if sandbox == nil || expression == "" {
		return false, nil
	}
	return sandbox.ExecuteAsBoolean(expression, vars)
}
