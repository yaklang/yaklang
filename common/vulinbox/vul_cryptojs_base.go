package vulinbox

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

//go:embed html/vul_cryptojs_basic.html
var cryptoBasicHtml []byte

//go:embed html/vul_cryptojs_rsa.html
var cryptoRsaHtml []byte

//go:embed html/vul_cryptojs_rsa_keyfromserver.html
var cryptoRsaKeyFromServerHtml []byte

//go:embed html/vul_cryptojs_rsa_keyfromserver_withresponse.html
var cryptoRsaKeyFromServerHtmlWithResponse []byte

//go:embed html/vul_cryptojs_rsa_and_aes.html
var cryptoRsaKeyAndAesHtml []byte

//go:embed html/vul_cryptojslib_template.html
var cryptoJSlibTemplateHtml string

var (
	pub []byte
)

func (s *VulinServer) registerCryptoJS() {
	r := s.router

	var (
		backupPass = []string{"admin", "123456", "admin123", "88888888", "666666"}
		pri        []byte
		username   = "admin"
		password   = backupPass[rand.Intn(len(backupPass))]
	)

	log.Infof("frontend end crypto js user:pass = %v:%v", username, password)
	var isLogined = func(loginUser, loginPass string) bool {
		return loginUser == username && loginPass == password
	}
	var isLoginedFromRaw = func(i any) bool {
		var params = make(map[string]any)
		switch i.(type) {
		case string:
			params = utils.ParseStringToGeneralMap(i)
		case []byte:
			params = utils.ParseStringToGeneralMap(i)
		default:
			params = utils.InterfaceToGeneralMap(i)
		}
		username := utils.MapGetString(params, "username")
		password := utils.MapGetString(params, "password")
		return isLogined(username, password)
	}
	var isLoginedFromRawViaDatabase = func(i any) (bool, string) {
		var params = make(map[string]any)
		switch i.(type) {
		case string:
			params = utils.ParseStringToGeneralMap(i)
		case []byte:
			params = utils.ParseStringToGeneralMap(i)
		default:
			params = utils.InterfaceToGeneralMap(i)
		}
		username := utils.MapGetString(params, "username")
		password := utils.MapGetString(params, "password")
		log.Info("username: ", username, " password: ", password)
		users, err := s.database.GetUserByUsernameUnsafe(username)
		if err != nil {
			return false, utils.Wrapf(err, "get user by username failed: %v", username).Error()
		}
		for _, user := range users {
			tUser := utils.InterfaceToString(*user["username"].(*any))
			tPass := utils.InterfaceToString(*user["password"].(*any))
			log.Infof("user: %v pass: %v expect: %v", tUser, tPass, password)
			if tPass == password {
				return true, "success! your password is correct! inject success!"
			}
		}
		return false, "failed! your password is incorrect! inject failed!"
	}

	var isLoginedFromRawViaDatabase2 = func(i any) (bool, string) {
		var params = make(map[string]any)
		switch i.(type) {
		case string:
			params = utils.ParseStringToGeneralMap(i)
		case []byte:
			params = utils.ParseStringToGeneralMap(i)
		default:
			params = utils.InterfaceToGeneralMap(i)
		}
		username := utils.MapGetString(params, "username")
		password := utils.MapGetString(params, "password")
		log.Info("username: ", username, " password: ", password)
		users, err := s.database.UnsafeSqlQuery(`select * from vulin_users where username = '` + username + "' and password = '" + password + "';")
		if err != nil {
			return false, utils.Wrapf(err, "get user by username failed: %v", username).Error()
		}
		if len(users) > 0 {
			return true, "success! your password is correct! inject success!"
		}
		return false, "failed! your password is incorrect! inject failed!"
	}

	var renderLoginSuccess = func(writer http.ResponseWriter, loginUsername, loginPassword string, fallback []byte, success ...[]byte) {
		if loginUsername != username || loginPassword != password {
			writer.WriteHeader(403)
			writer.Write(fallback)
			return
		}

		if len(success) > 0 {
			writer.Write(success[0])
			return
		}

		writer.Write([]byte(`<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	<p class="success-container">
        <h1>恭喜您！登录成功！</h1>
        <p>欢迎，您已成功登录。</p>
    </p>
</div>
</body>
</html>`))
	}

	var initKey = func() {
		log.Infof("start to GeneratePrivateAndPublicKeyPEMWithPrivateFormatter")
		pri, pub, _ = tlsutils.GeneratePrivateAndPublicKeyPEMWithPrivateFormatter("pkcs8")
	}
	var onceGenerator = sync.Once{}

	r.Use(func(handler http.Handler) http.Handler {
		onceGenerator.Do(initKey)
		return handler
	})

	cryptoGroup := r.PathPrefix("/crypto").Name("高级前端加解密与验签实战").Subrouter()
	cryptoRoutes := []*VulInfo{
		{
			Path:  "/sign/hmac/sha256",
			Title: "前端验证签名（验签）表单：HMAC-SHA256",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				unsafeTemplateRender(writer, request, cryptoJSlibTemplateHtml, map[string]any{
					"url":         `/crypto/sign/hmac/sha256/verify`,
					`extrakv`:     "username: jsonData.username, password: jsonData.password,",
					"title":       "HMAC-sha256 验签",
					"datafield":   "signature",
					"key":         `CryptoJS.enc.Utf8.parse("1234123412341234")`,
					"info":        "签名验证（又叫验签或签名）是验证请求参数是否被篡改的一种常见安全手段，验证签名方法主流的有两种，一种是 KEY+哈希算法，例如 HMAC-MD5 / HMAC-SHA256 等，本案例就是这种方法的典型案例。生成签名的规则为：username=*&password=*。在提交和验证的时候需要分别对提交数据进行处理，签名才可以使用和验证",
					"encrypt":     `CryptoJS.HmacSHA256(word, key.toString(CryptoJS.enc.Utf8)).toString();`,
					"decrypt":     `"";`,
					"jsonhandler": "`username=${jsonData.username}&password=${jsonData.password}`;",
				})
			},
		},
		{
			Path: "/sign/hmac/sha256/verify",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				raw, err := utils.HttpDumpWithBody(request, true)
				if err != nil {
					Failed(writer, request, "dump request failed: %v", err)
					return
				}
				params := utils.ParseStringToGeneralMap(lowhttp.GetHTTPPacketBody(raw))
				keyEncoded := utils.MapGetString(params, "key")
				keyPlain, err := codec.DecodeHex(keyEncoded)
				if err != nil {
					Failed(writer, request, "key decode hex failed: %v", err)
					return
				}
				_ = keyPlain
				originSign := utils.MapGetString(params, "signature")
				siginatureHmacHex := originSign
				//siginatureHmacHex, err := tlsutils.Pkcs1v15Decrypt(pri, []byte(originSignDecoded))
				//if err != nil {
				//	Failed(writer, request, "signature decrypt failed: %v", err)
				//	return
				//}
				username := utils.MapGetString(params, "username")
				password := utils.MapGetString(params, "password")
				backendCalcOrigin := fmt.Sprintf("username=%v&password=%v", username, password)
				dataRaw := codec.HmacSha256(keyPlain, backendCalcOrigin)
				var blocks []string
				var signFinished = string(siginatureHmacHex) == codec.EncodeToHex(dataRaw)
				msg := "ORIGIN -> " + originSign + "\n DECODED HMAC: " + string(siginatureHmacHex) + "\n" +
					"\n Expect: " + codec.EncodeToHex(dataRaw) +
					"\n Key: " + string(keyPlain) +
					"\n OriginData: " + backendCalcOrigin
				if signFinished {
					blocks = append(blocks, block("签名验证成功", msg))
				} else {
					blocks = append(blocks, block("签名验证失败", msg))
				}
				if isLoginedFromRaw(params) && signFinished {
					blocks = append(blocks, block("用户名密码验证成功", "恭喜您，登录成功！"))
				} else {
					blocks = append(blocks, block("用户名密码验证失败", "origin data: "+backendCalcOrigin))
				}

				DefaultRender(BlockContent(blocks...), writer, request)
			},
		},
		{
			Path:  "/sign/rsa/hmacsha256",
			Title: "前端验证签名（验签）表单：先 HMAC-SHA256 再 RSA",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				unsafeTemplateRender(writer, request, cryptoJSlibTemplateHtml, map[string]any{
					"url":     `/crypto/sign/rsa/hmacsha256/verify`,
					`extrakv`: "username: jsonData.username, password: jsonData.password,",
					"title":   "先 HMAC-SHA256 再 RSA",
					"initcode": `
document.getElementById('submit').disabled = true;
document.getElementById('submit').innerText = '需等待公钥获取';
let pubkey;
setTimeout(function(){
	fetch('/crypto/js/rsa/public/key').then(async function(rsp) {
		pubkey = await rsp.text()
		document.getElementById('submit').disabled = false;
		document.getElementById('submit').innerText = '提交表单数据';
		console.info(pubkey)
	})
},300)`,
					"datafield":   "signature",
					"key":         `CryptoJS.enc.Utf8.parse("1234123412341234")`,
					"info":        "签名验证（又叫验签或签名）是验证请求参数是否被篡改的一种常见安全手段，验证签名方法主流的有两种，一种是 KEY+哈希算法，例如 HMAC-MD5 / HMAC-SHA256 等，另一种是使用非对称加密加密 HMAC 的签名信息，本案例就是这种方法的典型案例。生成签名的规则为：username=*&password=*。在提交和验证的时候需要分别对提交数据进行处理，签名才可以使用和验证，这种情况相对来说很复杂",
					"encrypt":     `KEYUTIL.getKey(pubkey).encrypt(CryptoJS.HmacSHA256(word, key.toString(CryptoJS.enc.Utf8)).toString());`,
					"decrypt":     `"";`,
					"jsonhandler": "`username=${jsonData.username}&password=${jsonData.password}`;",
				})
			},
		},
		{
			Path: "/sign/rsa/hmacsha256/verify",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				raw, err := utils.HttpDumpWithBody(request, true)
				if err != nil {
					Failed(writer, request, "dump request failed: %v", err)
					return
				}
				params := utils.ParseStringToGeneralMap(lowhttp.GetHTTPPacketBody(raw))
				keyEncoded := utils.MapGetString(params, "key")
				keyPlain, err := codec.DecodeHex(keyEncoded)
				if err != nil {
					Failed(writer, request, "key decode hex failed: %v", err)
					return
				}
				_ = keyPlain
				originSign := utils.MapGetString(params, "signature")
				originSignDecoded, err := codec.DecodeHex(originSign)
				if err != nil {
					Failed(writer, request, "originSign decode hex failed: %v", err)
					return
				}
				siginatureHmacHex, err := tlsutils.Pkcs1v15Decrypt(pri, []byte(originSignDecoded))
				if err != nil {
					Failed(writer, request, "signature decrypt failed: %v", err)
					return
				}
				username := utils.MapGetString(params, "username")
				password := utils.MapGetString(params, "password")
				backendCalcOrigin := fmt.Sprintf("username=%v&password=%v", username, password)
				dataRaw := codec.HmacSha256(keyPlain, backendCalcOrigin)
				var blocks []string
				var signFinished = string(siginatureHmacHex) == codec.EncodeToHex(dataRaw)
				msg := "RSA -> " + originSign + "\n DECODED HMAC: " + string(siginatureHmacHex) + "\n" +
					"\n Expect: " + codec.EncodeToHex(dataRaw) +
					"\n Key: " + string(keyPlain) +
					"\n OriginData: " + backendCalcOrigin
				if signFinished {
					blocks = append(blocks, block("签名验证成功", msg))
				} else {
					blocks = append(blocks, block("签名验证失败", msg))
				}
				if isLoginedFromRaw(params) && signFinished {
					blocks = append(blocks, block("用户名密码验证成功", "恭喜您，登录成功！"))
				} else {
					blocks = append(blocks, block("用户名密码验证失败", "origin data: "+backendCalcOrigin))
				}

				DefaultRender(BlockContent(blocks...), writer, request)
			},
		},
		{
			Path:  "/js/lib/aes/cbc",
			Title: "CryptoJS.AES(CBC) 前端加密登陆表单",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				unsafeTemplateRender(writer, request, cryptoJSlibTemplateHtml, map[string]any{
					"url":      `/crypto/js/lib/aes/cbc/handler`,
					"initcode": "var iv = CryptoJS.lib.WordArray.random(128/8);",
					`extrakv`:  "iv: iv.toString(),",
					"title":    "AES-CBC(4.0.0 默认) 加密",
					"key":      `CryptoJS.enc.Utf8.parse("1234123412341234")`,
					"info":     "默认使用 CryptoJS.AES(CBC 需要 IV).encrypt/decrypt，默认 PKCS7Padding，密钥长度不足16字节，以 NULL 补充，超过16字节，截断\n 注意：这种加密方式每一次密文可能都不一样",
					"encrypt":  `CryptoJS.AES.encrypt(word, key, {iv: iv}).toString();`,
					"decrypt":  `CryptoJS.AES.decrypt(word, key, {iv: iv}).toString();`,
				})
			},
		},
		{
			Path: "/js/lib/aes/cbc/handler",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				raw, err := utils.HttpDumpWithBody(request, true)
				if err != nil {
					Failed(writer, request, "dump request failed: %v", err)
					return
				}
				params := utils.ParseStringToGeneralMap(lowhttp.GetHTTPPacketBody(raw))
				keyEncoded := utils.MapGetString(params, "key")
				keyPlain, err := codec.DecodeHex(keyEncoded)
				if err != nil {
					Failed(writer, request, "key decode hex failed: %v", err)
					return
				}
				dataBase64D := utils.MapGetString(params, "data")
				ivHex := utils.MapGetString(params, "iv")
				dataRaw, err := codec.DecodeBase64(dataBase64D)
				if err != nil {
					Failed(writer, request, "decode base64 failed: %v", err)
					return
				}
				ivRaw, err := codec.DecodeHex(ivHex)
				if err != nil {
					Failed(writer, request, "iv decode hex failed: %v", err)
					return
				}
				dec, err := codec.AESCBCDecrypt([]byte(keyPlain), dataRaw, ivRaw)
				if err != nil {
					Failed(writer, request, "decrypt failed: %v", err)
					return
				}
				var blocks []string
				blocks = append(blocks, block("解密前端内容成功", string(dec)))
				if isLoginedFromRaw(dec) {
					blocks = append(blocks, block("用户名密码验证成功", "恭喜您，登录成功！"))
				} else {
					blocks = append(blocks, block("用户名密码验证失败", "origin data: "+string(dataBase64D)))
				}

				DefaultRender(BlockContent(blocks...), writer, request)
			},
		},
		{
			Path:  "/js/lib/aes/ecb",
			Title: "CryptoJS.AES(ECB) 前端加密登陆表单",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				unsafeTemplateRender(writer, request, cryptoJSlibTemplateHtml, map[string]any{
					"url":      `/crypto/js/lib/aes/ecb/handler`,
					"initcode": "// ignore:  var iv = CryptoJS.lib.WordArray.random(128/8);",
					`extrakv`:  "// iv: iv.toString(),",
					"title":    "AES(ECB PKCS7) 加密",
					"key":      `CryptoJS.enc.Utf8.parse("1234123412341234")`,
					"info":     "CryptoJS.AES(ECB).encrypt/decrypt，默认 PKCS7Padding，密钥长度不足16字节，以 NULL 补充，超过16字节，截断",
					"encrypt":  `CryptoJS.AES.encrypt(word, key, {mode: CryptoJS.mode.ECB}).toString();`,
					"decrypt":  `CryptoJS.AES.decrypt(word, key, {mode: CryptoJS.mode.ECB}).toString();`,
				})
			},
		},
		{
			Path:  "/js/lib/aes/ecb/sqli",
			Title: "CryptoJS.AES(ECB) 被前端加密的 SQL 注入",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				unsafeTemplateRender(writer, request, cryptoJSlibTemplateHtml, map[string]any{
					"url":      `/crypto/js/lib/aes/ecb/handler/sqli`,
					"initcode": "// ignore:  var iv = CryptoJS.lib.WordArray.random(128/8);",
					`extrakv`:  "// iv: iv.toString(),",
					"title":    "AES(ECB PKCS7) 加密",
					"key":      `CryptoJS.enc.Utf8.parse("1234123412341234")`,
					"info":     "CryptoJS.AES(ECB).encrypt/decrypt，默认 PKCS7Padding，密钥长度不足16字节，以 NULL 补充，超过16字节，截断。本页面中表单有注入，一般扫描器均可扫出，Try it？",
					"encrypt":  `CryptoJS.AES.encrypt(word, key, {mode: CryptoJS.mode.ECB}).toString();`,
					"decrypt":  `CryptoJS.AES.decrypt(word, key, {mode: CryptoJS.mode.ECB}).toString();`,
				})
			},
		},
		{
			Path:  "/js/lib/aes/ecb/sqli/bypass",
			Title: "CryptoJS.AES(ECB) 被前端加密的 SQL 注入(Bypass认证)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				unsafeTemplateRender(writer, request, cryptoJSlibTemplateHtml, map[string]any{
					"url":      `/crypto/js/lib/aes/ecb/handler/sqli/bypass`,
					"initcode": "// ignore:  var iv = CryptoJS.lib.WordArray.random(128/8);",
					`extrakv`:  "// iv: iv.toString(),",
					"title":    "AES(ECB PKCS7) 加密",
					"key":      `CryptoJS.enc.Utf8.parse("1234123412341234")`,
					"info":     "CryptoJS.AES(ECB).encrypt/decrypt，默认 PKCS7Padding，密钥长度不足16字节，以 NULL 补充，超过16字节，截断。本页面中表单有注入，使用 SQL 注入的万能密码试试看？",
					"encrypt":  `CryptoJS.AES.encrypt(word, key, {mode: CryptoJS.mode.ECB}).toString();`,
					"decrypt":  `CryptoJS.AES.decrypt(word, key, {mode: CryptoJS.mode.ECB}).toString();`,
				})
			},
		},
		{
			Path: "/js/lib/aes/ecb/handler",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				raw, err := utils.HttpDumpWithBody(request, true)
				if err != nil {
					Failed(writer, request, "dump request failed: %v", err)
					return
				}
				params := utils.ParseStringToGeneralMap(lowhttp.GetHTTPPacketBody(raw))
				keyEncoded := utils.MapGetString(params, "key")
				keyPlain, err := codec.DecodeHex(keyEncoded)
				if err != nil {
					Failed(writer, request, "key decode hex failed: %v", err)
					return
				}
				dataBase64D := utils.MapGetString(params, "data")
				dataRaw, err := codec.DecodeBase64(dataBase64D)
				if err != nil {
					Failed(writer, request, "decode base64 failed: %v", err)
					return
				}
				dec, err := codec.AESECBDecrypt([]byte(keyPlain), dataRaw, nil)
				if err != nil {
					Failed(writer, request, "decrypt failed: %v", err)
					return
				}
				var blocks []string
				blocks = append(blocks, block("解密前端内容成功", string(dec)))
				if isLoginedFromRaw(dec) {
					blocks = append(blocks, block("用户名密码验证成功", "恭喜您，登录成功！"))
				} else {
					blocks = append(blocks, block("用户名密码验证失败", "origin data: "+string(dataBase64D)))
				}

				DefaultRender(BlockContent(blocks...), writer, request)
			},
		},
		{
			Path: "/js/lib/aes/ecb/handler/sqli",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				raw, err := utils.HttpDumpWithBody(request, true)
				if err != nil {
					Failed(writer, request, "dump request failed: %v", err)
					return
				}
				params := utils.ParseStringToGeneralMap(lowhttp.GetHTTPPacketBody(raw))
				keyEncoded := utils.MapGetString(params, "key")
				keyPlain, err := codec.DecodeHex(keyEncoded)
				if err != nil {
					Failed(writer, request, "key decode hex failed: %v", err)
					return
				}
				dataBase64D := utils.MapGetString(params, "data")
				dataRaw, err := codec.DecodeBase64(dataBase64D)
				if err != nil {
					Failed(writer, request, "decode base64 failed: %v", err)
					return
				}
				dec, err := codec.AESECBDecrypt([]byte(keyPlain), dataRaw, nil)
				if err != nil {
					Failed(writer, request, "decrypt failed: %v", err)
					return
				}
				var blocks []string
				blocks = append(blocks, block("解密前端内容成功", string(dec)))
				flag, ret := isLoginedFromRawViaDatabase(dec)
				if flag {
					blocks = append(blocks, block("用户名密码验证成功", "恭喜您，登录成功！"))
				} else {
					blocks = append(blocks, block("用户名密码验证失败", "origin data: "+string(dataBase64D)+" | "))
				}
				blocks = append(blocks, block("数据库执行额外信息", ret))
				DefaultRender(BlockContent(blocks...), writer, request)
			},
		},
		{
			Path: "/js/lib/aes/ecb/handler/sqli/bypass",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				raw, err := utils.HttpDumpWithBody(request, true)
				if err != nil {
					Failed(writer, request, "dump request failed: %v", err)
					return
				}
				params := utils.ParseStringToGeneralMap(lowhttp.GetHTTPPacketBody(raw))
				keyEncoded := utils.MapGetString(params, "key")
				keyPlain, err := codec.DecodeHex(keyEncoded)
				if err != nil {
					Failed(writer, request, "key decode hex failed: %v", err)
					return
				}
				dataBase64D := utils.MapGetString(params, "data")
				dataRaw, err := codec.DecodeBase64(dataBase64D)
				if err != nil {
					Failed(writer, request, "decode base64 failed: %v", err)
					return
				}
				dec, err := codec.AESECBDecrypt([]byte(keyPlain), dataRaw, nil)
				if err != nil {
					Failed(writer, request, "decrypt failed: %v", err)
					return
				}
				var blocks []string
				blocks = append(blocks, block("解密前端内容成功", string(dec)))
				flag, ret := isLoginedFromRawViaDatabase2(dec)
				if flag {
					blocks = append(blocks, block("用户名密码验证成功", "恭喜您，登录成功！"))
				} else {
					blocks = append(blocks, block("用户名密码验证失败", "origin data: "+string(dataBase64D)+" | "))
				}
				blocks = append(blocks, block("数据库执行额外信息", ret))
				DefaultRender(BlockContent(blocks...), writer, request)
			},
		},
		{
			Path:  "/js/basic",
			Title: "AES-ECB 加密表单（附密码）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var params = make(map[string]interface{})

				var data, _ = utils.HttpDumpWithBody(request, true)
				params["packet"] = string(data)

				if request.Method == "GET" {
					results, err := mutate.FuzzTagExec(cryptoBasicHtml, mutate.Fuzz_WithParams(params))
					if err != nil {
						writer.Write([]byte("<pre>" + string(data) + "<pre> <br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}
					writer.Write([]byte(results[0]))
					return
				}

				if request.Method == "POST" {
					_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
					err := json.Unmarshal(body, &params)
					if err != nil {
						writer.Write([]byte("<pre>" + string(data) + "<pre> <br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}

					key, _ := codec.DecodeHex(utils.MapGetString(params, "key"))
					iv, _ := codec.DecodeHex(utils.MapGetString(params, "iv"))
					encrypted := utils.MapGetString(params, "data")
					encryptedBase64Decoded, _ := codec.DecodeBase64(encrypted)

					var origin, decErr = codec.AESDecryptECBWithPKCSPadding([]byte(key), []byte(encryptedBase64Decoded), []byte(iv))
					spew.Dump(origin, decErr)
					var handled string
					var raw, _ = json.MarshalIndent(map[string]any{
						"key":             utils.MapGetString(params, "key"),
						"key_hex_decoded": string(key),
						"iv":              utils.MapGetString(params, "iv"),
						"iv_hex_deocded":  string(iv),

						"encrypted":                encrypted,
						"encrypted_base64_decoded": strconv.Quote(string(encryptedBase64Decoded)),
						"decrypted":                string(origin),
						"decrypted_error":          fmt.Sprint(decErr),
					}, "", "    ")
					handled = string(raw)
					_ = handled

					if !utf8.Valid(origin) {
						origin = []byte(strconv.Quote(string(origin)))
					} else {
						if strings.HasPrefix(string(origin), `"`) && strings.HasSuffix(string(origin), `"`) {
							var after, _ = strconv.Unquote(string(origin))
							if after != "" {
								origin = []byte(after)
							}
						}
					}

					var i any
					json.Unmarshal(origin, &i)
					params := utils.InterfaceToGeneralMap(i)
					username := utils.MapGetString(params, "username")
					password := utils.MapGetString(params, "password")
					var blocks []string
					blocks = append(blocks, block("解密前端内容成功", string(origin)))
					if isLogined(username, password) {
						blocks = append(blocks, block("用户名密码验证成功", "恭喜您，登录成功！"))
					} else {
						blocks = append(blocks, block("用户名密码验证失败", "origin data: "+string(origin)))
					}
					DefaultRender(BlockContent(blocks...), writer, request)
					//renderLoginSuccess(writer, username, password, []byte(
					//	`<br>`+
					//		`<pre>`+string(data)+`</pre> <br><br><br>	`+
					//		`<pre>`+handled+`</pre> <br><br>	`+
					//		`<pre>`+string(origin)+`</pre> <br><br>	`+
					//		`<pre>`+fmt.Sprint(err)+`</pre> <br><br>	`,
					//))
					return
				}

				writer.WriteHeader(http.StatusMethodNotAllowed)
			},
			RiskDetected: true,
		},
		{
			Path:  "/js/rsa",
			Title: "RSA：加密表单，附密钥",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var params = make(map[string]interface{})

				var data, _ = utils.HttpDumpWithBody(request, true)
				params["packet"] = string(data)

				if request.Method == "GET" {
					results, err := mutate.FuzzTagExec(cryptoRsaHtml, mutate.Fuzz_WithParams(params))
					if err != nil {
						writer.Write([]byte("<pre>" + string(data) + "<pre> <br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}
					writer.Write([]byte(results[0]))
					return
				}

				if request.Method == "POST" {
					_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
					err := json.Unmarshal(body, &params)
					if err != nil {
						writer.Write([]byte("<pre>" + string(data) + "<pre> <br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}

					pubkey := utils.MapGetString(params, "publicKey")
					prikey := utils.MapGetString(params, "privateKey")
					_ = pubkey

					println(prikey)

					encrypted := utils.MapGetString(params, "data")
					encryptedBase64Decoded, _ := codec.DecodeBase64(encrypted)

					var origin, decErr = tlsutils.PemPkcsOAEPDecrypt([]byte(prikey), encryptedBase64Decoded)
					spew.Dump(origin, decErr)
					var handled string
					var raw, _ = json.MarshalIndent(map[string]any{
						"publicKey":                pubkey,
						"privateKey":               prikey,
						"encrypted":                encrypted,
						"encrypted_base64_decoded": strconv.Quote(string(encryptedBase64Decoded)),
						"decrypted":                string(origin),
						"decrypted_error":          fmt.Sprint(decErr),
					}, "", "    ")
					handled = string(raw)

					_ = handled
					if !utf8.Valid(origin) {
						origin = []byte(strconv.Quote(string(origin)))
					} else {
						if strings.HasPrefix(string(origin), `"`) && strings.HasSuffix(string(origin), `"`) {
							var after, _ = strconv.Unquote(string(origin))
							if after != "" {
								origin = []byte(after)
							}
						}
					}

					var i any
					json.Unmarshal(origin, &i)
					if i != nil {
						params = utils.InterfaceToGeneralMap(i)
					} else {
						params = utils.InterfaceToGeneralMap(origin)
					}

					var blocks []string
					blocks = append(blocks, block("解密前端内容成功", string(origin)))
					username := utils.MapGetString(params, "username")
					password := utils.MapGetString(params, "password")
					if isLogined(username, password) {
						blocks = append(blocks, block("用户名密码验证成功", "恭喜您，登录成功！"))
					} else {
						blocks = append(blocks, block("用户名密码验证失败", "origin data: "+string(origin)))
					}
					DefaultRender(BlockContent(blocks...), writer, request)
					return
				}

				writer.WriteHeader(http.StatusMethodNotAllowed)
			},
			RiskDetected: true,
		},
		{
			Path:  "/js/rsa/fromserver",
			Title: "RSA：加密表单服务器传输密钥",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var params = make(map[string]interface{})

				var data, _ = utils.HttpDumpWithBody(request, true)
				params["packet"] = string(data)

				if request.Method == "GET" {
					results, err := mutate.FuzzTagExec(cryptoRsaKeyFromServerHtml, mutate.Fuzz_WithParams(params))
					if err != nil {
						writer.Write([]byte("<pre>" + string(data) + "<pre> <br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}
					writer.Write([]byte(results[0]))
					return
				}

				if request.Method == "POST" {
					_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
					err := json.Unmarshal(body, &params)
					if err != nil {
						writer.Write([]byte("<pre>" + string(data) + "<pre> <br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}

					pubkey := pub
					prikey := pri
					_ = pubkey

					println(prikey)

					encrypted := utils.MapGetString(params, "data")
					encryptedBase64Decoded, _ := codec.DecodeBase64(encrypted)

					var origin, decErr = tlsutils.PemPkcsOAEPDecrypt([]byte(prikey), encryptedBase64Decoded)
					spew.Dump(origin, decErr)
					var handled string
					var raw, _ = json.MarshalIndent(map[string]any{
						"publicKey":                pubkey,
						"privateKey":               prikey,
						"encrypted":                encrypted,
						"encrypted_base64_decoded": strconv.Quote(string(encryptedBase64Decoded)),
						"decrypted":                string(origin),
						"decrypted_error":          fmt.Sprint(decErr),
					}, "", "    ")
					handled = string(raw)
					_ = handled

					if !utf8.Valid(origin) {
						origin = []byte(strconv.Quote(string(origin)))
					} else {
						if strings.HasPrefix(string(origin), `"`) && strings.HasSuffix(string(origin), `"`) {
							var after, _ = strconv.Unquote(string(origin))
							if after != "" {
								origin = []byte(after)
							}
						}
					}

					var i any
					json.Unmarshal(origin, &i)
					if i != nil {
						params = utils.InterfaceToGeneralMap(i)
					} else {
						params = utils.InterfaceToGeneralMap(origin)
					}
					username := utils.MapGetString(params, "username")
					password := utils.MapGetString(params, "password")
					var blocks []string
					blocks = append(blocks, block("解密前端内容成功", string(origin)))
					if isLogined(username, password) {
						blocks = append(blocks, block("用户名密码验证成功", "恭喜您，登录成功！"))
					} else {
						blocks = append(blocks, block("用户名密码验证失败", "origin data: "+string(origin)))
					}
					DefaultRender(BlockContent(blocks...), writer, request)
					return
				}

				writer.WriteHeader(http.StatusMethodNotAllowed)
			},
			RiskDetected: true,
		},
		{
			Path:  "/js/rsa/fromserver/response",
			Title: "RSA：加密表单服务器传输密钥+响应加密",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var params = make(map[string]interface{})

				var data, _ = utils.HttpDumpWithBody(request, true)
				params["packet"] = string(data)

				if request.Method == "GET" {
					results, err := mutate.FuzzTagExec(cryptoRsaKeyFromServerHtmlWithResponse, mutate.Fuzz_WithParams(params))
					if err != nil {
						writer.Write([]byte("<pre>" + string(data) + "<pre> <br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}
					writer.Write([]byte(results[0]))
					return
				}

				if request.Method == "POST" {
					_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
					err := json.Unmarshal(body, &params)
					if err != nil {
						writer.Write([]byte("<pre>" + string(data) + "<pre> <br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}

					pubkey := pub
					prikey := pri
					_ = pubkey

					println(prikey)

					encrypted := utils.MapGetString(params, "data")
					encryptedBase64Decoded, _ := codec.DecodeBase64(encrypted)

					var origin, decErr = tlsutils.PemPkcsOAEPDecrypt([]byte(prikey), encryptedBase64Decoded)
					spew.Dump(origin, decErr)
					var handled string
					var raw, _ = json.MarshalIndent(map[string]any{
						"publicKey":                pubkey,
						"privateKey":               prikey,
						"encrypted":                encrypted,
						"encrypted_base64_decoded": strconv.Quote(string(encryptedBase64Decoded)),
						"decrypted":                string(origin),
						"decrypted_error":          fmt.Sprint(decErr),
					}, "", "    ")
					handled = string(raw)

					if !utf8.Valid(origin) {
						origin = []byte(strconv.Quote(string(origin)))
					} else {
						if strings.HasPrefix(string(origin), `"`) && strings.HasSuffix(string(origin), `"`) {
							var after, _ = strconv.Unquote(string(origin))
							if after != "" {
								origin = []byte(after)
							}
						}
					}

					var rawResponseBody = `<br>` +
						`<pre>` + string(data) + `</pre> <br><br><br>	` +
						`<pre>` + handled + `</pre> <br><br>	` +
						`<pre>` + string(origin) + `</pre> <br><br>	` +
						`<pre>` + fmt.Sprint(err) + `</pre> <br><br>	`

					var i any
					json.Unmarshal(origin, &i)
					if i != nil {
						params = utils.InterfaceToGeneralMap(i)
					} else {
						params = utils.InterfaceToGeneralMap(origin)
					}
					username := utils.MapGetString(params, "username")
					password := utils.MapGetString(params, "password")
					var results = make(map[string]any)
					results["username"] = username
					results["success"] = isLogined(username, password)
					encryptedData, err := tlsutils.PemPkcsOAEPEncrypt(pub, utils.Jsonify(results))
					if err != nil {
						writer.Write([]byte(rawResponseBody + "<br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}
					originData, err := tlsutils.PemPkcsOAEPDecrypt(pri, encryptedData)
					println("-------------------")
					println("-------------------")
					println("-------------------")
					spew.Dump(originData, err)
					println("-------------------")
					println("-------------------")
					println("-------------------")
					raw, _ = json.Marshal(map[string]any{
						"data":   codec.EncodeBase64(encryptedData),
						"origin": string(originData),
					})
					renderLoginSuccess(writer, username, password, raw, raw)
					return
				}

				writer.WriteHeader(http.StatusMethodNotAllowed)
			},
			RiskDetected: true,
		},
		{
			Path:  "/js/rsa/fromserver/response/aes-gcm",
			Title: "前端RSA加密AES密钥，服务器传输",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var params = make(map[string]interface{})

				var data, _ = utils.HttpDumpWithBody(request, true)
				params["packet"] = string(data)

				if request.Method == "GET" {
					results, err := mutate.FuzzTagExec(cryptoRsaKeyAndAesHtml, mutate.Fuzz_WithParams(params))
					if err != nil {
						writer.Write([]byte("<pre>" + string(data) + "<pre> <br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}
					writer.Write([]byte(results[0]))
					return
				}

				if request.Method == "POST" {
					_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(data)
					err := json.Unmarshal(body, &params)
					if err != nil {
						writer.Write([]byte("<pre>" + string(data) + "<pre> <br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}

					pubkey := pub
					prikey := pri
					_ = pubkey

					println(prikey)

					encrypted := utils.MapGetString(params, "data")
					encryptedBase64Decoded, _ := codec.DecodeBase64(encrypted)

					encryptedKey := utils.MapGetString(params, "encryptedKey")
					encryptedIV := utils.MapGetString(params, "encryptedIV")

					encKeyDec, _ := codec.DecodeBase64(encryptedKey)
					encIVDec, _ := codec.DecodeBase64(encryptedIV)

					var originKey, decErr = tlsutils.PemPkcsOAEPDecrypt([]byte(prikey), encKeyDec)
					spew.Dump(originKey, decErr)
					originIV, decErr := tlsutils.PemPkcsOAEPDecrypt([]byte(prikey), encIVDec)
					spew.Dump(originIV, decErr)

					origin, err := codec.AESGCMDecryptWithNonceSize12(originKey, encryptedBase64Decoded, originIV)
					if err != nil {
						spew.Dump(originKey, originIV, encryptedBase64Decoded)
						log.Warnf("AES-GCM Decrypt failed nonce size 12: %v", err)
						writer.Write([]byte("<pre>" + string(data) + "<pre> <br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}

					var handled string
					var raw, _ = json.MarshalIndent(map[string]any{
						"publicKey":    pubkey,
						"privateKey":   prikey,
						"aes-gcm":      true,
						"encryptedKey": encryptedKey,
						"encryptedIV":  encryptedIV,

						"encrypted":                encrypted,
						"encrypted_base64_decoded": strconv.Quote(string(encryptedBase64Decoded)),
						"decrypted":                string(origin),
						"decrypted_error":          fmt.Sprint(decErr),
					}, "", "    ")
					handled = string(raw)

					if !utf8.Valid(origin) {
						origin = []byte(strconv.Quote(string(origin)))
					} else {
						if strings.HasPrefix(string(origin), `"`) && strings.HasSuffix(string(origin), `"`) {
							var after, _ = strconv.Unquote(string(origin))
							if after != "" {
								origin = []byte(after)
							}
						}
					}

					var rawResponseBody = `<br>` +
						`<pre>` + string(data) + `</pre> <br><br><br>	` +
						`<pre>` + handled + `</pre> <br><br>	` +
						`<pre>` + string(origin) + `</pre> <br><br>	` +
						`<pre>` + fmt.Sprint(err) + `</pre> <br><br>	`

					var key, iv = utils.RandStringBytes(16), utils.RandStringBytes(12)
					encryptedKeyData, err := tlsutils.PemPkcsOAEPEncrypt(pub, key)
					if err != nil {
						log.Error(err)
						writer.Write([]byte(rawResponseBody + "<br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}

					encryptedIVData, err := tlsutils.PemPkcsOAEPEncrypt(pub, iv)
					if err != nil {
						writer.Write([]byte(rawResponseBody + "<br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}

					originData, err := tlsutils.PemPkcsOAEPDecrypt(pri, encryptedKeyData)
					println("-------------------")
					println("-------------------")
					println("-------------------")
					spew.Dump(originData, err)
					println("-------------------")
					println("-------------------")
					println("-------------------")

					var i any
					json.Unmarshal(origin, &i)
					if i != nil {
						params = utils.InterfaceToGeneralMap(i)
					} else {
						params = utils.InterfaceToGeneralMap(origin)
					}
					username := utils.MapGetString(params, "username")
					password := utils.MapGetString(params, "password")
					var results = make(map[string]any)
					results["username"] = username
					results["success"] = isLogined(username, password)
					_ = rawResponseBody
					aesEncrypted, err := codec.AESGCMEncryptWithNonceSize12([]byte(key), utils.Jsonify(results), []byte(iv))
					if err != nil {
						log.Errorf("AES-GCM Encrypt failed nonce size 12: %v", err)
						writer.Write([]byte(rawResponseBody + "<br/> <br/> <h2>error</h2> <br/>" + err.Error()))
						return
					}

					raw, _ = json.Marshal(map[string]any{
						"encryptedKey": codec.EncodeBase64(encryptedKeyData),
						"encryptedIV":  codec.EncodeBase64(encryptedIVData),
						"data":         codec.EncodeBase64(aesEncrypted),
					})
					writer.Write(raw)
					return
				}

				writer.WriteHeader(http.StatusMethodNotAllowed)
			},
			RiskDetected: true,
		},
		{
			Path: "/js/rsa/generator",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Write([]byte(`{"ok": true, "publicKey": ` + strconv.Quote(string(pub)) + `, "privateKey": ` + strconv.Quote(string(pri)) + `}`))
			},
		},
		{
			Path: "/js/rsa/public/key",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Write(pub)
			},
		},
	}

	cryptoRoutes = append(cryptoRoutes, s.getEncryptSQLinj()...)

	for _, route := range cryptoRoutes {
		addRouteWithVulInfo(cryptoGroup, route)
	}

	testHandler := func(writer http.ResponseWriter, request *http.Request) {
		DefaultRender("<h1>本测试页面会测试针对 /static/js/cryptojs_4.0.0/*.js 文件的导入</h1>\n"+`

<p id='error'>CryptoJS Status</p>	
<p id='404'></p>	

<script>
window.onerror = function(message, source, lineno, colno, error) {
    console.log('An error has occurred: ', message);
	document.getElementById('error').innerHTML = "CryptoJS ERROR: " + message;
    return true;
};

handle404 = function(event) {
	const p = document.createElement("p")
	p.innerText = "CryptoJS 404: " + event.target.src
	document.getElementById('404').appendChild(p)
}
</script>
<script src="/static/js/cryptojs_4.0.0/core.min.js" onerror="handle404(event)"></script>
<script src="/static/js/cryptojs_4.0.0/enc-base64.min.js" onerror="handle404(event)"></script>
<script src="/static/js/cryptojs_4.0.0/md5.min.js" onerror="handle404(event)"></script>
<script src="/static/js/cryptojs_4.0.0/evpkdf.min.js" onerror="handle404(event)"></script>
<script src="/static/js/cryptojs_4.0.0/cipher-core.min.js" onerror="handle404(event)"></script>
<script src="/static/js/cryptojs_4.0.0/aes.min.js" onerror="handle404(event)"></script>
<script src="/static/js/cryptojs_4.0.0/pad-pkcs7.min.js" onerror="handle404(event)"></script>
<script src="/static/js/cryptojs_4.0.0/mode-ecb.min.js" onerror="handle404(event)"></script>
<script src="/static/js/cryptojs_4.0.0/enc-utf8.min.js" onerror="handle404(event)"></script>
<script src="/static/js/cryptojs_4.0.0/enc-hex.min.js" onerror="handle404(event)"></script>

可以观察 Console 
`, writer, request)
	}
	cryptoGroup.HandleFunc("/_test/", testHandler)
	cryptoGroup.HandleFunc("/", testHandler)
}

func (s *VulinServer) registerCryptoJSBugs() {
	r := s.router
	cryptoGroup := r.PathPrefix("/crypto").Name("高级前端加解密与验签实战").Subrouter()

	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/challenge-api-docs",
		Title: "动态挑战响应API靶场(2024-07-31新增)",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			html := `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>动态挑战-响应 API 安全靶场</title>
    <style>
        body { font-family: sans-serif; line-height: 1.6; padding: 20px; max-width: 800px; margin: auto; }
        h1, h2 { color: #333; }
        code { background-color: #f4f4f4; padding: 2px 6px; border-radius: 4px; }
        .workflow { text-align: center; margin: 20px 0; }
        .key { color: #c7254e; background-color: #f9f2f4; padding: 2px 4px; border-radius: 4px; }
    </style>
</head>
<body>
    <h1>动态挑战-响应 API 安全靶场</h1>
    <p>这是一个模拟真实世界高安全性API的靶场。它使用动态挑战-响应机制来防止重放攻击，并对业务数据进行加密传输。常规的Fuzzer很难成功请求此类型API。</p>
    
    <h2>交互流程</h2>
    <div class="workflow">
        <script src="https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.min.js"></script>
        <script>mermaid.initialize({startOnLoad:true});</script>
        <div class="mermaid">
        sequenceDiagram
            participant Client as 客户端
            participant Server as 服务器
            Client->>Server: 1. GET /api/get-challenge
            Server-->>Client: 返回 {"challenge": "...", "iv": "..."}
            Client->>Client: 2. 解密 challenge 获取 nonce<br/>3. 使用 HMAC-SHA256(nonce) 计算签名
            Client->>Server: 4. GET /api/user/info<br/>Header: X-Auth-Signature: &lt;signature&gt;
            Server->>Server: 5. 验证签名，若通过则加密业务数据
            alt 签名有效
                Server-->>Client: 200 OK {"data": "...", "iv": "..."}
            else 签名/挑战无效
                Server-->>Client: 401 / 403 错误
            end
            Client->>Client: 6. 解密 data 获取最终信息
        </div>
    </div>

    <h2>任务步骤</h2>
    <ol>
        <li>向 <code><a href="/api/get-challenge" target="_blank">/api/get-challenge</a></code> 发起GET请求，获取加密后的挑战 <code>challenge</code> 和初始化向量 <code>iv</code>。</li>
        <li>使用预共享的AES密钥和获取到的 <code>iv</code> 解密 <code>challenge</code>，得到原始的 <code>nonce</code>。
            <ul><li>AES密钥: <code class="key">YakitVulinboxAES</code></li></ul>
        </li>
        <li>使用预共享的HMAC密钥，通过HMAC-SHA256算法计算 <code>nonce</code> 的签名。
            <ul><li>HMAC密钥: <code class="key">YakitVulinboxHMACKey-SIGNATURE</code></li></ul>
        </li>
        <li>将计算出的签名（Hex格式）放入 <code>X-Auth-Signature</code> 请求头，向 <code>/api/user/info</code> 发起GET请求。</li>
        <li>如果签名正确，你将收到加密的业务数据。再次使用AES密钥和响应中的新 <code>iv</code> 解密，即可看到最终的敏感信息。</li>
    </ol>
</body>
</html>
`
			writer.Write([]byte(html))
		},
	})

	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/hmac-sha256/login.html",
		Title: "前端验证签名(验签) 表单：HMAC-SHA256",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			unsafeTemplateRender(writer, request, cryptoJSlibTemplateHtml, map[string]any{
				"url":         `/crypto/sign/hmac/sha256/verify`,
				`extrakv`:     "username: jsonData.username, password: jsonData.password,",
				"title":       "HMAC-sha256 验签",
				"datafield":   "signature",
				"key":         `CryptoJS.enc.Utf8.parse("1234123412341234")`,
				"info":        "签名验证（又叫验签或签名）是验证请求参数是否被篡改的一种常见安全手段，验证签名方法主流的有两种，一种是 KEY+哈希算法，例如 HMAC-MD5 / HMAC-SHA256 等，本案例就是这种方法的典型案例。生成签名的规则为：username=*&password=*。在提交和验证的时候需要分别对提交数据进行处理，签名才可以使用和验证",
				"encrypt":     `CryptoJS.HmacSHA256(word, key.toString(CryptoJS.enc.Utf8)).toString();`,
				"decrypt":     `"";`,
				"jsonhandler": "`username=${jsonData.username}&password=${jsonData.password}`;",
			})
		},
	})
	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/hmac-sha256-rsa/login.html",
		Title: "前端验证签名(验签) 表单：先 HMAC-SHA256 再 RSA",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			unsafeTemplateRender(writer, request, cryptoJSlibTemplateHtml, map[string]any{
				"url":       `/crypto/sign/hmac/sha256/rsa/verify`,
				`extrakv`:   "username: jsonData.username, password: jsonData.password,",
				"title":     "HMAC-sha256 & RSA 验签",
				"datafield": "signature",
				"key":       "`" + string(pub) + "`",
				"info":      "签名验证（又叫验签或签名）是验证请求参数是否被篡改的一种常见安全手段，本例是 HMAC-SHA256 配合 RSA 签名的一种变种玩法。生成签名的规则为：username=*&password=*。在提交和验证的时候需要分别对提交数据进行处理，签名才可以使用和验证",
				"encrypt": `var sign = new JSEncrypt();
sign.setPrivateKey(key);
var signature = sign.sign(word, CryptoJS.SHA256, "sha256");
return signature;`,
				"decrypt":     `"";`,
				"jsonhandler": "`username=${jsonData.username}&password=${jsonData.password}`;",
			})
		},
	})

	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/cryptojs/aes/cbc/login.html",
		Title: "CryptoJS.AES(CBC)前端加密登陆表单",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			unsafeTemplateRender(writer, request, cryptoJSlibTemplateHtml, map[string]any{
				"url":         `/crypto/cryptojs/aes/cbc/login`,
				`extrakv`:     "",
				"title":       "CryptoJS.AES(CBC) 加密登陆",
				"datafield":   "password",
				"key":         `CryptoJS.enc.Utf8.parse("1234123412341234")`,
				"info":        "本例是典型的 AES-CBC 对用户提交的密码进行加密，防止密码明文传输。",
				"encrypt":     `CryptoJS.AES.encrypt(word, key, { iv: key, mode: CryptoJS.mode.CBC, padding: CryptoJS.pad.Pkcs7 }).toString()`,
				"decrypt":     `CryptoJS.AES.decrypt(word, key, { iv: key, mode: CryptoJS.mode.CBC, padding: CryptoJS.pad.Pkcs7 }).toString(CryptoJS.enc.Utf8)`,
				"jsonhandler": "`password=${jsonData.password}&username=${jsonData.username}`;",
			})
		},
	})
	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/cryptojs/aes/ecb/login.html",
		Title: "CryptoJS.AES(ECB)前端加密登陆表单",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			unsafeTemplateRender(writer, request, cryptoJSlibTemplateHtml, map[string]any{
				"url":         `/crypto/cryptojs/aes/ecb/login`,
				"title":       "CryptoJS.AES(ECB) 加密登陆",
				"datafield":   "password",
				"key":         `CryptoJS.enc.Utf8.parse('1234123412341234')`,
				"info":        "本例是典型的 AES-ECB 对用户提交的密码进行加密，防止密码明文传输。",
				"encrypt":     `CryptoJS.AES.encrypt(word, key, { mode: CryptoJS.mode.ECB, padding: CryptoJS.pad.Pkcs7 }).toString()`,
				"decrypt":     `CryptoJS.AES.decrypt(word, key, { mode: CryptoJS.mode.ECB, padding: CryptoJS.pad.Pkcs7 }).toString(CryptoJS.enc.Utf8)`,
				"jsonhandler": "`password=${jsonData.password}&username=${jsonData.username}`;",
			})
		},
	})
	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/cryptojs/aes/ecb/sql-injection-bypass-auth.html",
		Title: "CryptoJS.AES(ECB)被前端加密的SQL注入(Bypass认证)",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			unsafeTemplateRender(writer, request, cryptoJSlibTemplateHtml, map[string]any{
				"url":         `/crypto/cryptojs/aes/ecb/sql-injection-bypass-auth/login`,
				"title":       "CryptoJS.AES(ECB) 加密登陆",
				"datafield":   "username",
				"key":         `CryptoJS.enc.Utf8.parse('1234123412341234')`,
				"info":        "本例是典型的 AES-ECB 对用户提交的密码进行加密，但是 username 未经任何处理，导致了 SQL 注入漏洞。",
				"encrypt":     `CryptoJS.AES.encrypt(word, key, { mode: CryptoJS.mode.ECB, padding: CryptoJS.pad.Pkcs7 }).toString()`,
				"decrypt":     `CryptoJS.AES.decrypt(word, key, { mode: CryptoJS.mode.ECB, padding: CryptoJS.pad.Pkcs7 }).toString(CryptoJS.enc.Utf8)`,
				"jsonhandler": "`username=${jsonData.username}`;",
			})
		},
	})
	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/cryptojs/aes/ecb/sql-injection.html",
		Title: "CryptoJS.AES(ECB)被前端加密的SQL注入",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			unsafeTemplateRender(writer, request, cryptoJSlibTemplateHtml, map[string]any{
				"url":         `/crypto/cryptojs/aes/ecb/sql-injection/login`,
				"title":       "CryptoJS.AES(ECB) 加密登陆",
				"datafield":   "password",
				"key":         `CryptoJS.enc.Utf8.parse('1234123412341234')`,
				"info":        "本例是典型的 AES-ECB 对用户提交的密码进行加密，但是 username 未经任何处理，导致了 SQL 注入漏洞。",
				"encrypt":     `CryptoJS.AES.encrypt(word, key, { mode: CryptoJS.mode.ECB, padding: CryptoJS.pad.Pkcs7 }).toString()`,
				"decrypt":     `CryptoJS.AES.decrypt(word, key, { mode: CryptoJS.mode.ECB, padding: CryptoJS.pad.Pkcs7 }).toString(CryptoJS.enc.Utf8)`,
				"jsonhandler": "`password=${jsonData.password}&username=${jsonData.username}`;",
			})
		},
	})
	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/aes/ecb/login.html",
		Title: "AES-ECB 加密表单（附密码）",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Write(cryptoBasicHtml)
		},
	})
	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/rsa/login.html",
		Title: "RSA: 加密表单, 附密钥",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Write(cryptoRsaHtml)
		},
	})
	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/rsa/login_key_from_server.html",
		Title: "RSA: 加密表单服务器传输密钥",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Write(cryptoRsaKeyFromServerHtml)
		},
	})
	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/rsa/login_key_from_server_with_response_encrypted.html",
		Title: "RSA: 加密表单服务器传输密钥+响应加密",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Write(cryptoRsaKeyFromServerHtmlWithResponse)
		},
	})
	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/rsa/login_rsa_and_aes.html",
		Title: "前端RSA加密AES密钥, 服务器传输",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Write(cryptoRsaKeyAndAesHtml)
		},
	})
	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/sqlinjection/from-login-to-dump.html",
		Title: "SQL注入(从登陆到Dump数据库)",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			unsafeTemplateRender(writer, request, cryptoJSlibTemplateHtml, map[string]any{
				"url":         `/crypto/sqlinjection/login`,
				"title":       "SQL注入(从登陆到Dump数据库)",
				"datafield":   "password",
				"key":         `CryptoJS.enc.Utf8.parse('1234123412341234')`,
				"info":        "这是一个SQL注入漏洞，从登陆开始，最终可以dump数据库。这是一个综合性的漏洞，需要你自己探索。",
				"encrypt":     `CryptoJS.AES.encrypt(word, key, { mode: CryptoJS.mode.ECB, padding: CryptoJS.pad.Pkcs7 }).toString()`,
				"decrypt":     `CryptoJS.AES.decrypt(word, key, { mode: CryptoJS.mode.ECB, padding: CryptoJS.pad.Pkcs7 }).toString(CryptoJS.enc.Utf8)`,
				"jsonhandler": "`password=${jsonData.password}&username=${jsonData.username}`;",
			})
		},
	})

	addRouteWithVulInfo(cryptoGroup, &VulInfo{
		Path:  "/challenge-api-docs",
		Title: "动态挑战响应API靶场（20250623）",
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			html := `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>动态挑战-响应 API 安全靶场</title>
    <style>
        body { font-family: sans-serif; line-height: 1.6; padding: 20px; max-width: 800px; margin: auto; }
        h1, h2 { color: #333; }
        code { background-color: #f4f4f4; padding: 2px 6px; border-radius: 4px; }
        .workflow { text-align: center; margin: 20px 0; }
        .key { color: #c7254e; background-color: #f9f2f4; padding: 2px 4px; border-radius: 4px; }
    </style>
</head>
<body>
    <h1>动态挑战-响应 API 安全靶场</h1>
    <p>这是一个模拟真实世界高安全性API的靶场。它使用动态挑战-响应机制来防止重放攻击，并对业务数据进行加密传输。常规的Fuzzer很难成功请求此类型API。</p>
    
    <h2>交互流程</h2>
    <div class="workflow">
        <script src="https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.min.js"></script>
        <script>mermaid.initialize({startOnLoad:true});</script>
        <div class="mermaid">
        sequenceDiagram
            participant Client as 客户端
            participant Server as 服务器
            Client->>Server: 1. GET /api/get-challenge
            Server-->>Client: 返回 {"challenge": "...", "iv": "..."}
            Client->>Client: 2. 解密 challenge 获取 nonce<br/>3. 使用 HMAC-SHA256(nonce) 计算签名
            Client->>Server: 4. GET /api/user/info<br/>Header: X-Auth-Signature: &lt;signature&gt;
            Server->>Server: 5. 验证签名，若通过则加密业务数据
            alt 签名有效
                Server-->>Client: 200 OK {"data": "...", "iv": "..."}
            else 签名/挑战无效
                Server-->>Client: 401 / 403 错误
            end
            Client->>Client: 6. 解密 data 获取最终信息
        </div>
    </div>

    <h2>任务步骤</h2>
    <ol>
        <li>向 <code><a href="/api/get-challenge" target="_blank">/api/get-challenge</a></code> 发起GET请求，获取加密后的挑战 <code>challenge</code> 和初始化向量 <code>iv</code>。</li>
        <li>使用预共享的AES密钥和获取到的 <code>iv</code> 解密 <code>challenge</code>，得到原始的 <code>nonce</code>。
            <ul><li>AES密钥: <code class="key">YakitVulinboxAES</code></li></ul>
        </li>
        <li>使用预共享的HMAC密钥，通过HMAC-SHA256算法计算 <code>nonce</code> 的签名。
            <ul><li>HMAC密钥: <code class="key">YakitVulinboxHMACKey-SIGNATURE</code></li></ul>
        </li>
        <li>将计算出的签名（Hex格式）放入 <code>X-Auth-Signature</code> 请求头，向 <code>/api/user/info</code> 发起GET请求。</li>
        <li>如果签名正确，你将收到加密的业务数据。再次使用AES密钥和响应中的新 <code>iv</code> 解密，即可看到最终的敏感信息。</li>
    </ol>
</body>
</html>
`
			writer.Write([]byte(html))
		},
	})
}
