package wsm

import (
	"bytes"
	"encoding/hex"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/wsm/payloads/behinder"
	"github.com/yaklang/yaklang/common/wsm/payloads/yakshell"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

type YakShell struct {
	Url         string
	Pass        string
	Charset     string
	ShellScript string            //shell类型
	CipherMode  string            //加密方式
	Proxy       string            //代理
	Os          string            //系统
	IsSession   bool              //是否启用session mode
	Retry       int64             //重试次数
	Timeout     int64             //超时
	BlockSize   int64             //分块大小
	MaxSize     int64             //上传包最大（M）
	Posts       map[string]string //在post中添加的数据
	Headers     map[string]string //在headers中添加的数据

	encode codecFunc            //加密方式， todo:暂时未使用
	decode codecFunc            //解密方式   todo:暂时未使用
	cache  *utils.Cache[string] //缓存cookie
}

func NewYakShell(shell *ypb.WebShell) (*YakShell, error) {
	Yak := &YakShell{
		Url:        shell.Url,
		Pass:       shell.Pass,
		Charset:    shell.Charset,
		CipherMode: shell.EncMode,
		Proxy:      shell.Proxy,
		IsSession:  shell.ShellOptions.IsSession,
		Retry:      shell.ShellOptions.RetryCount,
		Timeout:    shell.ShellOptions.Timeout,
		BlockSize:  shell.ShellOptions.BlockSize,
		MaxSize:    shell.ShellOptions.MaxSize,
		Posts:      make(map[string]string, 2),
		Headers:    make(map[string]string, 2),
		Os:         shell.Os,
		encode:     nil,
		decode:     nil,
		cache:      utils.NewTTLCache[string](time.Second * 60 * 20),
	}
	if shell.Headers != nil {
		Yak.Headers = shell.Headers
	}
	if shell.Posts != nil {
		Yak.Posts = shell.Posts
	}
	Yak.setContentType()
	return Yak, nil
}

func (y *YakShell) setContentType() {
	if _, ok := y.Headers["Content-type"]; !ok {
		log.Infof("header has contains content-type")
		return
	}
	y.Headers["Content-type"] = "application/x-www-form-urlencoded"
}

func (y *YakShell) getOrCrateSession() string {
	if value, exists := y.cache.Get("session"); !exists {
		tmpSession := uuid.NewString()
		y.cache.Set("session", tmpSession)
		return tmpSession
	} else {
		return value
	}
}

func (y *YakShell) getPostConfig() []poc.PocConfigOption {
	var config []poc.PocConfigOption
	for key, value := range y.Posts {
		config = append(config, poc.WithAppendPostParam(key, value))
	}
	config = append(config, poc.WithProxy(y.Proxy))
	config = append(config, poc.WithAppendHeaders(y.Headers))
	config = append(config, poc.WithTimeout(float64(y.Timeout)))
	//todo: 应该增加重试次数
	config = append(config, poc.WithRetryTimes(int(y.Retry)))
	config = append(config, poc.WithRetryInStatusCode(200, 404, 403, 502, 503, 500))
	if y.IsSession {
		config = append(config, poc.WithSession(y.getOrCrateSession()))
	}
	return config
}

func (y *YakShell) post(data []byte) ([]byte, error) {
	options := append(y.getPostConfig(), poc.WithAppendPostParam(y.Pass, string(data)))
	resp, _, err := poc.DoPOST(y.Url, options...)
	if err != nil {
		return nil, utils.Errorf("http request error: %v", err)
	}
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(resp.RawPacket)
	if len(body) == 0 {
		return nil, utils.Errorf("empty response")
	}
	body = bytes.TrimSuffix(body, []byte("\r\n\r\n"))
	return body, nil
}

// InjectPayload 注入全部payload
func (y *YakShell) InjectPayload() error {
	var data []byte
	var err error
	switch y.ShellScript {
	case ypb.ShellScript_PHP.String():
		data, err = hex.DecodeString(string(yakshell.AllPayload))
	case ypb.ShellScript_ASPX.String():
		//todo
	case ypb.ShellScript_JSP.String():
		//todo
	default:
		log.Errorf("webshell类型错误")
		return utils.Errorf("not found this script")
	}
	encryption, err := yakshell.Encryption(data, []byte(y.Pass), y.CipherMode)
	if err != nil {
		return err
	}
	if _, err = y.post(encryption); err != nil {
		return err
	}
	return nil
}
func (y *YakShell) InjectPayloadIfNoCookie() error {
	if y.IsSession {
		if _, exists := y.cache.Get("session"); !exists {
			return y.InjectPayload()
		}
		log.Infof("当前session未过期，不进行重新注入")
	}
	return nil
}
func (y *YakShell) ClientRequestEncode(raw []byte) ([]byte, error) {
	//TODO implement me
	return nil, nil
}

func (y *YakShell) ServerResponseDecode(raw []byte) ([]byte, error) {
	//TODO implement me
	return nil, nil
}

func (y *YakShell) SetPacketScriptContent(content string) {
	//TODO implement me
}

func (y *YakShell) EchoResultEncodeFormYak(raw []byte) ([]byte, error) {
	return yakshell.Encryption(raw, []byte(y.Pass), y.CipherMode)
}

func (y *YakShell) EchoResultDecodeFormYak(raw []byte) ([]byte, error) {
	return yakshell.Decryption(raw, []byte(y.Pass), y.CipherMode)
}

func (y *YakShell) SetPayloadScriptContent(content string) {
	//TODO implement me
}

func (y *YakShell) Ping(opts ...behinder.ExecParamsConfig) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (y *YakShell) BasicInfo(opts ...behinder.ExecParamsConfig) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (y *YakShell) CommandExec(cmd string, opts ...behinder.ExecParamsConfig) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (y *YakShell) String() string {
	//TODO implement me
	panic("implement me")
}

func (y *YakShell) GenWebShell() string {
	//TODO implement me
	panic("implement me")
}
