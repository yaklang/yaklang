package yakscript

import (
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ExecScriptWithParam runs a profile DB plugin by name with KV params, streaming [ypb.ExecResult] to handler.
// Does not use gRPC [Server]; suitable for ssa_compile and other library callers.
func ExecScriptWithParam(
	ctx context.Context,
	pluginName string,
	para map[string]string,
	input string, // only for codec/port-scan plugin
	handler func(result *ypb.ExecResult) error,
) error {
	script, err := yakit.GetYakScriptByName(consts.GetGormProfileDatabase(), pluginName)
	if err != nil {
		return err
	}
	params := make([]*ypb.KVPair, 0, len(para))
	for name, value := range para {
		params = append(params, &ypb.KVPair{
			Key:   name,
			Value: value,
		})
	}
	fakeStream := NewFakeStream(ctx, handler)
	runtimeId := uuid.New().String()
	return ExecScriptWithExecParam(script, input, fakeStream, params, runtimeId, consts.GetGormProjectDatabase())
}
