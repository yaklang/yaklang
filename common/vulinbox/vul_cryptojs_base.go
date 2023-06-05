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
	"net/http"
	"strconv"
	"strings"
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

func (v *VulinServer) registerCryptoJS() {
	r := v.router

	pri, pub, _ := tlsutils.GeneratePrivateAndPublicKeyPEMWithPrivateFormatter("pkcs8")
	r.HandleFunc("/crypto/js/basic", func(writer http.ResponseWriter, request *http.Request) {
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

			writer.Write([]byte(
				`<br>` +
					`<pre>` + string(data) + `</pre> <br><br><br>	` +
					`<pre>` + handled + `</pre> <br><br>	` +
					`<pre>` + string(origin) + `</pre> <br><br>	` +
					`<pre>` + fmt.Sprint(err) + `</pre> <br><br>	`,
			))
			return
		}

		writer.WriteHeader(http.StatusMethodNotAllowed)
	})
	r.HandleFunc("/crypto/js/rsa", func(writer http.ResponseWriter, request *http.Request) {
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

			writer.Write([]byte(
				`<br>` +
					`<pre>` + string(data) + `</pre> <br><br><br>	` +
					`<pre>` + handled + `</pre> <br><br>	` +
					`<pre>` + string(origin) + `</pre> <br><br>	` +
					`<pre>` + fmt.Sprint(err) + `</pre> <br><br>	`,
			))
			return
		}

		writer.WriteHeader(http.StatusMethodNotAllowed)
	})
	r.HandleFunc("/crypto/js/rsa/fromserver", func(writer http.ResponseWriter, request *http.Request) {
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

			writer.Write([]byte(
				`<br>` +
					`<pre>` + string(data) + `</pre> <br><br><br>	` +
					`<pre>` + handled + `</pre> <br><br>	` +
					`<pre>` + string(origin) + `</pre> <br><br>	` +
					`<pre>` + fmt.Sprint(err) + `</pre> <br><br>	`,
			))
			return
		}

		writer.WriteHeader(http.StatusMethodNotAllowed)
	})
	r.HandleFunc("/crypto/js/rsa/generator", func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(`{"ok": true, "publicKey": ` + strconv.Quote(string(pub)) + `, "privateKey": ` + strconv.Quote(string(pri)) + `}`))
	})
	r.HandleFunc("/crypto/js/rsa/fromserver/response", func(writer http.ResponseWriter, request *http.Request) {
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
			encryptedData, err := tlsutils.PemPkcsOAEPEncrypt(pub, `hackeddata=`+utils.RandSecret(10))
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
			writer.Write(raw)
			return
		}

		writer.WriteHeader(http.StatusMethodNotAllowed)
	})
	r.HandleFunc("/crypto/js/rsa/fromserver/response/aes-gcm", func(writer http.ResponseWriter, request *http.Request) {
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

			aesEncrypted, err := codec.AESGCMEncryptWithNonceSize12([]byte(key), rawResponseBody, []byte(iv))
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
	})
}
