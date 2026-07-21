package yakgrpc

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

	mitmPort := utils.GetRandomAvailableTCPPort()
	config := &MITMTestConfig{}
	for _, opt := range opts {
		opt(config)
	}
	if config.OnPortFound != nil {
		config.OnPortFound(mitmPort)
	}

	if config.Context == nil {
		config.Context = utils.TimeoutContextSeconds(20)
	}
	stream, err := client.MITM(config.Context)
	if err != nil {
		t.Fatal(err)
	}

	stream.Send(&ypb.MITMRequest{
		Host:             "127.0.0.1",
		Port:             uint32(mitmPort),
		MaxContentLength: int64(config.MaxContentLength),
	})

	// OnServerStarted must not run inline in the Recv loop: MITM may stream.Send
	// (e.g. large-request notification) while the callback waits on poc/HTTP.
	var startedWg sync.WaitGroup
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp.GetHaveMessage() {
			msg := rsp.GetMessage().GetMessage()
			t.Logf("message: %s", msg)
			if strings.Contains(string(msg), `starting mitm server`) {
				if config.OnServerStarted != nil {
					startedWg.Add(1)
					go func() {
						defer startedWg.Done()
						config.OnServerStarted()
					}()
				}
			}
		}
	}

	done := make(chan struct{})
	go func() {
		startedWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-config.Context.Done():
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("OnServerStarted did not finish after MITM context canceled")
		}
	}
}
