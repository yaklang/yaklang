package yak

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/yakscript"
)

func TestYakScriptRetryDecisionContract(t *testing.T) {
	for _, tc := range []struct {
		name, script, kind string
		retry              bool
	}{
		{"business default", `chat = func(req, emit) { return ai.ModelError(req, "business", 0, "BUSY", "busy", "60") }`, "business", false},
		{"business decision", `chat = func(req, emit) { ai.ModelError(req, "business", 0, "BUSY", "busy", "60")~ }
   shouldRetry = func(req, failure) { assert failure.Code == "BUSY"; return ai.Retry("busy", "1") }`, "business", true},
		{"protocol opt out", `chat = func(req, emit) { return ai.ModelError(req, "protocol", 0, "", "bad schema", "") }
   shouldRetry = func(req, failure) { return ai.StopRetry("unsupported schema") }`, "protocol", false},
		{"hook failure", `chat = func(req, emit) { return ai.ModelError(req, "business", 0, "", "busy", "") }
   shouldRetry = func(req, failure) { panic("broken hook") }`, "script", false},
		{"hook return type", `chat = func(req, emit) { return ai.ModelError(req, "business", 0, "", "busy", "") }
   shouldRetry = func(req, failure) { return true }`, "script", false},
		{"configuration", `chat = func(req, emit) { return ai.ModelError(req, "http", 401, "", "unauthorized", "") }
   shouldRetry = func(req, failure) { return ai.Retry("force", "") }`, "http", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			chat, err := ai.LoadChater(yakscript.ProviderType, aispec.WithScript(tc.script))
			require.NoError(t, err)
			_, err = chat("fixture", aispec.WithContext(ctx))
			var failure *aispec.ModelCallFailure
			require.True(t, errors.As(err, &failure), "%v", err)
			require.Equal(t, tc.kind, failure.Kind)
			require.Equal(t, tc.retry, failure.CanRetry())
		})
	}
}
