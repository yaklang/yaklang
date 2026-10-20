package reactloops

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/schema"
)

func TestUIStructuredFieldKeepsJSONBytes(t *testing.T) {
	var output strings.Builder
	emitter := aicommon.NewEmitter("ui-test", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		if event.IsStream {
			output.Write(event.StreamDelta)
			output.Write(event.Content)
		}
		return event, nil
	})
	source := `{"id":"block","nodes":[{"id":"text","props":{"text":"引号\"及换行\n"}}]}`
	emitted, err := emitNativeActionField(emitter, "task", &LoopStreamField{
		FieldName: "ui", AINodeId: "ui_component", ContentType: interaction.ComponentContentType, RawJSON: true,
	}, strings.NewReader(source))
	require.NoError(t, err)
	require.True(t, emitted)
	require.Equal(t, source, output.String())
}
