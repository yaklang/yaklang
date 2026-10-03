package coordinator

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func TestCoordinatorActionDirectlyAnswerKeepsCoordinating(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "native"}[native], func(t *testing.T) {
			f := newActionFixture(t, true)
			reactloops.WithFunctionCallMode(native)(f.loop)
			op := f.invoke("directly_answer", map[string]any{"answer_payload": "正在核对 source.1"}, false)
			require.True(t, op.IsContinued())
			done, err := op.IsTerminated()
			require.NoError(t, err)
			require.False(t, done)
			require.False(t, f.c.Snapshot().Finished)
		})
	}
}
