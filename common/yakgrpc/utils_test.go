package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"testing"
)

type MITMTestConfig struct {
	Context          context.Context
	OnServerStarted  func()
	OnPortFound      func(int)
	MaxContentLength int
}

type MITMTestCaseOption func(config *MITMTestConfig)

func CaseWithContext(ctx context.Context) MITMTestCaseOption {
	return func(config *MITMTestConfig) {
		config.Context = ctx
	}
}

func CaseWithMaxContentLength(i int) MITMTestCaseOption {
	return func(config *MITMTestConfig) {
		config.MaxContentLength = i
	}
}

func CaseWithPort(p func(int)) MITMTestCaseOption {
	return func(config *MITMTestConfig) {
		config.OnPortFound = p
	}
}

func CaseWithServerStart(h func()) MITMTestCaseOption {
	return func(config *MITMTestConfig) {
		config.OnServerStarted = h
	}
}

func NewMITMTestCase(t *testing.T, opts ...MITMTestCaseOption) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	stream, err := client.MITM(utils.TimeoutContextSeconds(60))
	if err != nil {
		t.Fatal(err)
	}

	mitmPort := utils.GetRandomAvailableTCPPort()
	config := &MITMTestConfig{}
	for _, opt := range opts {
		opt(config)
	}
	if config.OnPortFound != nil {
		config.OnPortFound(mitmPort)
	}

	stream.Send(&ypb.MITMRequest{
		Host:             "127.0.0.1",
		Port:             uint32(mitmPort),
		MaxContentLength: int64(config.MaxContentLength),
	})

	for {
		rsp, err := stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if rsp.GetHaveMessage() {
			msg := rsp.GetMessage().GetMessage()
			t.Logf("message: %s", msg)
			if strings.Contains(string(msg), `starting mitm server`) {
				if config.OnServerStarted != nil {
					config.OnServerStarted()
				}
			}
		}
	}
}
