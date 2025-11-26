package aicommon

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"testing"
)

func TestConfig_Smoking(t *testing.T) {
	config := NewConfig(context.Background())
	require.NotNil(t, config)
	require.NotNil(t, config.OriginalAICallback)
}

func TestConfig_AIServiceName(t *testing.T) {
	token := uuid.NewString()
	serviceNameOk := false
	config := NewTestConfig(context.Background(),
		WithAIServiceName(token),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			if e.AIService == token {
				serviceNameOk = true
			}
		}),
	)
	config.EmitInfo("abc")

	if serviceNameOk == false {
		t.Fatalf("AIServiceName not set correctly")
	}
}
