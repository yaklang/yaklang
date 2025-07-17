package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/h2non/filetype"
	"github.com/yaklang/yaklang/common/authhack"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec/codegrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/common/yserx"
)

func (s *Server) AutoDecode(ctx context.Context, req *ypb.AutoDecodeRequest) (*ypb.AutoDecodeResponse, error) {
	if len(req.GetModifyResult()) == 0 { // 兼容旧版本
		results := funk.Map(codec.AutoDecode(req.GetData()), func(i *codec.AutoDecodeResult) *ypb.AutoDecodeResult {
			return &ypb.AutoDecodeResult{
				Type:        i.Type,
				TypeVerbose: i.TypeVerbose,
				Origin:      []byte(i.Origin),
				Result:      []byte(i.Result),
			}
		}).([]*ypb.AutoDecodeResult)
		return &ypb.AutoDecodeResponse{Results: results}, nil
	} else {
		modifyResult := req.GetModifyResult()
		modifyIndex := -1
		for i := 0; i < len(modifyResult); i++ {
			if modifyResult[i].Modify {
				modifyIndex = i
			}
		}
		if modifyIndex == -1 {
			return &ypb.AutoDecodeResponse{Results: modifyResult}, nil
		}
		for i := modifyIndex; i >= 0; i-- { // 从 result 推origin
			modifyResult[i].Modify = false
			modifyResult[i].Origin = []byte(codec.EncodeByType(modifyResult[i].Type, modifyResult[i].Result))
			if i-1 >= 0 { // 传递给上一级
				modifyResult[i-1].Result = modifyResult[i].Origin
			}
		}
		results2 := funk.Map(codec.AutoDecode(modifyResult[modifyIndex].Result), func(i *codec.AutoDecodeResult) *ypb.AutoDecodeResult {
			return &ypb.AutoDecodeResult{
				Type:        i.Type,
				TypeVerbose: i.TypeVerbose,
				Origin:      []byte(i.Origin),
				Result:      []byte(i.Result),
			}
		}).([]*ypb.AutoDecodeResult)
		if len(results2) == 1 && results2[0].Type == "No" {
			results2 = []*ypb.AutoDecodeResult{}
		}
		return &ypb.AutoDecodeResponse{Results: append(modifyResult[:modifyIndex+1], results2...)}, nil
	}
}

func (s *Server) PacketPrettifyHelper(ctx context.Context, req *ypb.PacketPrettifyHelperRequest) (*ypb.PacketPrettifyHelperResponse, error) {
	ret := req.GetPacket()
	if len(ret) <= 0 {
		return nil, utils.Error("empty packet")
	}

	if req.GetSetReplaceBody() {
		ret = lowhttp.ReplaceHTTPPacketBody(ret, []byte(req.GetBody()), false)
	}

	header, body := lowhttp.SplitHTTPPacketFast(ret)
	var (
		isImage   bool
		imgHeader string // `data:image/gif;base64,...`
	)
	ty, _ := filetype.Match(body)
	if t := ty.MIME.Value; strings.HasPrefix(ty.MIME.Value, "image/") {
		isImage = true
		imgHeader = `<img src=` + strconv.Quote(fmt.Sprintf("data:%s;base64,%s", t, codec.EncodeBase64(body))) + " />"
	}

	contentType := lowhttp.GetHTTPPacketContentType([]byte(header))

	if !isImage && !strings.Contains(contentType, "json") {
		_, ok := utils.IsJSON(string(body))
		if ok {
			contentType = "application/json"
		}
	}

	return &ypb.PacketPrettifyHelperResponse{
		Packet:       ret,
		ContentType:  utils.EscapeInvalidUTF8Byte([]byte(contentType)),
		IsImage:      isImage,
		ImageHtmlTag: []byte(imgHeader),
		Body:         body,
	}, nil
}

func (s *Server) Codec(ctx context.Context, req *ypb.CodecRequest) (*ypb.CodecResponse, error) {
	text := req.GetText()
	if len(req.GetInputBytes()) > 0 {
		text = string(req.GetInputBytes())
	}

	var result string
	var err error = nil
	var raw []byte

	params := make(map[string]string)
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
		buffer := ""
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
		res, err := mutate.FuzzTagExec(text, mutate.Fuzz_WithEnableDangerousTag())
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
		// result = codec.EncodeHtmlEntity(text)
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
		result = codec.StrConvQuoteHex(text)
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
		https := getParams("https", false)
		isHttps := false

		if strings.ToLower(https) == "true" {
			isHttps = true
		}
		cmd, err := lowhttp.GetCurlCommand(isHttps, []byte(text))
		if err != nil {
			return nil, utils.Errorf("codec[%v] failed: %s", `packet-to-curl`, err)
		}
		result = cmd.String()
		raw = []byte(result)
	case "packet-from-curl":
		raw, err = lowhttp.CurlToRawHTTPRequest(text)
		if err != nil {
			return nil, utils.Errorf("codec[%v] failed: %s", "packet-from-curl", err)
		}
		result = string(raw)
	case "packet-from-url":
		raw, err = lowhttp.UrlToHTTPRequest(text)
		if err != nil {
			return nil, utils.Errorf("codec[%v] failed: %s", "packet-from-url", err)
		}
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
		raw, err = codec.AESEncryptCBCWithPKCSPadding([]byte(getParams("key", true)), text, iv)
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
		raw, err = codec.AESDecryptCBCWithPKCSPadding([]byte(getParams("key", true)), decoded, iv)
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
		raw, err = codec.AESEncryptCBCWithPKCSPadding([]byte(getParams("key", true)), text, iv)
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
		raw, err = codec.AESDecryptCBCWithPKCSPadding([]byte(getParams("key", true)), decoded, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = string(raw)
	case "sm4-cfb-encrypt":
		raw, err = codec.SM4EncryptCFBWithPKCSPadding([]byte(getParams("key", true)), text, iv)
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
		raw, err = codec.SM4DecryptCFBWithPKCSPadding([]byte(getParams("key", true)), decoded, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = string(raw)
	case "sm4-ebc-encrypt":
		raw, err = codec.SM4EncryptECBWithPKCSPadding([]byte(getParams("key", true)), text, iv)
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
		raw, err = codec.SM4DecryptECBWithPKCSPadding([]byte(getParams("key", true)), decoded, iv)
		if err != nil {
			return nil, utils.Errorf("%v failed: %s", req.GetType(), err)
		}
		result = string(raw)
	case "sm4-ofb-encrypt":
		raw, err = codec.SM4EncryptOFBWithPKCSPadding([]byte(getParams("key", true)), text, iv)
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
		raw, err = codec.SM4DecryptOFBWithPKCSPadding([]byte(getParams("key", true)), decoded, iv)
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

//func (s *Server) NewCodec(ctx context.Context, req *ypb.CodecRequestFlow) (*ypb.CodecResponse, error) {
//	origin := req.GetText()
//	if len(req.GetInputBytes()) > 0 {
//		origin = string(req.GetInputBytes())
//	}
//
//	var result string
//	var err error = nil
//	var raw []byte
//
//	workFlow := req.GetWorkFlow()
//	text := origin
//
//	for _, work := range workFlow {
//
//		var params = make(map[string]string)
//		for _, item := range work.GetParams() {
//			params[item.Key] = item.Value
//		}
//
//		getParams := func(key string, hexDecode bool) string {
//			value, ok := params[key]
//			if ok {
//				if hexDecode {
//					raw, err := codec.DecodeHex(value)
//					if err != nil {
//						return value
//					}
//					return string(raw)
//				}
//				return value
//			}
//			return ""
//		}
//		ivStr := getParams("iv", true)
//		utils.Debug(func() {
//			spew.Dump(params)
//			log.Infof("fetch iv param: %v", ivStr)
//		})
//
//		var iv []byte
//		if ivStr != "" {
//			iv = []byte(ivStr)
//		}
//
//		codecType := work.GetCodecType()
//		switch codecType {
//		case "json-unicode":
//			// string => unicode => \u0000
//			var buffer = ""
//			for _, r := range []rune(text) {
//				buffer += strings.ReplaceAll(fmt.Sprintf("%U", r), "U+", "\\u")
//			}
//			result = buffer
//		case "json-unicode-decode":
//			TAG := "__YAKCODECFORMATTER_QUOTE_MARK__"
//			result, err = strconv.Unquote(fmt.Sprintf(`"%v"`, strings.ReplaceAll(text, `"`, TAG)))
//			if err != nil {
//				break
//			}
//			result = strings.ReplaceAll(result, TAG, `"`)
//		case "http-get-query":
//			var params url.Values
//			params, err = url.ParseQuery(text)
//			if err != nil {
//				break
//			}
//			result = spew.Sdump(params)
//		case "json-formatter":
//			var dst interface{}
//			err = json.Unmarshal([]byte(text), &dst)
//			if err != nil {
//				break
//			}
//			raw, err = json.MarshalIndent(dst, "", "    ")
//			if err != nil {
//				break
//			}
//			result = string(raw)
//		case "json-formatter-2":
//			var dst interface{}
//			err = json.Unmarshal([]byte(text), &dst)
//			if err != nil {
//				break
//			}
//			raw, err = json.MarshalIndent(dst, "", "  ")
//			if err != nil {
//				break
//			}
//			result = string(raw)
//		case "json-inline":
//			var i interface{}
//			err = json.Unmarshal([]byte(text), &i)
//			if err != nil {
//				break
//			}
//			raw, err = json.Marshal(i)
//			if err != nil {
//				break
//			}
//			result = string(raw)
//		case "fuzz":
//			res, err := mutate.FuzzTagExec(text, mutate.Fuzz_WithEnableFiletag())
//			if err != nil {
//				result = text
//			} else {
//				result = strings.Join(res, "\n")
//			}
//		case "sha1":
//			result = codec.Sha1(text)
//		case "sha256":
//			result = codec.Sha256(text)
//		case "sha512":
//			result = codec.Sha512(text)
//		case "md5":
//			result = codec.Md5(text)
//		case "base64":
//			result = codec.EncodeBase64(text)
//		case "base64-decode":
//			raw, err = codec.DecodeBase64(text)
//			result = string(raw)
//		case "urlencode":
//			result = codec.EncodeUrlCode(text)
//		case "urlescape":
//			result = codec.QueryEscape(text)
//		case "urlescape-path":
//			result = codec.PathEscape(text)
//		case "urlunescape":
//			result, err = codec.QueryUnescape(text)
//		case "urlunescape-path":
//			result, err = codec.PathUnescape(text)
//		case "htmlencode":
//			result = codec.EncodeHtmlEntity(text)
//		case "htmlencode-hex":
//			result = codec.EncodeHtmlEntityHex(text)
//		case "htmlescape":
//			result = codec.EscapeHtmlString(text)
//		case "htmldecode":
//			result = codec.UnescapeHtmlString(text)
//		case "double-urlencode":
//			result = codec.DoubleEncodeUrl(text)
//		case "double-urldecode":
//			result, err = codec.DoubleDecodeUrl(text)
//		case "hex-encode":
//			result = codec.EncodeToHex(text)
//		case "hex-decode":
//			raw, err = codec.DecodeHex(text)
//			result = string(raw)
//		case "str-quote":
//			result = codec.StrConvQuote(text)
//		case "str-unquote":
//			result, err = codec.StrConvUnquote(text)
//		case "http-chunked-encode":
//			raw = codec.HTTPChunkedEncode([]byte(text))
//			result = string(raw)
//		case "http-chunked-decode":
//			raw, err = codec.HTTPChunkedDecode([]byte(text))
//			result = string(raw)
//
//		case "jwt-parse-weak":
//			token, key, err1 := authhack.JwtParse(text)
//			if err1 != nil {
//				return nil, utils.Errorf("codec[%v] failed: %s", codecType, err1)
//			}
//			err = nil
//			raw, err = json.MarshalIndent(map[string]interface{}{
//				"raw":                       token.Raw,
//				"alg":                       token.Method.Alg(),
//				"is_valid":                  token.Valid,
//				"brute_secret_key_finished": token.Valid,
//				"header":                    token.Header,
//				"claims":                    token.Claims,
//				"secret_key":                utils.EscapeInvalidUTF8Byte(key),
//			}, "", "    ")
//			result = string(raw)
//		case "java-unserialize-hex-dumper":
//			raw, _ := codec.DecodeHex(text)
//			result = yserx.JavaSerializedDumper(raw)
//			raw = []byte(result)
//			err = nil
//		case "java-serialize-json":
//			var obj []yserx.JavaSerializable
//			obj, err = yserx.FromJson([]byte(text))
//			if err != nil {
//				return nil, utils.Errorf("codec[%v] failed: %s", codecType, err)
//			}
//			result = codec.EncodeToHex(yserx.MarshalJavaObjects(obj...))
//			raw = []byte(result)
//			err = nil
//		case "java-unserialize-hex":
//			objs, err := yserx.ParseHexJavaSerialized(text)
//			if err != nil {
//				return nil, utils.Errorf("codec[%v] failed: %s", codecType, err)
//			}
//			raw, err = yserx.ToJson(objs)
//			result = string(raw)
//		case "java-unserialize-base64":
//			rawSerial, err := codec.DecodeBase64(text)
//			if err != nil {
//				return nil, utils.Errorf("codec[%v] failed: %s", codecType, err)
//			}
//			objs, err := yserx.ParseJavaSerialized(rawSerial)
//			if err != nil {
//				return nil, utils.Errorf("codec[%v] failed: %s", codecType, err)
//			}
//			raw, err = yserx.ToJson(objs)
//			result = string(raw)
//		case "packet-to-curl":
//			https := getParams("https", false)
//			isHttps := false
//
//			if strings.ToLower(https) == "true" {
//				isHttps = true
//			}
//			cmd, err := lowhttp.GetCurlCommand(isHttps, []byte(text))
//			if err != nil {
//				return nil, utils.Errorf("codec[%v] failed: %s", `packet-to-curl`, err)
//			}
//			result = cmd.String()
//			raw = []byte(result)
//		case "packet-from-curl":
//			raw, err = lowhttp.CurlToHTTPRequest(text)
//			if err != nil {
//				return nil, utils.Errorf("codec[%v] failed: %s", "packet-from-curl", err)
//			}
//			result = string(raw)
//		case "packet-from-url":
//			raw, err = lowhttp.UrlToHTTPRequest(text)
//			if err != nil {
//				return nil, utils.Errorf("codec[%v] failed: %s", "packet-from-url", err)
//			}
//			result = string(raw)
//		case "pretty-packet":
//			headers, bytes := lowhttp.SplitHTTPHeadersAndBodyFromPacket([]byte(text))
//			if bytes != nil && headers != "" {
//				var value interface{}
//				e := json.Unmarshal(bytes, &value)
//				if e != nil {
//					raw = []byte(text)
//					result = text
//					break
//				}
//
//				var newBody []byte
//				newBody, e = json.MarshalIndent(value, "", "    ")
//				if e != nil {
//					raw = []byte(text)
//					result = text
//					break
//				}
//
//				raw = lowhttp.ReplaceHTTPPacketBody([]byte(text), newBody, false)
//				result = string(raw)
//			} else {
//				raw = []byte(text)
//				result = text
//				break
//			}
//		case "aes-cbc-encrypt":
//			raw, err = codec.AESEncryptCBCWithPKCSPadding([]byte(getParams("key", true)), text, iv)
//			if err != nil {
//				return nil, utils.Errorf("aes cbc enc failed: %s", err)
//			}
//			result = codec.EncodeToHex(raw)
//			raw = []byte(result)
//		case "aes-cbc-decrypt":
//			decoded, err := codec.DecodeHex(text)
//			if err != nil {
//				decoded, err = codec.DecodeBase64(text)
//			}
//			if err != nil {
//				return nil, utils.Errorf("decode hex/base64 failed: %s", err)
//			}
//			raw, err = codec.AESDecryptCBCWithPKCSPadding([]byte(getParams("key", true)), decoded, iv)
//			if err != nil {
//				return nil, utils.Errorf("aes cbc dec failed: %s", err)
//			}
//			result = string(raw)
//		case "aes-gcm-encrypt":
//			raw, err = codec.AESGCMEncrypt([]byte(getParams("key", true)), text, iv)
//			if err != nil {
//				return nil, utils.Errorf("aes gcm enc failed: %s", err)
//			}
//			result = codec.EncodeToHex(raw)
//			raw = []byte(result)
//		case "aes-gcm-decrypt":
//			decoded, err := codec.DecodeHex(text)
//			if err != nil {
//				decoded, err = codec.DecodeBase64(text)
//			}
//			if err != nil {
//				return nil, utils.Errorf("decode hex/base64 failed: %s", err)
//			}
//			raw, err = codec.AESGCMEncrypt([]byte(getParams("key", true)), decoded, iv)
//			if err != nil {
//				return nil, utils.Errorf("%v failed: %s", codecType, err)
//			}
//			result = string(raw)
//		case "sm3":
//			raw = codec.SM3(text)
//			result = codec.EncodeToHex(raw)
//			raw = []byte(result)
//		case "sm4-cbc-encrypt":
//			raw, err = codec.AESEncryptCBCWithPKCSPadding([]byte(getParams("key", true)), text, iv)
//			if err != nil {
//				return nil, utils.Errorf("%v failed: %s", codecType, err)
//			}
//			result = codec.EncodeToHex(raw)
//			raw = []byte(result)
//		case "sm4-cbc-decrypt":
//			decoded, err := codec.DecodeHex(text)
//			if err != nil {
//				decoded, err = codec.DecodeBase64(text)
//			}
//			if err != nil {
//				return nil, utils.Errorf("decode hex/base64 failed: %s", err)
//			}
//			raw, err = codec.AESDecryptCBCWithPKCSPadding([]byte(getParams("key", true)), decoded, iv)
//			if err != nil {
//				return nil, utils.Errorf("%v failed: %s", codecType, err)
//			}
//			result = string(raw)
//		case "sm4-cfb-encrypt":
//			raw, err = codec.SM4CFBEnc([]byte(getParams("key", true)), text, iv)
//			if err != nil {
//				return nil, utils.Errorf("%v failed: %s", codecType, err)
//			}
//			result = codec.EncodeToHex(raw)
//			raw = []byte(result)
//		case "sm4-cfb-decrypt":
//			decoded, err := codec.DecodeHex(text)
//			if err != nil {
//				decoded, err = codec.DecodeBase64(text)
//			}
//			if err != nil {
//				return nil, utils.Errorf("decode hex/base64 failed: %s", err)
//			}
//			raw, err = codec.SM4CFBDec([]byte(getParams("key", true)), decoded, iv)
//			if err != nil {
//				return nil, utils.Errorf("%v failed: %s", codecType, err)
//			}
//			result = string(raw)
//		case "sm4-ebc-encrypt":
//			raw, err = codec.SM4ECBEnc([]byte(getParams("key", true)), text, iv)
//			if err != nil {
//				return nil, utils.Errorf("%v failed: %s", codecType, err)
//			}
//			result = codec.EncodeToHex(raw)
//			raw = []byte(result)
//		case "sm4-ebc-decrypt":
//			decoded, err := codec.DecodeHex(text)
//			if err != nil {
//				decoded, err = codec.DecodeBase64(text)
//			}
//			if err != nil {
//				return nil, utils.Errorf("decode hex/base64 failed: %s", err)
//			}
//			raw, err = codec.SM4ECBDec([]byte(getParams("key", true)), decoded, iv)
//			if err != nil {
//				return nil, utils.Errorf("%v failed: %s", codecType, err)
//			}
//			result = string(raw)
//		case "sm4-ofb-encrypt":
//			raw, err = codec.SM4OFBEnc([]byte(getParams("key", true)), text, iv)
//			if err != nil {
//				return nil, utils.Errorf("%v failed: %s", codecType, err)
//			}
//			result = codec.EncodeToHex(raw)
//			raw = []byte(result)
//		case "sm4-ofb-decrypt":
//			decoded, err := codec.DecodeHex(text)
//			if err != nil {
//				decoded, err = codec.DecodeBase64(text)
//			}
//			if err != nil {
//				return nil, utils.Errorf("decode hex/base64 failed: %s", err)
//			}
//			raw, err = codec.SM4OFBDec([]byte(getParams("key", true)), decoded, iv)
//			if err != nil {
//				return nil, utils.Errorf("%v failed: %s", codecType, err)
//			}
//			result = string(raw)
//		case "sm4-gcm-encrypt":
//			raw, err = codec.SM4GCMEnc([]byte(getParams("key", true)), text, iv)
//			if err != nil {
//				return nil, utils.Errorf("%v failed: %s", codecType, err)
//			}
//			result = codec.EncodeToHex(raw)
//			raw = []byte(result)
//		case "sm4-gcm-decrypt":
//			decoded, err := codec.DecodeHex(text)
//			if err != nil {
//				decoded, err = codec.DecodeBase64(text)
//			}
//			if err != nil {
//				return nil, utils.Errorf("decode hex/base64 failed: %s", err)
//			}
//			raw, err = codec.SM4GCMDec([]byte(getParams("key", true)), decoded, iv)
//			if err != nil {
//				return nil, utils.Errorf("%v failed: %s", codecType, err)
//			}
//			result = string(raw)
//		case "base64-url-encode":
//			result = codec.QueryEscape(codec.EncodeBase64(text))
//		case "url-base64-decode":
//			r, _ := codec.QueryUnescape(text)
//			if r == "" {
//				r = text
//			}
//			raw, err = codec.DecodeBase64(r)
//			if err != nil {
//				return nil, utils.Errorf("url-base64-decode failed: %s", err)
//			}
//			result = string(raw)
//		case "unicode-encode":
//			result = codec.JsonUnicodeEncode(text)
//		case "unicode-decode":
//			result = codec.JsonUnicodeDecode(text)
//		case "rc4-encrypt":
//			raw, err = codec.RC4Encrypt([]byte(getParams("key", true)), []byte(text))
//			if err != nil {
//				return nil, utils.Errorf("%v failed: %s", codecType, err)
//			}
//			result = codec.EncodeToHex(raw)
//			raw = []byte(result)
//		case "rc4-decrypt":
//			decoded, err := codec.DecodeHex(text)
//			if err != nil {
//				decoded, err = codec.DecodeBase64(text)
//			}
//			raw, err = codec.RC4Decrypt([]byte(getParams("key", true)), decoded)
//			if err != nil {
//				return nil, utils.Errorf("%v failed: %s", codecType, err)
//			}
//			result = string(raw)
//		case "plugin":
//			if work.GetPluginName() != "" {
//				script, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), work.GetPluginName())
//				if err != nil {
//					return nil, err
//				}
//
//				engine, err := yak.NewScriptEngine(1000).ExecuteEx(script.Content, map[string]interface{}{
//					"YAK_FILENAME": work.GetPluginName(),
//				})
//				if err != nil {
//					return nil, utils.Errorf("execute file %s code failed: %s", work.GetPluginName(), err.Error())
//				}
//				pluginRes, err := engine.CallYakFunction(context.Background(), "handle", []interface{}{text})
//				if err != nil {
//					return nil, utils.Errorf("import %v' s handle failed: %s", work.GetPluginName(), err)
//				}
//				result = fmt.Sprint(pluginRes)
//			} else {
//				return nil, utils.Errorf("not found codec plugin[%v]", work.GetPluginName())
//			}
//		case "custom-script":
//			if work.GetScript() != "" {
//				engine, err := yak.NewScriptEngine(1000).ExecuteEx(work.GetScript(), map[string]interface{}{
//					"YAK_FILENAME": "custom-script",
//				})
//				if err != nil {
//					return nil, utils.Errorf("execute file custom code failed: %s", err.Error())
//				}
//				customScriptRes, err := engine.CallYakFunction(context.Background(), "handle", []interface{}{text})
//				if err != nil {
//					return nil, utils.Errorf("import %v' s handle failed: %s", work.GetPluginName(), err)
//				}
//				result = fmt.Sprint(customScriptRes)
//			}
//		default:
//			return nil, utils.Errorf("unimplemented codec[%v]", codecType)
//		}
//		text = result
//	}
//
//	// 如果是调用 Codec YakScript
//
//	if result == "" {
//		return nil, utils.Errorf("empty result")
//	}
//
//	return &ypb.CodecResponse{Result: utils.EscapeInvalidUTF8Byte([]byte(result))}, nil
//}

func (s *Server) NewCodec(ctx context.Context, req *ypb.CodecRequestFlow) (resp *ypb.CodecResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error(r)
			utils.PrintCurrentGoroutineRuntimeStack()
			err = r.(error)
		}
	}()

	return codegrpc.CodecFlowExec(req)
}

func (s *Server) GetAllCodecMethods(ctx context.Context, in *ypb.Empty) (*ypb.CodecMethods, error) {
	return &ypb.CodecMethods{Methods: codegrpc.GetCodecLibsDocMethods()}, nil
}

func (s *Server) SaveCodecFlow(ctx context.Context, req *ypb.CustomizeCodecFlow) (*ypb.Empty, error) {
	flowByte, err := json.Marshal(req.GetWorkFlow())
	if err != nil {
		return nil, err
	}
	cf := &schema.CodecFlow{
		FlowName:   req.GetFlowName(),
		WorkFlow:   flowByte,
		WorkFlowUI: req.GetWorkFlowUI(),
	}
	err = yakit.CreateCodecFlow(s.GetProfileDatabase(), cf)
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) UpdateCodecFlow(ctx context.Context, req *ypb.UpdateCodecFlowRequest) (*ypb.Empty, error) {
	flowByte, err := json.Marshal(req.GetWorkFlow())
	if err != nil {
		return nil, err
	}
	cf := &schema.CodecFlow{
		FlowName:   req.GetFlowName(),
		WorkFlow:   flowByte,
		WorkFlowUI: req.GetWorkFlowUI(),
	}
	err = yakit.UpdateCodecFlow(s.GetProfileDatabase(), cf)
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeleteCodecFlow(ctx context.Context, req *ypb.DeleteCodecFlowRequest) (*ypb.Empty, error) {
	var err error
	if req.GetDeleteAll() {
		err = yakit.ClearCodecFlow(s.GetProfileDatabase())
	} else {
		err = yakit.DeleteCodecFlow(s.GetProfileDatabase(), req.GetFlowName())
	}
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) GetAllCodecFlow(ctx context.Context, req *ypb.Empty) (*ypb.GetCodecFlowResponse, error) {
	flows, err := yakit.GetAllCodecFlow(s.GetProfileDatabase())
	if err != nil {
		return nil, err
	}
	var res []*ypb.CustomizeCodecFlow
	for _, flow := range flows {
		res = append(res, flow.ToGRPC())
	}

	return &ypb.GetCodecFlowResponse{Flows: res}, nil
}
