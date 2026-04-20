package loop_intent

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

func recommendedCapabilitiesStreamHandler(fieldReader io.Reader, emitWriter io.Writer) {
	content, err := io.ReadAll(fieldReader)
	if err != nil {
		return
	}
	display := formatRecommendedCapabilitiesDisplay(string(content))
	if strings.TrimSpace(display) == "" {
		return
	}
	_, _ = io.WriteString(emitWriter, display)
}

func formatRecommendedCapabilitiesDisplay(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if unquoted, ok := tryUnquoteRecommendedCapabilitiesString(raw); ok {
		raw = strings.TrimSpace(unquoted)
	}
	if raw == "" || raw == "[]" {
		return ""
	}

	if !strings.HasPrefix(raw, "[") {
		return raw
	}

	items := reactloops.NormalizeCapabilityNames(raw)
	if len(items) == 0 {
		return ""
	}

	var builder strings.Builder
	for i, item := range items {
		if i > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(fmt.Sprintf("%d. %s", i+1, item))
	}
	return builder.String()
}

func tryUnquoteRecommendedCapabilitiesString(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	unquoted, err := strconv.Unquote(raw)
	if err != nil {
		return "", false
	}
	return unquoted, true
}

func recommendedCapabilitiesStreamCallback(invoker aicommon.AIInvokeRuntime) aicommon.StreamableFieldEmitterCallback {
	return func(_ string, reader io.Reader, emitter *aicommon.Emitter) {
		content, err := io.ReadAll(reader)
		if err != nil {
			log.Errorf("intent recommended_capabilities stream read failed: %v", err)
			return
		}
		display := formatRecommendedCapabilitiesDisplay(string(content))
		if strings.TrimSpace(display) == "" {
			return
		}
		if emitter == nil {
			return
		}
		_, err = emitter.EmitDefaultStreamEvent(
			"intent",
			strings.NewReader(display),
			invoker.GetCurrentTaskId(),
		)
		if err != nil {
			log.Errorf("intent recommended_capabilities stream emit failed: %v", err)
		}
	}
}
