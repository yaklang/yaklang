package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestConfig_SwapEmitter_RestoresPrevious(t *testing.T) {
	cfg := NewConfig(context.Background())
	original := NewEmitter("original", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	alternate := NewEmitter("alternate", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		return e, nil
	})
	cfg.Emitter = original

	restore := cfg.SwapEmitter(alternate)
	require.Same(t, alternate, cfg.GetEmitter())
	restore()
	require.Same(t, original, cfg.GetEmitter())
}
