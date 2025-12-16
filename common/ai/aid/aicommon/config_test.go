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
	token2 := uuid.NewString()
	serviceNameOk := false
	serviceModelOk := false
	config := NewTestConfig(context.Background(),
		WithAIChatInfo(token, token2),
		WithEventHandler(func(e *schema.AiOutputEvent) {
			if e.AIService == token {
				serviceNameOk = true
			}
			if e.AIModelName == token2 {
				serviceModelOk = true
			}
		}),
	)
	config.EmitInfo("abc")

	if serviceNameOk == false {
		t.Fatalf("AIServiceName not set correctly")
	}

	if serviceModelOk == false {
		t.Fatalf("AIModelName not set correctly")
	}
}
