package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPC_Ai_List_Model(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	apikey := ""
	proxy := "http://127.0.0.1:7890"
	modelType := "openai"
	rsp, err := client.ListAiModel(context.Background(), &ypb.ListAiModelRequest{
		Config: &ypb.AiConfig{
			ModelType: modelType,
			ApiKey:    apikey,
			NoHTTPS:   false,
			Domain:    "",
			Proxy:     proxy,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, rsp)
	for _, name := range rsp.ModelName {
		t.Log(name)
	}
}
