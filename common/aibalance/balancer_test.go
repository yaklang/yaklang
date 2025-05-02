package aibalance

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
)

func TestBalancerBasic(t *testing.T) {
	t.Skip()

	b, err := NewBalancerFromRawConfig([]byte(`keys:
  - key: "your-api-key"
    allowed_models:
      - "model1"
      - "model2"
models:
  - name: "model1"
    providers:
      - model_name: "gemini-2.0-flash"
        type_name: "openai"
        domain_or_url: "http://unreachable-` + utils.RandStringBytes(10) + `.ai.yaklang.io/v1/chat/completions"
        no_https: true
        api_key: "sk-yak-yyds" `))
	if err != nil {
		t.Fatal(err)
	}
	port := utils.GetRandomAvailableTCPPort()
	go func() {
		if err := b.RunWithPort(port); err != nil {
			t.Fatal(err)
		}
	}()
	err = utils.WaitConnect(utils.HostPort("127.0.0.1", port), 3)
	if err != nil {
		t.Fatal(err)
	}
	ai.GetAI(
		"openai",
		aispec.WithAPIKey("your-api-key"),
		aispec.WithModel("model1"),
		aispec.WithDomain("127.0.0.1:"+fmt.Sprint(port)),
		aispec.WithNoHTTPS(true),
	).Chat("你好")
}

// 测试 Balancer 的 Close 方法
func TestBalancerClose(t *testing.T) {
	// 创建临时配置文件
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	// 写入测试配置
	configContent := `
keys:
  - key: test-api-key
    allowed_models:
      - test-model

models:
  - name: test-model
    providers:
      - type_name: openai
        domain_or_url: http://fake-openai-service
        api_key: fake-openai-key
        model_name: gpt-3.5-turbo
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	assert.NoError(t, err, "写入测试配置文件失败")

	// 创建 Balancer
	balancer, err := NewBalancer(configPath)
	assert.NoError(t, err, "创建 Balancer 失败")
	assert.NotNil(t, balancer, "Balancer 不应为空")

	// 在随机端口上运行 Balancer
	port := utils.GetRandomAvailableTCPPort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// 在 goroutine 中运行 Balancer
	go func() {
		err := balancer.RunWithAddr(addr)
		if err != nil {
			// 只有非正常关闭才会报错
			t.Logf("Balancer 运行失败: %v", err)
		}
	}()

	// 等待 Balancer 启动
	time.Sleep(500 * time.Millisecond)

	// 测试连接是否成功
	conn, err := net.Dial("tcp", addr)
	assert.NoError(t, err, "连接到 Balancer 失败")
	assert.NotNil(t, conn, "连接不应为空")
	conn.Close()

	// 关闭 Balancer
	err = balancer.Close()
	assert.NoError(t, err, "关闭 Balancer 失败")

	// 等待关闭完成
	time.Sleep(500 * time.Millisecond)

	// 尝试再次连接，应该失败
	_, err = net.Dial("tcp", addr)
	assert.Error(t, err, "连接应该失败，因为 Balancer 已关闭")
}

// 测试 Balancer 在运行前关闭
func TestBalancerCloseBeforeRun(t *testing.T) {
	// 创建临时配置文件
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	// 写入测试配置
	configContent := `
keys:
  - key: test-api-key
    allowed_models:
      - test-model

models:
  - name: test-model
    providers:
      - type_name: openai
        domain_or_url: http://fake-openai-service
        api_key: fake-openai-key
        model_name: gpt-3.5-turbo
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	assert.NoError(t, err, "写入测试配置文件失败")

	// 创建 Balancer
	balancer, err := NewBalancer(configPath)
	assert.NoError(t, err, "创建 Balancer 失败")
	assert.NotNil(t, balancer, "Balancer 不应为空")

	// 直接关闭 Balancer
	err = balancer.Close()
	assert.NoError(t, err, "关闭 Balancer 失败")
}
