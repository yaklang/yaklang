package yak

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/yakscript"
	"testing"
	"time"
)

func TestYakScriptProviderContract(t *testing.T) {
	for _, test := range []struct {
		name, code, want string
		fail             bool
	}{
		{"return", `chat = func(req, emit) { return req.Prompt }`, "hello", false},
		{"emit", `chat = func(req, emit) { emit("he")~; emit("llo")~ }`, "hello", false},
		{"mixed", `chat = func(req, emit) { emit("he")~; return "llo" }`, "", true},
		{"returned error", `chat = func(req, emit) { return error("adapter failed") }`, "", true},
		{"panic", `chat = func(req, emit) { panic("adapter failed") }`, "", true},
		{"missing", `value = 1`, "", true},
		{"empty", `chat = func(req, emit) { return "" }`, "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			chat, err := ai.LoadChater(yakscript.ProviderType, aispec.WithScript(test.code))
			require.NoError(t, err)
			got, err := chat("hello")
			if test.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.want, got)
			}
		})
	}
	t.Run("cancel VM", func(t *testing.T) {
		chat, err := ai.LoadChater(yakscript.ProviderType, aispec.WithScript(`chat = func(req, emit) { for { } }`))
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err = chat("hello", aispec.WithContext(ctx))
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}
