package yaklib

import (
	"context"
	"yaklang.io/yaklang/common/utils"
)

var ContextExports = map[string]interface{}{
	"Seconds":      utils.TimeoutContextSeconds,
	"New":          context.Background,
	"Background":   context.Background,
	"WithCancel":   context.WithCancel,
	"WithTimeout":  context.WithTimeout,
	"WithDeadline": context.WithDeadline,
	"WithValue":    context.WithValue,
}
