package yakgrpc

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
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

	yamlPath := fmt.Sprintf("/tmp/%s.yaml", utils.RandStringBytes(8))
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.ExportHTTPFuzzerTaskToYaml(context.Background(), &ypb.ExportHTTPFuzzerTaskToYamlRequest{
		Requests: seq,
		YamlPath: yamlPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	byts, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(string(byts))
	_ = res
}
