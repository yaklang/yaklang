package vulinbox

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

//go:embed vul_cryptojs_basic.html
var cryptoBasicHtml []byte

//go:embed vul_cryptojs_rsa.html
var cryptoRsaHtml []byte

//go:embed vul_cryptojs_rsa_keyfromserver.html
var cryptoRsaKeyFromServerHtml []byte

//go:embed vul_cryptojs_rsa_keyfromserver_withresponse.html
var cryptoRsaKeyFromServerHtmlWithResponse []byte

//go:embed vul_cryptojs_rsa_and_aes.html
var cryptoRsaKeyAndAesHtml []byte

func (s *VulinServer) registerCryptoJS() {
	r := s.router

	var (
		backupPass = []string{"admin", "123456", "admin123", "88888888", "666666"}
		pri, pub   []byte
		username   = "admin"
		password   = backupPass[rand.Intn(len(backupPass))]
	)

	log.Infof("frontend end crypto js user:pass = %v:%v", username, password)
	var isLogined = func(loginUser, loginPass string) bool {
		return loginUser == username && loginPass == password
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
	cryptoGroup := r.Name("高级场景前端加密").Subrouter()
	cryptoGroup.HandleFunc("/crypto/js/basic", func(writer http.ResponseWriter, request *http.Request) {
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

			var origin, decErr = codec.AESECBDecryptWithPKCS7Padding([]byte(key), []byte(encryptedBase64Decoded), []byte(iv))
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
			renderLoginSuccess(writer, username, password, []byte(
				`<br>`+
					`<pre>`+string(data)+`</pre> <br><br><br>	`+
					`<pre>`+handled+`</pre> <br><br>	`+
					`<pre>`+string(origin)+`</pre> <br><br>	`+
					`<pre>`+fmt.Sprint(err)+`</pre> <br><br>	`,
			))
			return
		}

		writer.WriteHeader(http.StatusMethodNotAllowed)
	}).Name("AES-ECB 加密表单（附密码）")
	cryptoGroup.HandleFunc("/crypto/js/rsa", func(writer http.ResponseWriter, request *http.Request) {
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
			renderLoginSuccess(writer, username, password, []byte(
				`<br>`+
					`<pre>`+string(data)+`</pre> <br><br><br>	`+
					`<pre>`+handled+`</pre> <br><br>	`+
					`<pre>`+string(origin)+`</pre> <br><br>	`+
					`<pre>`+fmt.Sprint(err)+`</pre> <br><br>	`,
			))
			return
		}

		writer.WriteHeader(http.StatusMethodNotAllowed)
	}).Name("RSA：加密表单，附密钥")
	cryptoGroup.HandleFunc("/crypto/js/rsa/fromserver", func(writer http.ResponseWriter, request *http.Request) {
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
			renderLoginSuccess(writer, username, password, []byte(
				`<br>`+
					`<pre>`+string(data)+`</pre> <br><br><br>	`+
					`<pre>`+handled+`</pre> <br><br>	`+
					`<pre>`+string(origin)+`</pre> <br><br>	`+
					`<pre>`+fmt.Sprint(err)+`</pre> <br><br>	`,
			))
			return
		}

		writer.WriteHeader(http.StatusMethodNotAllowed)
	})
	cryptoGroup.HandleFunc("/crypto/js/rsa/generator", func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(`{"ok": true, "publicKey": ` + strconv.Quote(string(pub)) + `, "privateKey": ` + strconv.Quote(string(pri)) + `}`))
	}).Name("postMessage 基础案例")
	cryptoGroup.HandleFunc("/crypto/js/rsa/fromserver/response", func(writer http.ResponseWriter, request *http.Request) {
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
	}).Name("RSA：加密表单服务器传输密钥+响应加密")
	cryptoGroup.HandleFunc("/crypto/js/rsa/fromserver/response/aes-gcm", func(writer http.ResponseWriter, request *http.Request) {
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
	}).Name("前端RSA加密AES密钥，服务器传输")
}
