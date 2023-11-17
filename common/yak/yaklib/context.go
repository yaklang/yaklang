package yaklib

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
)

func _seconds(d float64) context.Context {
	return utils.TimeoutContextSeconds(d)
}

func _withTimeoutSeconds(d float64) context.Context {
	return utils.TimeoutContextSeconds(d)
}

var ContextExports = map[string]interface{}{
	"Seconds":            _seconds,
	"New":                context.Background,
	"Background":         context.Background,
	"WithCancel":         context.WithCancel,
	"WithTimeout":        context.WithTimeout,
	"WithTimeoutSeconds": _withTimeoutSeconds,
	"WithDeadline":       context.WithDeadline,
	"WithValue":          context.WithValue,
}
