package yakgrpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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
