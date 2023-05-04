package yaklib

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
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
