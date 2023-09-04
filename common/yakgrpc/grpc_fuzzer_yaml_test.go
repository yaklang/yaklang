package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestWebFuzzerSequence(t *testing.T) {
	seq := &ypb.FuzzerRequests{
		Requests: []*ypb.FuzzerRequest{
			{
				FuzzerIndex: "1",
			},
		},
	}

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.ExportHTTPFuzzerTaskToYaml(context.Background(), &ypb.ExportHTTPFuzzerTaskToYamlRequest{
		Requests: seq,
	})
	if err != nil {
		t.Fatal(err)
	}
	println(res.YamlContent)
	_ = res
}
