package ai

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
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
	client := GetAI("tongyi",
		aispec.WithType("tongyi"),
		aispec.WithAPIKey(string(keyContent)),
		aispec.WithModel("qwen-max"),
		aispec.WithDebugStream(true),
	)
	client.Chat("你是谁？输出一个400字故事")
}

func TestAIBalanceLatest(t *testing.T) {
	if utils.InGithubActions() {
		return
	}

	dir, err := os.UserHomeDir()
	if err != nil {
		t.Fail()
	}
	keyPath := filepath.Join(dir, `yakit-projects/aibalance.txt`)
	keyContent, _ := os.ReadFile(keyPath)
	client := GetAI("aibalance",
		aispec.WithType("aibalance"),
		aispec.WithAPIKey(string(keyContent)),
		aispec.WithModel("gemini-2.0-flash"),
		aispec.WithDebugStream(true),
	)
	client.Chat("你是谁？输出一个400字故事")
}

//go:embed demo2.jpg
var imgzip string

func TestQwenVLMaxLatest(t *testing.T) {
	if utils.InGithubActions() {
		return
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		t.Fail()
	}
	keyPath := filepath.Join(dir, `yakit-projects/yaklang-bailian-apikey.txt`)
	keyContent, _ := os.ReadFile(keyPath)

	a := consts.TempAIFileFast("*.jpg", imgzip)

	client := GetAI("tongyi", aispec.WithType("tongyi"),
		aispec.WithAPIKey(string(keyContent)),
		aispec.WithImageFile(a),
		aispec.WithModel(`qwen-vl-max-latest`),
		aispec.WithDebugStream(true),
	)
	result, err := client.Chat("你看看图片中是什么？")
	if err != nil {
		t.Fatal(err)
		return
	}

	fmt.Println(string(result))
}

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

	expectListGetter := func(latterCall ...string) []string {
		allList := aispec.RegisteredAIGateways()
		result := make([]string, 0)
		result = append(result, latterCall...)

		for _, v := range allList {
			if !utils.StringArrayContains(latterCall, v) {
				result = append(result, v)
			}
		}
		return result
	}

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
	assert.Equal(t, expectListGetter("openai", "chatglm"), cfg.AiApiPriority) // check update new ai type

	// test order of ai type
	cfg.AiApiPriority = []string{"comate", "chatglm"}
	yakit.ConfigureNetWork(cfg)
	Chat("你好", aispec.WithDomain("127.0.0.1"), aispec.WithTimeout(0.01))
	cfg = yakit.GetNetworkConfig()
	assert.Equal(t, expectListGetter("comate", "chatglm"), cfg.AiApiPriority)

	// test auto remove not registered ai type
	cfg.AiApiPriority = []string{"invalidAI", "moonshot", "invalidAI2", "chatglm"}
	yakit.ConfigureNetWork(cfg)
	Chat("你好", aispec.WithDomain("127.0.0.1"), aispec.WithTimeout(0.01))
	cfg = yakit.GetNetworkConfig()
	assert.Equal(t, expectListGetter("moonshot", "chatglm"), cfg.AiApiPriority)
}

type TestGateway struct {
	config *aispec.AIConfig
}

func (t *TestGateway) GetConfig() *aispec.AIConfig {
	return t.config
}

func (t *TestGateway) GetModelList() ([]*aispec.ModelMeta, error) {
	return nil, nil
}

func (t *TestGateway) SupportedStructuredStream() bool {
	return false
}

func (t *TestGateway) StructuredStream(s string, function ...any) (chan *aispec.StructuredData, error) {
	ch := make(chan *aispec.StructuredData)
	defer close(ch)
	return ch, nil
}

func (t *TestGateway) Chat(s string, function ...any) (string, error) {
	if t.config.StreamHandler != nil {
		t.config.StreamHandler(nil)
	}
	return "ok", nil
}

func (t *TestGateway) ChatStream(s string) (io.Reader, error) {
	return nil, nil
}

func (t *TestGateway) ExtractData(data string, desc string, fields map[string]any) (map[string]any, error) {
	return map[string]any{
		"provider": t.config.Type,
		"model":    t.config.Model,
	}, nil
}

func (t *TestGateway) LoadOption(opt ...aispec.AIConfigOption) {
	t.config = aispec.NewDefaultAIConfig(opt...)
}

func (t *TestGateway) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	return nil, nil
}

func (t *TestGateway) CheckValid() error {
	return nil
}

var _ aispec.AIClient = &TestGateway{}

type fallbackControlGateway struct {
	config  *aispec.AIConfig
	valid   bool
	chatErr error
	onChat  func()
}

func (t *fallbackControlGateway) GetConfig() *aispec.AIConfig {
	return t.config
}

func (t *fallbackControlGateway) GetModelList() ([]*aispec.ModelMeta, error) {
	return nil, nil
}

func (t *fallbackControlGateway) SupportedStructuredStream() bool {
	return false
}

func (t *fallbackControlGateway) StructuredStream(string, ...any) (chan *aispec.StructuredData, error) {
	return nil, errors.New("unsupported")
}

func (t *fallbackControlGateway) Chat(string, ...any) (string, error) {
	if t.onChat != nil {
		t.onChat()
	}
	return "ok", t.chatErr
}

func (t *fallbackControlGateway) ChatStream(string) (io.Reader, error) {
	return nil, nil
}

func (t *fallbackControlGateway) ExtractData(string, string, map[string]any) (map[string]any, error) {
	return nil, errors.New("unsupported")
}

func (t *fallbackControlGateway) LoadOption(opt ...aispec.AIConfigOption) {
	t.config = aispec.NewDefaultAIConfig(opt...)
}

func (t *fallbackControlGateway) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	return nil, nil
}

func (t *fallbackControlGateway) CheckValid() error {
	if t.valid {
		return nil
	}
	return errors.New("invalid")
}

var _ aispec.AIClient = &fallbackControlGateway{}

func TestClientStreamExtInfo(t *testing.T) {
	cfg := yakit.GetNetworkConfig()
	if cfg == nil {
		t.Fail()
	}
	bak := cfg.AiApiPriority // backup the original value
	defer func() {           // restore the original value
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

func TestChat_DisableProviderFallback(t *testing.T) {
	cfg := yakit.GetNetworkConfig()
	if cfg == nil {
		t.Fail()
	}
	bak := append([]string(nil), cfg.AiApiPriority...)
	defer func() {
		cfg := yakit.GetNetworkConfig()
		cfg.AiApiPriority = bak
		yakit.ConfigureNetWork(cfg)
	}()

	const badProvider = "test-disable-fallback-bad"
	const goodProvider = "test-disable-fallback-good"

	var badCalls int
	var goodCalls int

	aispec.Register(badProvider, func() aispec.AIClient {
		return &fallbackControlGateway{
			valid:   true,
			chatErr: errors.New("bad provider failed"),
			onChat: func() {
				badCalls++
			},
		}
	})
	aispec.Register(goodProvider, func() aispec.AIClient {
		return &fallbackControlGateway{
			valid: true,
			onChat: func() {
				goodCalls++
			},
		}
	})

	cfg.AiApiPriority = []string{goodProvider, badProvider}
	yakit.ConfigureNetWork(cfg)

	_, err := Chat("hello",
		aispec.WithType(badProvider),
		aispec.WithDisableProviderFallback(true),
	)
	assert.ErrorContains(t, err, "bad provider failed")
	assert.Equal(t, 1, badCalls)
	assert.Equal(t, 0, goodCalls)
}

func TestDashscope_Search_StructuredStream(t *testing.T) {
	if utils.InGithubActions() {
		return
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		t.Fail()
	}
	keyPath := filepath.Join(dir, `yakit-projects/yaklang-bailian-apikey.txt`)
	keyContent, _ := os.ReadFile(keyPath)

	// 测试用例 提前中断请求
	t.Run("提前中断", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// 模拟提前取消（例如用户中断）
		go func() {
			time.Sleep(500 * time.Millisecond) // 等待部分数据到达
			cancel()
		}()

		ch, err := StructuredStream("web fuzzer 用法", aispec.WithType("yaklang-com-search"), aispec.WithAPIKey(string(keyContent)), aispec.WithContext(ctx))
		if err != nil {
			t.Fatalf("提前中断流程失败: %v", err)
		}

		for range ch { // 仅消费部分数据
			t.Log("接收到数据，即将中断...")
			break
		}
		// 验证通道是否正常关闭且无 panic
	})

	// 测试用例 并发请求
	t.Run("并发请求", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ch, err := StructuredStream("web fuzzer 用法", aispec.WithType("yaklang-com-search"), aispec.WithAPIKey(string(keyContent)))
				if err != nil {
					t.Errorf("并发请求失败: %v", err)
					return
				}
				for range ch { // 消费所有数据
				}
			}()
		}
		wg.Wait()
	})

}

func TestChatPreferredTierOverridesGlobalPolicy(t *testing.T) {
	original := consts.GetTieredAIConfig()
	defer consts.SetTieredAIConfig(original)

	aispec.Register("test-intelligent", func() aispec.AIClient { return &TestGateway{} })
	aispec.Register("test-lightweight", func() aispec.AIClient { return &TestGateway{} })
	aispec.Register("test-vision", func() aispec.AIClient { return &TestGateway{} })

	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:       true,
		RoutingPolicy: consts.PolicyCost,
		IntelligentConfigs: []*ypb.AIModelConfig{{
			Provider:  &ypb.ThirdPartyApplicationConfig{Type: "test-intelligent", APIKey: "intelligent-key"},
			ModelName: "intelligent-model",
		}},
		LightweightConfigs: []*ypb.AIModelConfig{{
			Provider:  &ypb.ThirdPartyApplicationConfig{Type: "test-lightweight", APIKey: "lightweight-key"},
			ModelName: "lightweight-model",
		}},
		VisionConfigs: []*ypb.AIModelConfig{{
			Provider:  &ypb.ThirdPartyApplicationConfig{Type: "test-vision", APIKey: "vision-key"},
			ModelName: "vision-model",
		}},
	})

	t.Run("quality priority", func(t *testing.T) {
		var provider, model string
		_, err := Chat("hello",
			aispec.WithQualityPriority(),
			aispec.WithModelInfoCallback(func(p, m string) {
				provider = p
				model = m
			}),
		)
		assert.NoError(t, err)
		assert.Equal(t, "test-intelligent", provider)
		assert.Equal(t, "intelligent-model", model)
	})

	t.Run("speed priority", func(t *testing.T) {
		var provider, model string
		_, err := Chat("hello",
			aispec.WithSpeedPriority(),
			aispec.WithModelInfoCallback(func(p, m string) {
				provider = p
				model = m
			}),
		)
		assert.NoError(t, err)
		assert.Equal(t, "test-lightweight", provider)
		assert.Equal(t, "lightweight-model", model)
	})

	t.Run("vision priority", func(t *testing.T) {
		var provider, model string
		_, err := Chat("hello",
			aispec.WithVisionPriority(),
			aispec.WithModelInfoCallback(func(p, m string) {
				provider = p
				model = m
			}),
		)
		assert.NoError(t, err)
		assert.Equal(t, "test-vision", provider)
		assert.Equal(t, "vision-model", model)
	})

	t.Run("image input auto prefers vision", func(t *testing.T) {
		var provider, model string
		_, err := Chat("hello",
			aispec.WithImageRaw([]byte{
				0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
				0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
				0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
				0x08, 0x04, 0x00, 0x00, 0x00, 0xb5, 0x1c, 0x0c,
				0x02, 0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41,
				0x54, 0x78, 0xda, 0x63, 0xfc, 0xff, 0x1f, 0x00,
				0x03, 0x03, 0x02, 0x00, 0xef, 0xbf, 0xc7, 0x35,
				0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44,
				0xae, 0x42, 0x60, 0x82,
			}),
			aispec.WithModelInfoCallback(func(p, m string) {
				provider = p
				model = m
			}),
		)
		assert.NoError(t, err)
		assert.Equal(t, "test-vision", provider)
		assert.Equal(t, "vision-model", model)
	})
}

func TestChat_DefaultsToAIGlobalConfigWhenNoTypeSpecified(t *testing.T) {
	original := consts.GetTieredAIConfig()
	defer consts.SetTieredAIConfig(original)

	const provider = "test-global-default"
	aispec.Register(provider, func() aispec.AIClient { return &TestGateway{} })

	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled: false,
		IntelligentConfigs: []*ypb.AIModelConfig{{
			Provider:  &ypb.ThirdPartyApplicationConfig{Type: provider, APIKey: "global-key"},
			ModelName: "global-model",
		}},
	})

	var gotProvider, gotModel string
	_, err := Chat("hello",
		aispec.WithModelInfoCallback(func(p, m string) {
			gotProvider = p
			gotModel = m
		}),
		aispec.WithStreamAndConfigHandler(func(reader io.Reader, cfg *aispec.AIConfig) {
			assert.Equal(t, provider, cfg.Type)
			assert.Equal(t, "global-model", cfg.Model)
			assert.Equal(t, "global-key", cfg.APIKey)
		}),
	)
	assert.NoError(t, err)
	assert.Equal(t, provider, gotProvider)
	assert.Equal(t, "global-model", gotModel)
}

func TestFunctionCallSupportsPreferredTier(t *testing.T) {
	original := consts.GetTieredAIConfig()
	defer consts.SetTieredAIConfig(original)

	aispec.Register("test-intelligent-fc", func() aispec.AIClient { return &TestGateway{} })
	aispec.Register("test-lightweight-fc", func() aispec.AIClient { return &TestGateway{} })
	aispec.Register("test-vision-fc", func() aispec.AIClient { return &TestGateway{} })
	aispec.Register("test-explicit-fc", func() aispec.AIClient { return &TestGateway{} })

	consts.SetTieredAIConfig(&consts.TieredAIConfig{
		Enabled:       true,
		RoutingPolicy: consts.PolicyPerformance,
		IntelligentConfigs: []*ypb.AIModelConfig{{
			Provider:  &ypb.ThirdPartyApplicationConfig{Type: "test-intelligent-fc", APIKey: "intelligent-key"},
			ModelName: "intelligent-model",
		}},
		LightweightConfigs: []*ypb.AIModelConfig{{
			Provider:  &ypb.ThirdPartyApplicationConfig{Type: "test-lightweight-fc", APIKey: "lightweight-key"},
			ModelName: "lightweight-model",
		}},
		VisionConfigs: []*ypb.AIModelConfig{{
			Provider:  &ypb.ThirdPartyApplicationConfig{Type: "test-vision-fc", APIKey: "vision-key"},
			ModelName: "vision-model",
		}},
	})

	funcs := map[string]any{"echo": func(input string) string { return input }}

	t.Run("quality priority", func(t *testing.T) {
		result, err := FunctionCall("hello", funcs, aispec.WithQualityPriority())
		assert.NoError(t, err)
		assert.Equal(t, "test-intelligent-fc", result["provider"])
		assert.Equal(t, "intelligent-model", result["model"])
	})

	t.Run("speed priority", func(t *testing.T) {
		result, err := FunctionCall("hello", funcs, aispec.WithSpeedPriority())
		assert.NoError(t, err)
		assert.Equal(t, "test-lightweight-fc", result["provider"])
		assert.Equal(t, "lightweight-model", result["model"])
	})

	t.Run("vision convenience", func(t *testing.T) {
		result, err := VisionFunctionCall("hello", funcs)
		assert.NoError(t, err)
		assert.Equal(t, "test-vision-fc", result["provider"])
		assert.Equal(t, "vision-model", result["model"])
	})

	t.Run("image input auto prefers vision", func(t *testing.T) {
		result, err := FunctionCall("hello", funcs,
			aispec.WithImageRaw([]byte{
				0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
				0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
				0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
				0x08, 0x04, 0x00, 0x00, 0x00, 0xb5, 0x1c, 0x0c,
				0x02, 0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41,
				0x54, 0x78, 0xda, 0x63, 0xfc, 0xff, 0x1f, 0x00,
				0x03, 0x03, 0x02, 0x00, 0xef, 0xbf, 0xc7, 0x35,
				0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44,
				0xae, 0x42, 0x60, 0x82,
			}),
		)
		assert.NoError(t, err)
		assert.Equal(t, "test-vision-fc", result["provider"])
		assert.Equal(t, "vision-model", result["model"])
	})

	t.Run("explicit type wins", func(t *testing.T) {
		result, err := FunctionCall("hello", funcs,
			aispec.WithType("test-explicit-fc"),
			aispec.WithQualityPriority(),
		)
		assert.NoError(t, err)
		assert.Equal(t, "test-explicit-fc", result["provider"])
	})
}

func TestExportsExposeTierHelpers(t *testing.T) {
	assert.Contains(t, Exports, "IntelligentChat")
	assert.Contains(t, Exports, "LightweightChat")
	assert.Contains(t, Exports, "VisionChat")
	assert.Contains(t, Exports, "IntelligentFunctionCall")
	assert.Contains(t, Exports, "LightweightFunctionCall")
	assert.Contains(t, Exports, "VisionFunctionCall")
	assert.Contains(t, Exports, "preferredTier")
	assert.Contains(t, Exports, "speedPriority")
	assert.Contains(t, Exports, "qualityPriority")
	assert.Contains(t, Exports, "visionPriority")
	assert.Contains(t, Exports, "imageAI")
}
