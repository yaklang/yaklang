//go:build !hids || !linux

package scannode

import (
	"context"
	"time"
)

func executeHIDSResponseAction(
	_ context.Context,
	_ string,
	process hidsResponseActionProcess,
) (hidsResponseActionExecutionResult, error) {
	return hidsResponseActionExecutionResult{
		ObservedAt: time.Now().UTC(),
		Process:    process,
	}, ErrHIDSResponseActionUnsupported
}
