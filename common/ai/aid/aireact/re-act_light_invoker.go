package aireact

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// BuildLightReActInvoker creates a lightweight ReAct invoker for background/trigger-only usage.
// Compared with BuildReActInvoker, it avoids creating a nested AIMemory instance and does not
// start the event loop.
func BuildLightReActInvoker(ctx context.Context, options ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
	cfg := aicommon.NewConfig(ctx, options...)
	dirname := consts.TempAIDir(cfg.GetRuntimeId())
	if existed, _ := utils.PathExists(dirname); !existed {
		return nil, utils.Errorf("temp ai dir %s not existed", dirname)
	}

	invoker := &ReAct{
		config:               cfg,
		Emitter:              cfg.Emitter,
		taskQueue:            NewTaskQueue("react-light-queue"),
		mirrorOfAIInputEvent: make(map[string]func(*ypb.AIInputEvent)),
		saveTimelineThrottle: utils.NewThrottleEx(3, true, true),
		artifacts:            filesys.NewRelLocalFs(dirname),
		wg:                   new(sync.WaitGroup),
		pureInvokerMode:      true,
	}

	if !utils.IsNil(cfg.MemoryTriage) {
		invoker.memoryTriage = cfg.MemoryTriage
		invoker.config.MemoryTriage = cfg.MemoryTriage
		invoker.memoryTriage.SetInvoker(invoker)
	}

	cfg.EnhanceKnowledgeManager.SetEmitter(cfg.Emitter)

	workdir := cfg.Workdir
	if workdir == "" {
		if wd, _ := invoker.artifacts.Getwd(); wd != "" {
			workdir = wd
		}
		if workdir == "" {
			workdir = filepath.Join(consts.GetDefaultBaseHomeDir(), "code")
			if utils.GetFirstExistedFile(workdir) == "" {
				_ = os.MkdirAll(workdir, os.ModePerm)
			}
		}
	}
	invoker.promptManager = NewPromptManager(invoker, workdir)
	invoker.promptManager.cpm = cfg.ContextProviderManager

	if wd, err := invoker.artifacts.Getwd(); err == nil && wd != "" {
		invoker.Emitter.EmitPinDirectory(wd)
	}

	return invoker, nil
}

func init() {
	aicommon.RegisterLightAIRuntimeInvoker(BuildLightReActInvoker)
}
