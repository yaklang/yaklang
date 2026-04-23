package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/yak/yakscript"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func ExecScriptWithParam(
	ctx context.Context,
	pluginName string,
	para map[string]string,
	input string, // only for codec/port-scan plugin
	handler func(result *ypb.ExecResult) error,
) error {
	return yakscript.ExecScriptWithParam(ctx, pluginName, para, input, handler)
}
