package reactloops

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

func TestReactiveDataBuilderFailureDoesNotStopPromptBuild(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true))
	loop := makeSchemaStabilityTestLoop(cfg)
	loop.invoker = &promptCompareInvoker{MockInvoker: mock.NewMockInvoker(context.Background())}
	loop.reactiveDataBuilder = func(*ReActLoop, *bytes.Buffer, string) (string, error) {
		return "", errors.New("reactive data unavailable")
	}

	_, err := loop.generateLoopPrompt("reactive-data-failure", "input", "", nil, "", &LoopActionHandlerOperator{})
	require.NoError(t, err)
}
