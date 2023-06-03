package vulinbox

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

//go:embed vul_cryoto_sm_sm4.html
var cryptoSM4BasicHtml []byte

func (v *VulinServer) registerCryptoSM() {
	v.router.HandleFunc("/crypto/sm4", func(writer http.ResponseWriter, request *http.Request) {
		var params = make(map[string]interface{})

		var data, _ = utils.HttpDumpWithBody(request, true)
		params["packet"] = string(data)

		if request.Method == "GET" {
			results, err := mutate.FuzzTagExec(cryptoSM4BasicHtml, mutate.Fuzz_WithParams(params))
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
			encrypted := utils.MapGetString(params, "data")
			encryptedBase64Decoded, _ := codec.DecodeBase64(encrypted)

			spew.Dump(key, encryptedBase64Decoded)
			println("-----------------------")
			var origin, decErr = codec.SM4ECBDec([]byte(key), []byte(encryptedBase64Decoded), []byte(""))
			spew.Dump(origin, decErr)

			var handled string
			var raw, _ = json.MarshalIndent(map[string]any{
				"key":             utils.MapGetString(params, "key"),
				"key_hex_decoded": string(key),

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
}
