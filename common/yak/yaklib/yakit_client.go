package yaklib

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"strings"
	"time"
)

// NewVirtualYakitClient 用于脚本执行结果在 grpc 调用时的消息传递
func NewVirtualYakitClient(h func(i *ypb.ExecResult) error) *YakitClient {
	remoteClient := NewYakitClient("")
	remoteClient.send = func(i interface{}) error { // 对于脚本传递的消息，需要封装成 ExecResult
		switch ret := i.(type) {
		case *YakitProgress:
			raw, _ := YakitMessageGenerator(ret)
			if err := h(&ypb.ExecResult{
				IsMessage: true,
				Message:   raw,
			}); err != nil {
				return err
			}
		case *YakitLog:
			raw, _ := YakitMessageGenerator(ret)
			if raw != nil {
				if err := h(&ypb.ExecResult{
					IsMessage: true,
					Message:   raw,
				}); err != nil {
					return err
				}
			}

		}
		return nil
	}
	return remoteClient
}

func RawHandlerToExecOutput(h func(any) error) func(result *ypb.ExecResult) error {
	return func(result *ypb.ExecResult) error {
		return h(result)
	}
}

type YakitClient struct {
	addr   string
	client *http.Client
	send   func(i interface{}) error
}

func NewYakitClient(addr string) *YakitClient {
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

func (c *YakitClient) SetProgress(id string, progress float64) error {
	if c == nil {
		return nil
	}

	return c.send(&YakitProgress{
		Id:       id,
		Progress: progress,
	})
}

// 输入
func (c *YakitClient) OutputLog(level string, info string, items ...interface{}) error {
	var data string
	if len(items) > 0 {
		data = fmt.Sprintf(info, items...)
	} else {
		data = info
	}
	f := log.Info
	switch strings.ToLower(level) {
	case "error", "errorf", "failed", "fatal", "panic":
		f = log.Error
	case "warning", "warn":
		f = log.Warn
	case "info", "note", "debug", "":
		fallthrough
	default:
		f = log.Info
	}
	if len(data) > 256 {
		f(string(data[:100]) + "...")
	} else {
		f(data)
	}

	// client 不存在
	if c == nil {
		return nil
	}

	err := c.send(&YakitLog{
		Level:     level,
		Data:      data,
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		log.Errorf("feedback yakit log failed: %s", err)
		return err
	}
	return nil
}

func (c *YakitClient) SendRaw(y *YakitLog) error {
	if c == nil {
		return utils.Error("no client")
	}
	return c.send(y)
}

func (c *YakitClient) Info(info string, items ...interface{}) error {
	return c.OutputLog("info", info, items...)
}

func (c *YakitClient) Error(tmp string, items ...interface{}) error {
	return c.OutputLog("error", tmp, items...)
}

func (c *YakitClient) Warn(info string, items ...interface{}) error {
	return c.OutputLog("warning", info, items...)
}

func (c *YakitClient) Output(t interface{}) error {
	if t == nil {
		return nil
	}
	switch t.(type) {
	case *ypb.ExecResult:
		return c.send(t)
	}
	level, data := MarshalYakitOutput(t)
	if level == "" {
		return utils.Errorf("marshal yakit output failed")
	}

	return c.OutputLog(level, data)
}

func (c *YakitClient) Save(t interface{}) error {
	var r interface{}
	switch ret := t.(type) {
	case *fp.MatchResult:
		r = NewPortFromMatchResult(ret)
	case *synscan.SynScanResult:
		r = NewPortFromSynScanResult(ret)
	}

	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}

	switch t.(type) {
	case *fp.MatchResult:
		return c.OutputLog("asset-port", string(raw))
	case *synscan.SynScanResult:
		return c.OutputLog("asset-port", string(raw))
	default:
		return c.OutputLog("json", string(raw))
	}
}
func SetEngineClient(e *antlr4yak.Engine, client *YakitClient) {
	//修改yakit库的客户端
	e.ImportSubLibs("yakit", GetExtYakitLibByClient(client))

	//修改全局默认客户端
	InitYakit(client)
}
