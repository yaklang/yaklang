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
