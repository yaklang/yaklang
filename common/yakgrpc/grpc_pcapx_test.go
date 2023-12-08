package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	bin_parser "github.com/yaklang/yaklang/common/bin-parser"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gopkg.in/yaml.v2"
	"testing"
)

func TestServer_PcapX(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.GetPcapMetadata(context.Background(), &ypb.PcapMetadataRequest{})
	if err != nil {
		panic(err)
	}
	spew.Dump(rsp)
}
func TestParseTraffic(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	parseRes, err := client.ParseTraffic(context.Background(), &ypb.ParseTrafficRequest{
		Id:   9,
		Type: "reassembled",
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := bin_parser.JsonToResult(parseRes.GetResult())
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(res)
	yamlStr, err := yaml.Marshal(res)
	println(string(yamlStr))
}
