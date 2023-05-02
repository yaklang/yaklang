package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"testing"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_CreateYaklangShell(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	stream, err := client.CreateYaklangShell(context.Background())
	if err != nil {
		return
	}

	stream.Send(&ypb.YaklangShellRequest{Input: "a = 1"})
	rsp, err := stream.Recv()
	if err != nil {
		panic(err)
	}

	right := false
	for _, scope := range rsp.Scope {
		if scope.GetKey() == "a" && string(scope.GetValue()) == "1" {
			right = true
		}
	}
	if !right {
		panic("scope not right")
	}

	// 检查 a 的值是否可以获到
	stream.Send(&ypb.YaklangShellRequest{Input: "a ++"})
	rsp, err = stream.Recv()
	if err != nil {
		panic(err)
	}
	right = false
	for _, scope := range rsp.Scope {
		if scope.GetKey() == "a" && string(scope.GetValue()) == "2" {
			right = true
		}
	}
	if !right {
		panic("scope not right")
	}
	spew.Dump(rsp.Scope)

	// 检查 a 的值是否可以获到
	stream.Send(&ypb.YaklangShellRequest{Input: "a + 12"}) // 14
	rsp, err = stream.Recv()
	if err != nil {
		panic(err)
	}
	right = false
	for _, scope := range rsp.Scope {
		if scope.GetKey() == "_" && string(scope.GetValue()) == "14" {
			right = true
		}
	}
	if !right {
		panic("scope not right")
	}
	spew.Dump(rsp.Scope)

}
