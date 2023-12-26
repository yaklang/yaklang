package yakgrpc

import (
	"context"
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_LANGUAGE_GetYakitCompletionRaw(t *testing.T) {
	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	rsp, err := c.GetYakitCompletionRaw(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}

	if len(rsp.RawJson) <= 0 {
		t.Fatal("empty result")
	}

	if !utils.MatchAllOfSubString(
		string(rsp.RawJson),
		"QueryUrlsAll() chan string",
		"O_RDWR: int = 0x2",
	) {
		t.Fatal("generate completion failed")
	}
}

func TestServer_GetYakitCompletionRaw_Antlr4Yak(t *testing.T) {
	os.Setenv("YAKMODE", "vm")

	c, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	rsp2, err := c.GetYakVMBuildInMethodCompletion(context.Background(), &ypb.GetYakVMBuildInMethodCompletionRequest{})
	if err != nil {
		t.Fatal(err)
	}

	if len(rsp2.GetSuggestions()) <= 0 {
		t.Fatal("buildin method completion empty")
	}

	rsp, err := c.GetYakitCompletionRaw(context.Background(), &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}

	if len(rsp.RawJson) <= 0 {
		t.Fatal("empty result")
	}
}
