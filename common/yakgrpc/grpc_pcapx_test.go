package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
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
func TestQueryTrafficTCPReassembled(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.QueryTrafficTCPReassembled(context.Background(), &ypb.QueryTrafficTCPReassembledRequest{
		FromId:  1,
		UntilId: 2,
	})
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
		Id:   746,
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
func TestName(t *testing.T) {
	type MyStruct struct {
		Message string `json:"message"`
	}
	myStruct := MyStruct{
		Message: "\u0000",
	}

	// 使用 json.Marshal 进行编码
	jsonData, err := json.Marshal(&myStruct)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// 打印编码后的 JSON 数据
	fmt.Println(string(jsonData))
}
