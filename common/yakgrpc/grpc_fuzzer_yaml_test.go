package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"testing"
)

func TestWebFuzzerSequence(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	infos, err := utils.ReadDir("/Users/z3/Downloads/nuclei-templates-main/http/cves/2023")
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range infos {
		if info.IsDir {
			continue
		}
		byts, err := os.ReadFile(info.Path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(byts)
		rsp, err := client.ImportHTTPFuzzerTaskFromYaml(context.Background(), &ypb.ImportHTTPFuzzerTaskFromYamlRequest{
			YamlContent: content,
		})
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.ExportHTTPFuzzerTaskToYaml(context.Background(), &ypb.ExportHTTPFuzzerTaskToYamlRequest{
			Requests: rsp.Requests,
		})

		if err != nil {
			t.Fatal(err)
		}
		println(res.YamlContent)
		_ = res
	}
}
