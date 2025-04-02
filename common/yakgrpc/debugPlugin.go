package yakgrpc

import (
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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
	// para to param
	params := make([]*ypb.KVPair, 0, len(para))
	for name, value := range para {
		params = append(params, &ypb.KVPair{
			Key:   name,
			Value: value,
		})
	}
	fakeStream := NewFakeStream(ctx, handler)
	runtimeId := uuid.New().String()
	return execScriptWithExecParam(script, input, fakeStream, params, runtimeId, consts.GetGormProjectDatabase())
}
