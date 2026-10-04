package aimem

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

func TestMemoryAuxiliaryCallbacksSkipPreparationAndPreserveErrors(t *testing.T) {
	for _, mode := range []string{"skip", "error"} {
		for _, operation := range []string{"extract", "tags", "deduplicate"} {
			t.Run(mode+"/"+operation, func(t *testing.T) {
				ctx := context.Background()
				invoker := mock.NewMockInvoker(ctx)
				cfg := invoker.GetConfig().(*mock.MockedAIConfig)
				cause := errors.New("test auxiliary provider error")
				scheduled := false
				cfg.ScheduleAuxiliaryTaskFunc = func(_ context.Context, _ string, _ func() string, _ func(*aicommon.Action), options ...aicommon.AuxiliaryTaskOption) {
					scheduled = true
					spec := &aicommon.AuxiliaryTaskSpec{}
					for _, option := range options {
						option(spec)
					}
					if mode == "error" {
						require.NotNil(t, spec.OnError)
						spec.OnError(cause)
					}
				}
				prepared := false
				// No DB/vector store is installed: none is needed when scheduling
				// suppresses prompt preparation. Previously those accesses ran first.
				memory := &AIMemoryTriage{
					ctx: ctx, invoker: invoker,
					contextProvider: func() (string, error) { prepared = true; return "", nil },
				}
				var err error
				switch operation {
				case "extract":
					_, err = memory.AddRawText("evidence")
				case "tags":
					_, err = memory.SelectTags(ctx, "evidence")
				case "deduplicate":
					_, err = memory.BatchIsRepeatedMemoryEntitiesByAI(ctx, []*aicommon.MemoryEntity{{Content: "evidence"}}, []int{0}, nil)
				}
				require.True(t, scheduled)
				require.False(t, prepared)
				if mode == "error" {
					require.ErrorIs(t, err, cause)
				}
			})
		}
	}
}

func TestMemoryTriageUsesTextInstructionRegardlessOfParentMode(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "function_call"}[native], func(t *testing.T) {
			invoker := mock.NewMockInvoker(context.Background())
			cfg := invoker.GetConfig().(*mock.MockedAIConfig)
			cfg.SetConfig("EnableFunctionCallMode", native)
			scheduled := false
			cfg.ScheduleAuxiliaryTaskFunc = func(_ context.Context, name string, _ func() string, _ func(*aicommon.Action), opts ...aicommon.AuxiliaryTaskOption) {
				scheduled = true
				require.Equal(t, aicommon.CallerLabelMemoryTriage, name)
				spec := &aicommon.AuxiliaryTaskSpec{}
				for _, opt := range opts {
					opt(spec)
				}
				instruction := aicommon.NewGeneralKVConfig(spec.Opts...).GetLiteForgeStaticInstruction()
				require.Contains(t, instruction, "七个评分")
				require.Equal(t, memoryTriageInstruction, instruction)
				require.NotContains(t, instruction, "原生函数 arguments 示例")
			}
			memory := &AIMemoryTriage{ctx: context.Background(), invoker: invoker}
			_, _ = memory.AddRawText("runtime input")
			require.True(t, scheduled)
		})
	}
}
