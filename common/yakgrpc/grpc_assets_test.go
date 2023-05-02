package yakgrpc

import (
	"context"
	"testing"
	"yaklang/common/yakgrpc/ypb"
)

func die(err interface{}) {
	if err == nil {
		return
	}
	panic(err)
}

func TestServer_QueryDomains(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	/*rsp, err := client.QueryDomains(context.Background(), &ypb.QueryDomainsRequest{Title: "百度"})
	die(err)

	//println(rsp.String())
	_ = rsp

	rsp2, err := client.QueryAvailableRiskType(context.Background(), &ypb.Empty{})
	die(err)
	//spew.Dump(rsp2)
	_ = rsp2

	ctx := context.Background()
	empty := &ypb.Empty{}
	rsp3, err := client.QueryAvailableRiskLevel(ctx, empty)
	die(err)
	_ = rsp3
	spew.Dump(rsp3)*/

	rsp, err := client.DeletePorts(context.Background(), &ypb.DeletePortsRequest{Ids: []int64{14}})
	die(err)
	_ = rsp
	//rsp2, err := client.QueryPorts(context.Background(), &ypb.QueryPortsRequest{Ports: ""})
	die(err)
	//spew.Dump(rsp2)
	//_ = rsp2
	//rsp2, err := client.QueryAvailableRiskType(context.Background(), &ypb.Empty{})
	//die(err)
	////spew.Dump(rsp2)
	//_ = rsp2
	//
	//ctx := context.Background()
	//empty := &ypb.Empty{}
	//rsp3, err := client.QueryAvailableRiskLevel(ctx, empty)
	//die(err)
	//_ = rsp3
	//spew.Dump(rsp3)
}
