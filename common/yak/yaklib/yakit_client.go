package yaklib

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"time"
)

func YakitOutputToExecResult(i interface{}) *ypb.ExecResult {
	switch ret := i.(type) {
	case *YakitProgress:
		raw, _ := YakitMessageGenerator(ret)
		return &ypb.ExecResult{
			IsMessage: true,
			Message:   raw,
		}
	case *YakitLog:
		raw, _ := YakitMessageGenerator(ret)
		if raw != nil {
			return &ypb.ExecResult{
				IsMessage: true,
				Message:   raw,
			}
		}
	case *ypb.ExecResult:
		return ret
	default:
		log.Warnf("YakitOutputToExecResult unknown type: %T", i)
	}
	return nil
}

// NewVirtualYakitClient 用于脚本执行结果在 grpc 调用时的消息传递
func NewVirtualYakitClient(h func(i *ypb.ExecResult) error) *YakitClient {
	remoteClient := NewYakitClient("")
	remoteClient.send = func(i interface{}) error { // 对于脚本传递的消息，需要封装成 ExecResult
		result := YakitOutputToExecResult(i)
		if h != nil && result != nil {
			return h(result)
		}
		return fmt.Errorf("convert to ExecResult failed: `%v`", i)
	}
	return remoteClient
}

func NewVirtualYakitClientWithRuntimeID(h func(i *ypb.ExecResult) error, runtimeID string) *YakitClient {
	yakitClient := NewVirtualYakitClient(h)
	yakitClient.runtimeID = runtimeID
	return yakitClient
}

func RawHandlerToExecOutput(h func(any) error) func(result *ypb.ExecResult) error {
	return func(result *ypb.ExecResult) error {
		return h(result)
	}
}

type YakitClient struct {
	addr      string
	client    *http.Client
	yakLogger *YakLogger
	send      func(i interface{}) error
	runtimeID string
}

func NewYakitClient(addr string) *YakitClient {
	logger := CreateYakLogger()
	client := &YakitClient{
		addr: addr,
		client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
					MaxVersion:         tls.VersionTLS13,
				},
				TLSHandshakeTimeout:   10 * time.Second,
				DisableCompression:    true,
				MaxIdleConns:          1,
				MaxIdleConnsPerHost:   1,
				MaxConnsPerHost:       1,
				IdleConnTimeout:       5 * time.Minute,
				ResponseHeaderTimeout: 30 * time.Second,
				ExpectContinueTimeout: 30 * time.Second,
			},
			Timeout: 15 * time.Second,
		},
		yakLogger: logger,
	}

	client.send = func(i interface{}) error {
		if client == nil {
			return utils.Errorf("no client set")
		}

		if client.addr == "" {
			return nil
		}

		msgRaw, err := YakitMessageGenerator(i)
		if err != nil {
			return err
		}
		req, err := http.NewRequest("GET", client.addr, bytes.NewBuffer(msgRaw))
		if err != nil {
			return utils.Errorf("build http request failed: %s", err)
		}
		_, err = client.client.Do(req)
		if err != nil {
			log.Errorf("client failed: %s", err)
			return err
		}
		return nil
	}
	client.client.Timeout = 15 * time.Second
	return client
}
func (c *YakitClient) SetYakLog(logger *YakLogger) {
	c.yakLogger = logger
}

func (c *YakitClient) RawSend(i any) error {
	if c.send == nil {
		return nil
	}
	return c.send(i)
}

// 输入
func (c *YakitClient) YakitLog(level string, tmp string, items ...interface{}) error {
	var data = tmp
	if len(items) > 0 {
		data = fmt.Sprintf(tmp, items...)
	}
	logger := c.yakLogger.Info
	switch level {
	case "warn":
		logger = c.yakLogger.Warn
	case "debug":
		logger = c.yakLogger.Debug
	case "error":
		logger = c.yakLogger.Error
	}
	shrinked := data
	if len(shrinked) > 256 {
		shrinked = string([]rune(shrinked)[:100]) + "..."
	}
	logger(shrinked)
	return c.send(&YakitLog{
		Level:     level,
		Data:      data,
		Timestamp: time.Now().Unix(),
	})
}

func (c *YakitClient) YakitDraw(level string, data interface{}) {
	err := c.send(&YakitLog{
		Level:     level,
		Data:      utils.InterfaceToString(data),
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		log.Error(err)
	}
}
func (c *YakitClient) Output(i interface{}) error {
	level, msg := MarshalYakitOutput(i)
	return c.YakitLog(level, msg)
}
func (c *YakitClient) SendRaw(y *YakitLog) error {
	if c == nil {
		return utils.Error("no client")
	}
	return c.send(y)
}

func SetEngineClient(e *antlr4yak.Engine, client *YakitClient) {
	e.OverrideRuntimeGlobalVariables(map[string]any{
		"yakit": GetExtYakitLibByClient(client),
		"risk": map[string]any{
			"Save":                      YakitSaveRiskBuilder(client),
			"NewRisk":                   YakitNewRiskBuilder(client),
			"CheckDNSLogByToken":        yakit.YakitNewCheckDNSLogByToken(yakit.YakitPluginInfo{}),
			"CheckHTTPLogByToken":       yakit.YakitNewCheckHTTPLogByToken(yakit.YakitPluginInfo{}),
			"CheckRandomTriggerByToken": yakit.YakitNewCheckRandomTriggerByToken(yakit.YakitPluginInfo{}),
			"CheckICMPTriggerByLength":  yakit.YakitNewCheckICMPTriggerByLength(yakit.YakitPluginInfo{}),
		},
	})

	//修改全局默认客户端
	InitYakit(client)
}
