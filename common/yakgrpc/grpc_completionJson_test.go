package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
	_ "yaklang.io/yaklang/common/yak"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

func TestServer_GetYakitCompletionRaw(t *testing.T) {
	test := assert.New(t)

	c, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
	}

	rsp, err := c.GetYakitCompletionRaw(context.Background(), &ypb.Empty{})
	if err != nil {
		test.FailNow(err.Error())
	}

	if len(rsp.RawJson) <= 0 {
		test.FailNow("empty result")
	}
	spew.Dump(len(rsp.RawJson))
}

func TestServer_GetYakitCompletionRaw_Antlr4Yak(t *testing.T) {
	test := assert.New(t)

	os.Setenv("YAKMODE", "vm")

	c, err := NewLocalClient()
	if err != nil {
		test.FailNow(err.Error())
	}

	rsp2, err := c.GetYakVMBuildInMethodCompletion(context.Background(), &ypb.GetYakVMBuildInMethodCompletionRequest{})
	if err != nil {
		panic(err)
	}

	if len(rsp2.GetSuggestions()) <= 0 {
		panic(1)
	}

	rsp, err := c.GetYakitCompletionRaw(context.Background(), &ypb.Empty{})
	if err != nil {
		test.FailNow(err.Error())
	}

	if len(rsp.RawJson) <= 0 {
		test.FailNow("empty result")
	}
	spew.Dump(len(rsp.RawJson))

}
