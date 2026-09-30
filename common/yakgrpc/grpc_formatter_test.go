package yakgrpc

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestYakFormatterRPC(t *testing.T) {
	server := &Server{}
	req := &ypb.YaklangCompileAndFormatRequest{Code: `include "missing.yak";select{case v:= <-ch:println(v);default:println(0)}`}
	response, err := server.YaklangCompileAndFormat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	want := "include \"missing.yak\"\nselect {\ncase v := <-ch:\n    println(v)\ndefault:\n    println(0)\n}"
	if response.GetCode() != want {
		t.Fatalf("got:\n%s", response.GetCode())
	}
	req.Code = response.Code
	again, err := server.YaklangCompileAndFormat(context.Background(), req)
	if err != nil || again.Code != response.Code {
		t.Fatal("unstable RPC result", err)
	}
	req.Code = "a="
	if response, err := server.YaklangCompileAndFormat(context.Background(), req); err == nil || response != nil {
		t.Fatal("RPC returned partial source", response, err)
	}
	for _, code := range []string{"break", "1=2", "select{case 1:}"} {
		// Saving still validates compiler semantics before touching a database.
		if _, err := server.SaveYakScript(context.Background(), &ypb.YakScript{Type: "yak", Content: code}); err == nil {
			t.Fatal("save accepted semantic error", code)
		}
	}
}
