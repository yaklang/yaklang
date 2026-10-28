//go:build hids

package rule

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/yak"
)

func NewSandbox() *yak.Sandbox {
	return yak.NewSandbox(yak.WithSandbox_ExternalLib(allowedHelpers()), yak.WithSandbox_IsolatedExecution())
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
	return EvaluateBooleanExpressionContext(context.Background(), sandbox, expression, vars)
}

func EvaluateBooleanExpressionContext(
	ctx context.Context,
	sandbox *yak.Sandbox,
	expression string,
	vars map[string]any,
) (bool, error) {
	expression = strings.TrimSpace(expression)
	if sandbox == nil || expression == "" {
		return false, nil
	}
	return sandbox.ExecuteAsBooleanContext(ctx, expression, vars)
}
