package aireact

import (
	"context"
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_yaklangcode"
)

func (r *ReAct) invokeWriteYaklangCode(ctx context.Context, approach string) (string, error) {
	loop, err := reactloops.CreateLoopByName(loop_yaklangcode.LOOP_NAME_WRITE_YAKLANG_CODE, r)
	if err != nil {
		return "", err
	}
	err = loop.Execute(ctx, approach)
	if err != nil {
		return "", err
	}
	return loop.Get("filename"), nil
}
