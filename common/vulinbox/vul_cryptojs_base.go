package vulinbox

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
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

func (v *VulinServer) registerCryptoJS() {
	r := v.router

	pri, pub, _ := tlsutils.GeneratePrivateAndPublicKeyPEM()
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

}
