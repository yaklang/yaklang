package yakgrpc

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
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
