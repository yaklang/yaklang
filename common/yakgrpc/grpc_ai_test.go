package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestGRPC_Ai_List_Model(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	config := make(map[string]string)
	config["api_key"] = "${api_key}"
	config["proxy"] = "http://127.0.0.1:7890"
	config["Type"] = "openai"
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	rsp, err := client.ListAiModel(context.Background(), &ypb.ListAiModelRequest{
		Config: string(raw),
	})
	require.NoError(t, err)
	require.NotNil(t, rsp)
	for _, name := range rsp.ModelName {
		t.Log(name)
	}
}
