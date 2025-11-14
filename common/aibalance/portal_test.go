package aibalance

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	// 初始化数据库
	consts.InitializeYakitDatabase("", "", "")
}

func TestPortalPage(t *testing.T) {
	t.Skip()

	// 从嵌入式模板解析
	tmpl, err := template.ParseFS(templatesFS, "templates/portal.html")
	if err != nil {
		t.Fatalf("Failed to parse template: %v", err)
	}

	// 创建一个测试数据
	data := PortalData{
		CurrentTime:      time.Now().Format("2006-01-02 15:04:05"),
		TotalProviders:   2,
		HealthyProviders: 1,
		TotalRequests:    100,
		SuccessRate:      85.5,
		Providers: []ProviderData{
			{
				ID:            1,
				WrapperName:   "Test Model 1",
				ModelName:     "test-model-1",
				TypeName:      "openai",
				DomainOrURL:   "https://api.example.com",
				TotalRequests: 60,
				SuccessRate:   90.0,
				LastLatency:   120,
				IsHealthy:     true,
			},
			{
				ID:            2,
				WrapperName:   "Test Model 2",
				ModelName:     "test-model-2",
				TypeName:      "chatglm",
				DomainOrURL:   "https://api.example.org",
				TotalRequests: 40,
				SuccessRate:   75.0,
				LastLatency:   200,
				IsHealthy:     false,
			},
		},
		AllowedModels: map[string]string{
			"test-key-1": "test-model-1, test-model-2",
			"test-key-2": "test-model-1",
		},
	}

	// 渲染模板到buffer
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		t.Fatalf("Failed to execute template: %v", err)
	}

	// 验证渲染输出包含期望的内容
	output := buf.String()
	expectedContents := []string{
		"AIBalancer Portal Table",
		data.CurrentTime,
		"Test Model 1",
		"test-model-1",
		"Test Model 2",
		"test-model-2",
		"test-key-1",
		"test-key-2",
	}

	for _, expected := range expectedContents {
		if !bytes.Contains(buf.Bytes(), []byte(expected)) {
			t.Errorf("Expected output to contain %q, but it didn't", expected)
		}
	}

	// 设置一个最短的测试
	if len(output) < 1000 {
		t.Errorf("Template output is suspiciously short: %d bytes", len(output))
	}

	log.Infof("Test successful, template rendered with %d bytes", len(output))
}

// 完整的端到端测试，启动服务器并发送请求
func TestPortalEndToEnd(t *testing.T) {
	t.Skip("Skipping end-to-end test in automated tests")

	// 创建配置并添加测试数据
	config := NewServerConfig()

	// 添加一个模拟的 provider 到数据库
	_, err := RegisterAiProvider(
		"Test Model", "test-model", "openai",
		"https://api.example.com", "test-key", false,
	)
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// 添加API密钥
	key := &Key{
		Key: "test-key",
		AllowedModels: map[string]bool{
			"test-model": true,
		},
	}
	config.Keys.keys["test-key"] = key

	// 添加允许的模型
	config.KeyAllowedModels.allowedModels["test-key"] = map[string]bool{
		"test-model": true,
	}

	// 创建一个上下文来控制服务器生命周期
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 在随机端口启动服务器
	port := utils.GetRandomAvailableTCPPort()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	// 启动服务器
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				conn, err := lis.Accept()
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					t.Errorf("Accept error: %v", err)
					continue
				}
				go config.Serve(conn)
			}
		}
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 发送请求到 /portal 路径
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get(fmt.Sprintf("http://%s/portal", addr))
	if err != nil {
		t.Fatalf("Failed to get portal page: %v", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}

	// 读取响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// 检查响应内容是否包含期望的信息
	expectedContents := []string{
		"AIBalancer Portal Table",
		"Test Model",
		"test-model",
		"test-key",
	}

	for _, expected := range expectedContents {
		if !bytes.Contains(body, []byte(expected)) {
			t.Errorf("Expected response to contain %q, but it didn't", expected)
		}
	}

	log.Infof("End-to-end test successful, received %d bytes", len(body))
}
