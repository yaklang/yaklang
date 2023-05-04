package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/authhack"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/common/yserx"
	"moul.io/http2curl"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (s *Server) AutoDecode(ctx context.Context, req *ypb.AutoDecodeRequest) (*ypb.AutoDecodeResponse, error) {
	results := funk.Map(codec.AutoDecode(req.GetData()), func(i *codec.AutoDecodeResult) *ypb.AutoDecodeResult {
		return &ypb.AutoDecodeResult{
			Type:        i.Type,
			TypeVerbose: i.TypeVerbose,
			Origin:      []byte(i.Origin),
			Result:      []byte(i.Result),
		}
	}).([]*ypb.AutoDecodeResult)
	return &ypb.AutoDecodeResponse{Results: results}, nil
}

func (s *Server) Codec(ctx context.Context, req *ypb.CodecRequest) (*ypb.CodecResponse, error) {
	text := req.GetText()
	if len(req.GetInputBytes()) > 0 {
		text = string(req.GetInputBytes())
	}

	var result string
	var err error = nil
	var raw []byte

	var params = make(map[string]string)
	for _, item := range req.GetParams() {
		params[item.Key] = item.Value
	}

	getParams := func(key string, hexDecode bool) string {
		value, ok := params[key]
		if ok {
			if hexDecode {
				raw, err := codec.DecodeHex(value)
				if err != nil {
					return value
				}
				return string(raw)
			}
			return value
		}
		return ""
	}
	ivStr := getParams("iv", true)
	utils.Debug(func() {
		spew.Dump(params)
		log.Infof("fetch iv param: %v", ivStr)
	})

	var iv []byte
	if ivStr != "" {
		iv = []byte(ivStr)
	}

	// 如果是调用 Codec YakScript
	if req.GetScriptName() != "" {
		rsp := &ypb.CodecResponse{}
		script, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), req.GetScriptName())
		if err != nil {
			return nil, err
		}

		engine, err := yak.NewScriptEngine(1000).ExecuteEx(script.Content, map[string]interface{}{
			"YAK_FILENAME": req.GetScriptName(),
		})
		if err != nil {
			return nil, utils.Errorf("execute file %s code failed: %s", req.GetScriptName(), err.Error())
		}
		result, err := engine.CallYakFunction(context.Background(), "handle", []interface{}{text})
		if err != nil {
			return nil, utils.Errorf("import %v' s handle failed: %s", req.GetScriptName(), err)
		}
		rsp.Result = fmt.Sprint(result)
		return rsp, nil
	}

	switch req.Type {
	case "json-unicode":
		// string => unicode => \u0000
		var buffer = ""
		for _, r := range []rune(text) {
			buffer += strings.ReplaceAll(fmt.Sprintf("%U", r), "U+", "\\u")
		}
		result = buffer
	case "json-unicode-decode":
		TAG := "__YAKCODECFORMATTER_QUOTE_MARK__"
		result, err = strconv.Unquote(fmt.Sprintf(`"%v"`, strings.ReplaceAll(text, `"`, TAG)))
		if err != nil {
			break
		}
		result = strings.ReplaceAll(result, TAG, `"`)
	case "http-get-query":
		var params url.Values
		params, err = url.ParseQuery(text)
		if err != nil {
			break
		}
		result = spew.Sdump(params)
	case "json-formatter":
		var dst interface{}
		err = json.Unmarshal([]byte(text), &dst)
		if err != nil {
			break
		}
		raw, err = json.MarshalIndent(dst, "", "    ")
		if err != nil {
			break
		}
		result = string(raw)
	case "json-formatter-2":
		var dst interface{}
		err = json.Unmarshal([]byte(text), &dst)
		if err != nil {
			break
		}
		raw, err = json.MarshalIndent(dst, "", "  ")
		if err != nil {
			break
		}
		result = string(raw)
	case "json-inline":
		var i interface{}
		err = json.Unmarshal([]byte(text), &i)
		if err != nil {
			break
		}
		raw, err = json.Marshal(i)
		if err != nil {
			break
		}
		result = string(raw)
	case "fuzz":
		res, err := mutate.QuickMutate(text, s.GetProfileDatabase())
		if err != nil {
			result = text
		} else {
			result = strings.Join(res, "\n")
		}
	case "sha1":
		result = codec.Sha1(text)
	case "sha256":
		result = codec.Sha256(text)
	case "sha512":
		result = codec.Sha512(text)
	case "md5":
		result = codec.Md5(text)
	case "base64":
		result = codec.EncodeBase64(text)
	case "base64-decode":
		raw, err = codec.DecodeBase64(text)
		result = string(raw)
	case "urlencode":
		result = codec.EncodeUrlCode(text)
	case "urlescape":
		result = codec.QueryEscape(text)
	case "urlescape-path":
		result = codec.PathEscape(text)
	case "urlunescape":
		result, err = codec.QueryUnescape(text)
	case "urlunescape-path":
		result, err = codec.PathUnescape(text)
	case "htmlencode":
		result = codec.EncodeHtmlEntity(text)
	case "htmlencode-hex":
		result = codec.EncodeHtmlEntityHex(text)
	case "htmlescape":
		result = codec.EscapeHtmlString(text)
	case "htmldecode":
		result = codec.UnescapeHtmlString(text)
	case "double-urlencode":
		result = codec.DoubleEncodeUrl(text)
	case "double-urldecode":
		result, err = codec.DoubleDecodeUrl(text)
	case "hex-encode":
		result = codec.EncodeToHex(text)
	case "hex-decode":
		raw, err = codec.DecodeHex(text)
		result = string(raw)
	case "str-quote":
		result = codec.StrConvQuote(text)
	case "str-unquote":
		result, err = codec.StrConvUnquote(text)
	case "http-chunked-encode":
		raw = codec.HTTPChunkedEncode([]byte(text))
		result = string(raw)
	case "http-chunked-decode":
		raw, err = codec.HTTPChunkedDecode([]byte(text))
		result = string(raw)

	case "jwt-parse-weak":
		token, key, err1 := authhack.JwtParse(text)
		if err1 != nil {
			return nil, utils.Errorf("codec[%v] failed: %s", req.Type, err1)
		}
		err = nil
		raw, err = json.MarshalIndent(map[string]interface{}{
			"raw":                       token.Raw,
			"alg":                       token.Method.Alg(),
			"is_valid":                  token.Valid,
			"brute_secret_key_finished": token.Valid,
			"header":                    token.Header,
			"claims":                    token.Claims,
			"secret_key":                utils.EscapeInvalidUTF8Byte(key),
		}, "", "    ")
		result = string(raw)
	case "java-unserialize-hex-dumper":
		raw, _ := codec.DecodeHex(text)
		result = yserx.JavaSerializedDumper(raw)
		raw = []byte(result)
		err = nil
	case "java-serialize-json":
		var obj []yserx.JavaSerializable
		obj, err = yserx.FromJson([]byte(text))
		if err != nil {
			return nil, utils.Errorf("codec[%v] failed: %s", req.Type, err)
		}
		result = codec.EncodeToHex(yserx.MarshalJavaObjects(obj...))
		raw = []byte(result)
		err = nil
	case "java-unserialize-hex":
		objs, err := yserx.ParseHexJavaSerialized(text)
		if err != nil {
			return nil, utils.Errorf("codec[%v] failed: %s", req.Type, err)
		}
		raw, err = yserx.ToJson(objs)
		result = string(raw)
	case "java-unserialize-base64":
		rawSerial, err := codec.DecodeBase64(text)
		if err != nil {
			return nil, utils.Errorf("codec[%v] failed: %s", req.Type, err)
		}
		objs, err := yserx.ParseJavaSerialized(rawSerial)
		if err != nil {
			return nil, utils.Errorf("codec[%v] failed: %s", req.Type, err)
		}
		raw, err = yserx.ToJson(objs)
		result = string(raw)
	case "packet-to-curl":
		req, err := lowhttp.ParseStringToHttpRequest(text)
		if err != nil {
			return nil, utils.Errorf("codec[%v] failed: %s", "packet-to-curl", err)
		}
		cmd, err := http2curl.GetCurlCommand(req)
		if err != nil {
			return nil, utils.Errorf("codec[%v] failed: %s", `packet-to-curl`, err)
		}
		result = cmd.String()
		raw = []byte(result)
	case "packet-from-curl":
		raw, err = lowhttp.CurlToHTTPRequest(text)
		if err != nil {
			return nil, utils.Errorf("codec[%v] failed: %s", "packet-from-curl", err)
		}
		result = string(raw)
	case "packet-from-url":
		var r *http.Request
		if !(strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://")) {
			text = "http://" + text
		}
		r, err = http.NewRequest("GET", text, http.NoBody)
		if err != nil {
			break
		}

		raw, err = utils.HttpDumpWithBody(r, true)
		if err != nil {
			break
		}
		raw = lowhttp.FixHTTPRequestOut(raw)
		result = string(raw)
	case "pretty-packet":
		headers, bytes := lowhttp.SplitHTTPHeadersAndBodyFromPacket([]byte(text))
		if bytes != nil && headers != "" {
			var value interface{}
			e := json.Unmarshal(bytes, &value)
			if e != nil {
				raw = []byte(text)
				result = text
				break
			}

			var newBody []byte
			newBody, e = json.MarshalIndent(value, "", "    ")
			if e != nil {
				raw = []byte(text)
				result = text
				break
			}

			raw = lowhttp.ReplaceHTTPPacketBody([]byte(text), newBody, false)
			result = string(raw)
		} else {
			raw = []byte(text)
			result = text
			break
		}
	case "aes-cbc-encrypt":
		raw, err = codec.AESCBCEncrypt([]byte(getParams("key", true)), text, iv)
		if err != nil {
			return nil, utils.Errorf("aes cbc enc failed: %s", err)
		}
		result = codec.EncodeToHex(raw)
		raw = []byte(result)
	case "aes-cbc-decrypt":
		decoded, err := codec.DecodeHex(text)
		if err != nil {
			decoded, err = codec.DecodeBase64(text)
		}
		if err != nil {
			return nil, utils.Errorf("decode hex/base64 failed: %s", err)
		}
		raw, err = codec.AESCBCDecrypt([]byte(getParams("key", true)), decoded, iv)
		if err != nil {
			return nil, utils.Errorf("aes cbc dec failed: %s", err)
		}
		result = string(raw)
	case "aes-gcm-encrypt":
		raw, err = codec.AESGCMEncrypt([]byte(getParams("key", true)), text, iv)
		if err != nil {
			return nil, utils.Errorf("aes gcm enc failed: %s", err)
		}
		result = codec.EncodeToHex(raw)
		raw = []byte(result)
	case "aes-gcm-decrypt":
		decoded, err := codec.DecodeHex(text)
		if err != nil {
			decoded, err = codec.DecodeBase64(text)
		}
		if err != nil {
			return nil, utils.Errorf("decode hex/base64 failed: %s", err)
		}
		raw, err = codec.AESGCMEncrypt([]byte(getParams("key", true)), decoded, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = string(raw)
	case "sm3":
		raw = codec.SM3(text)
		result = codec.EncodeToHex(raw)
		raw = []byte(result)
	case "sm4-cbc-encrypt":
		raw, err = codec.AESCBCEncrypt([]byte(getParams("key", true)), text, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = codec.EncodeToHex(raw)
		raw = []byte(result)
	case "sm4-cbc-decrypt":
		decoded, err := codec.DecodeHex(text)
		if err != nil {
			decoded, err = codec.DecodeBase64(text)
		}
		if err != nil {
			return nil, utils.Errorf("decode hex/base64 failed: %s", err)
		}
		raw, err = codec.AESCBCDecrypt([]byte(getParams("key", true)), decoded, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = string(raw)
	case "sm4-cfb-encrypt":
		raw, err = codec.SM4CFBEnc([]byte(getParams("key", true)), text, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = codec.EncodeToHex(raw)
		raw = []byte(result)
	case "sm4-cfb-decrypt":
		decoded, err := codec.DecodeHex(text)
		if err != nil {
			decoded, err = codec.DecodeBase64(text)
		}
		if err != nil {
			return nil, utils.Errorf("decode hex/base64 failed: %s", err)
		}
		raw, err = codec.SM4CFBDec([]byte(getParams("key", true)), decoded, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = string(raw)
	case "sm4-ebc-encrypt":
		raw, err = codec.SM4ECBEnc([]byte(getParams("key", true)), text, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = codec.EncodeToHex(raw)
		raw = []byte(result)
	case "sm4-ebc-decrypt":
		decoded, err := codec.DecodeHex(text)
		if err != nil {
			decoded, err = codec.DecodeBase64(text)
		}
		if err != nil {
			return nil, utils.Errorf("decode hex/base64 failed: %s", err)
		}
		raw, err = codec.SM4ECBDec([]byte(getParams("key", true)), decoded, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = string(raw)
	case "sm4-ofb-encrypt":
		raw, err = codec.SM4OFBEnc([]byte(getParams("key", true)), text, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = codec.EncodeToHex(raw)
		raw = []byte(result)
	case "sm4-ofb-decrypt":
		decoded, err := codec.DecodeHex(text)
		if err != nil {
			decoded, err = codec.DecodeBase64(text)
		}
		if err != nil {
			return nil, utils.Errorf("decode hex/base64 failed: %s", err)
		}
		raw, err = codec.SM4OFBDec([]byte(getParams("key", true)), decoded, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = string(raw)
	case "sm4-gcm-encrypt":
		raw, err = codec.SM4GCMEnc([]byte(getParams("key", true)), text, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = codec.EncodeToHex(raw)
		raw = []byte(result)
	case "sm4-gcm-decrypt":
		decoded, err := codec.DecodeHex(text)
		if err != nil {
			decoded, err = codec.DecodeBase64(text)
		}
		if err != nil {
			return nil, utils.Errorf("decode hex/base64 failed: %s", err)
		}
		raw, err = codec.SM4GCMDec([]byte(getParams("key", true)), decoded, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = string(raw)
	case "base64-url-encode":
		result = codec.QueryEscape(codec.EncodeBase64(text))
	case "url-base64-decode":
		r, _ := codec.QueryUnescape(text)
		if r == "" {
			r = text
		}
		raw, err = codec.DecodeBase64(r)
		if err != nil {
			return nil, utils.Errorf("url-base64-decode failed: %s", err)
		}
		result = string(raw)
	case "unicode-encode":
		result = codec.JsonUnicodeEncode(text)
	case "unicode-decode":
		result = codec.JsonUnicodeDecode(text)
	case "rc4-encrypt":
		raw, err = codec.RC4Encrypt([]byte(getParams("key", true)), []byte(text))
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = codec.EncodeToHex(raw)
		raw = []byte(result)
	case "rc4-decrypt":
		decoded, err := codec.DecodeHex(text)
		if err != nil {
			decoded, err = codec.DecodeBase64(text)
		}
		raw, err = codec.RC4Decrypt([]byte(getParams("key", true)), decoded)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = string(raw)
	default:
		return nil, utils.Errorf("unimplemented codec[%v]", req.Type)
	}

	if err != nil {
		return nil, utils.Errorf("codec[%v] failed: %s", req.Type, err)
	}

	if result == "" {
		return nil, utils.Errorf("empty result")
	}

	return &ypb.CodecResponse{Result: utils.EscapeInvalidUTF8Byte([]byte(result))}, nil
}
