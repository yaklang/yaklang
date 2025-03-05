package ai

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashscope_Search(t *testing.T) {
	if utils.InGithubActions() {
		return
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		t.Fail()
	}
	keyPath := filepath.Join(dir, `yakit-projects/yaklang-bailian-apikey.txt`)
	keyContent, _ := os.ReadFile(keyPath)
	ch, err := StructuredStream("web fuzzer 用法", aispec.WithType("yaklang-com-search"), aispec.WithAPIKey(string(keyContent)))
	if err != nil {
		t.Fail()
	}
	for data := range ch {
		if strings.HasPrefix(data.OutputNodeId, "End_") {
			println(data.OutputText)
		}
	}
}

func TestAutoUpdateAiList(t *testing.T) {
	cfg := yakit.GetNetworkConfig()
	if cfg == nil {
		t.Fail()
	}
	bak := cfg.AiApiPriority // backup the original value
	defer func() { // restore the original value
		cfg.AiApiPriority = bak
		yakit.ConfigureNetWork(cfg)
	}()

	aispec.Register("comate", func() aispec.AIClient { // override the comate gateway, because it wast lots of time to fetch the token
		return nil
	})

	// test auto append the new ai type
	cfg.AiApiPriority = []string{"openai", "chatglm"} // old ai type
	yakit.ConfigureNetWork(cfg)
	Chat("你好", aispec.WithDomain("127.0.0.1"), aispec.WithTimeout(0.01))
	cfg = yakit.GetNetworkConfig()
	if cfg == nil {
		t.Fail()
	}
	assert.Equal(t, []string{"openai", "chatglm", "comate", "moonshot", "tongyi"}, cfg.AiApiPriority) // check update new ai type

	// test order of ai type
	cfg.AiApiPriority = []string{"comate", "chatglm"}
	yakit.ConfigureNetWork(cfg)
	Chat("你好", aispec.WithDomain("127.0.0.1"), aispec.WithTimeout(0.01))
	cfg = yakit.GetNetworkConfig()
	assert.Equal(t, []string{"comate", "chatglm", "openai", "moonshot", "tongyi"}, cfg.AiApiPriority)

	// test auto remove not registered ai type
	cfg.AiApiPriority = []string{"invalidAI", "moonshot", "invalidAI2", "chatglm"}
	yakit.ConfigureNetWork(cfg)
	Chat("你好", aispec.WithDomain("127.0.0.1"), aispec.WithTimeout(0.01))
	cfg = yakit.GetNetworkConfig()
	assert.Equal(t, []string{"moonshot", "chatglm", "comate", "openai", "tongyi"}, cfg.AiApiPriority)
}

type TestGateway struct {
	config *aispec.AIConfig
}

func (t *TestGateway) SupportedStructuredStream() bool {
	//TODO implement me
	panic("implement me")
}

func (t *TestGateway) StructuredStream(s string, function ...aispec.Function) (chan *aispec.StructuredData, error) {
	//TODO implement me
	panic("implement me")
}

func (t *TestGateway) Chat(s string, function ...aispec.Function) (string, error) {
	t.config.StreamHandler(nil)
	return "ok", nil
}

func (t *TestGateway) ChatEx(details []aispec.ChatDetail, function ...aispec.Function) ([]aispec.ChatChoice, error) {
	//TODO implement me
	panic("implement me")
}

func (t *TestGateway) ChatStream(s string) (io.Reader, error) {
	//TODO implement me
	panic("implement me")
}

func (t *TestGateway) ExtractData(data string, desc string, fields map[string]any) (map[string]any, error) {
	//TODO implement me
	panic("implement me")
}

func (t *TestGateway) LoadOption(opt ...aispec.AIConfigOption) {
	t.config = aispec.NewDefaultAIConfig(opt...)
}

func (t *TestGateway) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	//TODO implement me
	panic("implement me")
}

func (t *TestGateway) CheckValid() error {
	return nil
}

var _ aispec.AIClient = &TestGateway{}

func TestClientStreamExtInfo(t *testing.T) {
	cfg := yakit.GetNetworkConfig()
	if cfg == nil {
		t.Fail()
	}
	bak := cfg.AiApiPriority // backup the original value
	defer func() { // restore the original value
		cfg.AiApiPriority = bak
		yakit.ConfigureNetWork(cfg)
	}()
	cfg.AiApiPriority = []string{"test"} // old ai type
	yakit.ConfigureNetWork(cfg)

	aispec.Register("test", func() aispec.AIClient { // override the comate gateway, because it wast lots of time to fetch the token
		return &TestGateway{}
	})

	// test auto append the new ai type

	_, err := Chat("你好", aispec.WithStreamAndConfigHandler(func(reader io.Reader, cfg *aispec.AIConfig) {
		assert.Equal(t, "test", cfg.Type)
	}))
	if err != nil {
		t.Fatal(err)
	}
}
