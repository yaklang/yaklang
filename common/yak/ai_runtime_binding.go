package yak

import (
	"context"
	"io"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func aiToolOutputWriters(stdout, stderr io.Writer) (io.Writer, io.Writer) {
	mu := new(sync.Mutex)
	return &aiToolOutputWriter{mu: mu, writer: stdout}, &aiToolOutputWriter{mu: mu, writer: stderr}
}

func aiToolRiskSaveHandler(runtime *aitool.ToolRuntimeConfig) func(context.Context, *schema.Risk) error {
	if runtime == nil {
		return nil
	}
	if runtime.RiskSaveHandler != nil {
		return runtime.RiskSaveHandler
	}
	if runtime.ProjectDatabase == nil {
		return nil
	}
	db := runtime.ProjectDatabase
	return func(ctx context.Context, risk *schema.Risk) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return yakit.SaveRiskWithDatabase(db, risk)
	}
}

func bindAIToolPluginRuntime(caller *YakToCallerManager, ctx context.Context, runtime *aitool.ToolRuntimeConfig, feedback func(*ypb.ExecResult) error) <-chan error {
	caller.SetCtx(ctx)
	caller.aiRuntimeConfig = runtime
	caller.aiRuntimeClient = yaklib.NewVirtualYakitClientWithRuntimeID(feedback, caller.runtimeId)
	failures := make(chan error, 1)
	if save := aiToolRiskSaveHandler(runtime); save != nil {
		caller.aiRuntimeRiskSaveHandler = func(ctx context.Context, risk *schema.Risk) error {
			err := save(ctx, risk)
			if err != nil {
				select {
				case failures <- err:
				default:
				}
			}
			return err
		}
	}
	return failures
}
