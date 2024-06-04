package yakgrpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_GetSpaceEngineStatus(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
	}{
		{
			name: "zoomeye",
		},
		{
			name: "shodan",
		},
		{
			name: "hunter",
		},
		{
			name: "fofa",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := &ypb.GetSpaceEngineStatusRequest{Type: test.name}
			resp, err := client.GetSpaceEngineStatus(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			assert.NotNil(t, resp)
			fmt.Println(resp)
		})
	}
}

func TestGRPCMUSTPASS_SpaceEngineCustomDomain(t *testing.T) {
	client, err := NewLocalClient(true)
	if err != nil {
		t.Fatal(err)
	}
	passed := false

	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		passed = true
		spew.Dump(req)
		return []byte(`{}`)
	})

	defer client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})

	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	require.NoError(t, err)
	token := utils.RandSecret(10)
	config.AppConfigs = []*ypb.ThirdPartyApplicationConfig{
		{
			Type:           "fofa",
			APIKey:         token,
			UserIdentifier: "user",
			ExtraParams: []*ypb.KVPair{
				{Key: "domain", Value: utils.HostPort(host, port)},
			},
		},
	}
	_, err = client.SetGlobalNetworkConfig(context.Background(), config)
	require.NoError(t, err)
	yak.Execute(`
ch, err = spacengine.Query("port:8080", spacengine.fofa(str.RandSecret(10)))
for r in ch {
	dump(r)
}
`)
	require.NoError(t, err)
	require.True(t, passed, `space engine set custom domain failed`)
}

// func TestServer_GetSpaceEngineAccountStatus(t *testing.T) {
// 	client, err := NewLocalClient()
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	tests := []struct {
// 		typ     string
// 		apiKey  string
// 		account string
// 	}{
// 		{
// 			typ:    "shodan",
// 			apiKey: "",
// 		},
// 		{
// 			typ:    "hunter",
// 			apiKey: "",
// 		},
// 		{
// 			typ:     "fofa",
// 			apiKey:  "",
// 			account: "",
// 		},
// 		{
// 			typ:    "quake",
// 			apiKey: "",
// 		},
// 	}
// 	for _, test := range tests {
// 		t.Run(test.typ, func(t *testing.T) {
// 			req := &ypb.GetSpaceEngineAccountStatusRequest{Type: test.typ, Key: test.apiKey, Account: test.account}
// 			resp, err := client.GetSpaceEngineAccountStatus(context.Background(), req)
// 			if err != nil {
// 				t.Fatal(err)
// 			}
// 			assert.NotNil(t, resp)
// 			fmt.Println(resp)
// 		})
// 	}
// }
