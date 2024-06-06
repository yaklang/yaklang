package ai

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"testing"
)

func TestAutoUpdateAiList(t *testing.T) {
	cfg := yakit.GetNetworkConfig()
	if cfg == nil {
		t.Fail()
	}
	bak := cfg.AiApiPriority // backup the original value
	defer func() {           // restore the original value
		cfg.AiApiPriority = bak
		yakit.ConfigureNetWork(cfg)
	}()

	aispec.Register("comate", func() aispec.AIGateway { // override the comate gateway, because it wast lots of time to fetch the token
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
