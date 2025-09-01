package ai

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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
	t.config.StreamHandler(nil)
	return "ok", nil
}

func (t *TestGateway) ChatStream(s string) (io.Reader, error) {
	return nil, nil
}

func (t *TestGateway) ExtractData(data string, desc string, fields map[string]any) (map[string]any, error) {
	return nil, nil
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
