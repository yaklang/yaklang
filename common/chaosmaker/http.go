package chaosmaker

import (
	"bytes"
	"fmt"
	uuid2 "github.com/satori/go.uuid"
	"net/http"
	"strconv"
	"strings"
	"yaklang.io/yaklang/common/filter"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/mutate"
	"yaklang.io/yaklang/common/suricata"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/lowhttp"
)

func init() {
	httpHandler := &chaosHandler{
		Generator: func(maker *ChaosMaker, chaosRule *ChaosMakerRule, originRule *suricata.Rule) chan *ChaosTraffic {
			if originRule == nil {
				return nil
			}
			if originRule.Protocol != "http" {
				return nil
			}

			httpPacket, err := mutate.NewFuzzHTTPRequest(`GET / HTTP/1.1
Host: www.example.com
`)
			if err != nil {
				return nil
			}
			_ = httpPacket
			config := originRule.ContentRuleConfig
			_ = config
			ch := make(chan *ChaosTraffic)

			var forBoth = config.HttpBaseSticky.HaveBeenSet()
			var forReq, forRsp bool
			if !forBoth {
				forReq = config.HttpRequestSticky.HaveBeenSet()
				forRsp = config.HttpResponseSticky.HaveBeenSet()
				if forReq && forBoth {
					forBoth = true
				}
			}
			if !forReq && !forRsp && !forBoth {
				forBoth = true
			}
			if forBoth {
				forReq = true
				forRsp = true
			}

			// flow control
			if config.Flow != nil {
				if (!config.Flow.ToClient && config.Flow.ToServer) || (!config.Flow.ToServer && config.Flow.ToClient) {
					forReq = config.Flow.ToServer
					forRsp = config.Flow.ToClient
				}
			}

			go func() {
				defer close(ch)

				f := filter.NewFilter()
				feedback := func(raw []byte) {
					if raw == nil || len(raw) <= 0 {
						return
					}

					hash := utils.CalcSha1(raw)
					if f.Exist(hash) {
						return
					}
					f.Insert(hash)

					if bytes.HasPrefix(raw, []byte("HTTP/")) {
						ch <- HttpResponseBytesToChaosTraffic(chaosRule, originRule, raw)
					} else {
						ch <- HttpRequestBytesToChaosTraffic(chaosRule, originRule, raw)
					}
				}

				log.Debugf("channel for %v is ready", originRule.Sid)

				if forReq {
					var extraRules []*suricata.ContentRule

					freqIns, _ := mutate.NewFuzzHTTPRequest(`GET /order/article?id=12 HTTP/1.1
Host: www.example.com
`)
					var freq mutate.FuzzHTTPRequestIf = freqIns
					freq = freq.FuzzHTTPHeader("Host", "www.ac.{{lower({{rs(3,4,2)}})}}.com")

					var extraBody []string
					var rules = config.ContentRules
				REQ_RULES:
					for _, rule := range rules {
						content := string(rule.Content)
						switch ret := rule.HttpBaseModifier; true {
						case ret.HttpCookie:
							freq.FuzzCookieRaw(content).FirstHTTPRequestBytes()
							freq.FuzzCookie("admin", content).FirstHTTPRequestBytes()
							feedback(freq.FuzzCookie("sid", content).FirstHTTPRequestBytes())
						case ret.HttpHeader:
							fallthrough
						case ret.HttpRawHeader:
							if k, v := lowhttp.SplitHTTPHeader(content); v != "" {
								feedback(freq.FuzzHTTPHeader(k, v).FirstHTTPRequestBytes())
							}
							feedback(freq.FuzzHTTPHeader("X-Test", content).FirstHTTPRequestBytes())
						}

						switch ret := rule.HttpRequestModifier; true {
						case ret.HttpHost:
							feedback(freq.FuzzHTTPHeader("Host", string(rule.Content)).FirstHTTPRequestBytes())
						case ret.HttpRawHost:
							feedback(freq.FuzzHTTPHeader("Host", string(rule.Content)).FirstHTTPRequestBytes())
						case ret.HttpUserAgent:
							feedback(freq.FuzzHTTPHeader("User-Agent", string(rule.Content)).FirstHTTPRequestBytes())
						case ret.HttpRawUri:
							feedback(freq.FuzzPathAppend("/" + utils.RandStringBytes(10) + content).FirstHTTPRequestBytes())
							feedback(freq.FuzzPathAppend("/" + content + "/").FirstHTTPRequestBytes())
							feedback(freq.FuzzPathAppend("/" + content).FirstHTTPRequestBytes())
							feedback(freq.FuzzPathAppend(content).FirstHTTPRequestBytes())
							feedback(freq.FuzzPath("/" + content).FirstHTTPRequestBytes())
							feedback(freq.FuzzPath(content + " " + "/").FirstHTTPRequestBytes())
							feedback(freq.FuzzPath("/" + utils.RandStringBytes(10) + content).FirstHTTPRequestBytes())
						case ret.HttpMethod:
							freq = freq.FuzzMethod(string(rule.Content))
							continue
						case ret.HttpUri:
							if rule.PCRE != "" {
								// 有正则，则以生成的正则为主
								extraRules = rule.PCREStringGenerator(5)
							}
							if strings.HasPrefix(content, ".") {
								path := utils.RandStringBytes(10) + content
								freq = freq.FuzzPathAppend(path, utils.RandStringBytes(3)+"/"+path)
								continue
							} else {
								path := utils.RandStringBytes(10) + content
								freq = freq.FuzzPathAppend(path, utils.RandStringBytes(3)+"/"+path, content, "/"+content)
								continue
							}
						}

						extraBody = append(extraBody, content)
					}
					if len(extraRules) > 0 {
						rules = extraRules
						extraRules = nil
						goto REQ_RULES
					}

					if len(extraBody) > 0 {
						var result []string
						concatStr := strings.Join(extraBody, "")
						if len(concatStr) <= 50 {
							result = append(result, concatStr)
							result = append(result, strings.Join(extraBody, " "))
							result = append(result, strings.Join(extraBody, ",{{rs(3)}}.{{ri(0,24)}}.{{ri(0,24)}}"))
						}
						result = append(result, extraBody...)
						freq = freq.FuzzPostRaw(result...)
					}
					res, _ := freq.Results()
					if res != nil {
						for _, result := range res {
							var raw, err = utils.HttpDumpWithBody(result, true)
							if err != nil {
								log.Error(err)
							}
							feedback(raw)
						}
					}
				}
				// http request/response
				if forRsp {
					var (
						httpVersion  = "HTTP/1.1"
						extraHeader  = make(http.Header)
						code         = "200"
						status       = "OK"
						extraContent []string
						bodyJson     = `{"ami": "ok", "reason": "ok", "uid": "1-2-3-4-5", "uuid": ` + uuid2.NewV4().String() + `}`
						htmlBody     = `<html>
    <body>
        <div>
Hello


    </div> <!-- /container -->
<div id = 111` + utils.RandSecret(13) + `></div>

  </body>
</html>`
						server   = "nginx"
						location = ""
					)
					_ = htmlBody
					_ = bodyJson
					var extraRules []*suricata.ContentRule
					rules := config.ContentRules
				WRITE_RULES:
					for _, rule := range rules {
						if rule == nil || len(rule.Content) <= 0 {
							continue
						}
						content := string(rule.Content)

						switch ret := rule.HttpBaseModifier; true {
						case ret.HttpCookie:
							content = strings.ReplaceAll(content, "Set-Cookie: ", "")
							content = strings.ReplaceAll(content, "Set-Cookie:", "")
							content = strings.ReplaceAll(content, "Cookie:", "")
							extraHeader.Add("Set-Cookie", content)
							continue
						case ret.HttpHeader:
							fallthrough
						case ret.HttpRawHeader:
							if k, v := lowhttp.SplitHTTPHeader(content); v != "" {
								extraHeader.Add(k, v)
							} else {
								extraHeader.Add("Content-Type", content)
							}
							continue
						}

						if ret := rule.PCREStringGenerator(5); ret != nil {
							extraRules = append(extraRules, ret...)
						}

						switch ret := rule.HttpResponseModifier; true {
						case ret.HttpLocation:
							location = `{{list(|/|/admin/|/{{(rs(5,5,2))/|http://|https://|login/}})}}` + location
							continue
						case ret.HttpServerBody:
							if rule.Negative {
								bodyJson = strings.ReplaceAll(bodyJson, content, "")
								htmlBody = strings.ReplaceAll(htmlBody, content, "")
							} else {
								bodyJson = content + bodyJson
								htmlBody = content + htmlBody
								extraContent = append(extraContent, content)
							}
							continue
						case ret.HttpServer:
							server = content + `{{list(/||1.3.1|1.2.3|{{randstr(4,5,3)}})}}`
							if strings.HasPrefix(server, "Server: ") {
								server = server[8:]
							}
							continue
						case ret.HttpStatCode:
							code = content
							var codeInt, _ = strconv.Atoi(code)
							if codeInt > 0 {
								status = http.StatusText(codeInt)
							}
							continue
						case ret.HttpStatMsg:
							status = content
							continue
						}

						if !rule.Negative {
							bodyJson += content
							htmlBody += content
							extraContent = append(extraContent, content)
						}
					}

					if extraRules != nil || len(extraRules) > 0 {
						rules = extraRules
						extraRules = nil
						goto WRITE_RULES
					}

					if location != "" {
						code = "302"
						status = http.StatusText(302)
					}
					var headerExtraLine []string
					var (
						ignoreCT     = false
						ignoreServer = false
					)
					for k, v := range extraHeader {
						lowerKey := strings.ToLower(k)
						if strings.Contains(lowerKey, "content-type") {
							ignoreCT = true
						}
						if strings.Contains(lowerKey, "server") {
							ignoreServer = true
						}
						for _, v1 := range v {
							headerExtraLine = append(headerExtraLine, k+": "+v1)
						}
					}

					var lines []string
					lines = append(lines, fmt.Sprintf("%v %v %v", httpVersion, code, status))
					lines = append(lines, headerExtraLine...)
					if !ignoreCT {
						lines = append(lines, fmt.Sprintf("Content-Type: %v", "application/json"))
					}
					if !ignoreServer {
						lines = append(lines, fmt.Sprintf("Server: %v", server))
					}
					var header = strings.Join(lines, "\r\n")
					var jsonPacket = lowhttp.ReplaceHTTPPacketBody([]byte(header), []byte(bodyJson), false)
					for _, i := range mutate.MutateQuick(jsonPacket) {
						feedback([]byte(i))
					}
					lines = []string{}
					lines = append(lines, fmt.Sprintf("%v %v %v", httpVersion, code, status))
					lines = append(lines, headerExtraLine...)
					if !ignoreCT {
						lines = append(lines, fmt.Sprintf("Content-Type: %v", "text/html"))
					}
					if !ignoreServer {
						lines = append(lines, fmt.Sprintf("Server: %v", server))
					}
					header = strings.Join(lines, "\r\n")
					var htmlpacket = lowhttp.ReplaceHTTPPacketBody([]byte(header), []byte(htmlBody), false)
					for _, i := range mutate.MutateQuick(htmlpacket) {
						feedback([]byte(i))
					}
					lines = []string{}
					lines = append(lines, fmt.Sprintf("%v %v %v", httpVersion, code, status))
					lines = append(lines, headerExtraLine...)
					if !ignoreCT {
						lines = append(lines, fmt.Sprintf("Content-Type: %v", "application/octet-stream"))
					}
					if !ignoreServer {
						lines = append(lines, fmt.Sprintf("Server: %v", server))
					}
					header = strings.Join(lines, "\r\n")
					for _, body := range extraContent {
						var finPacket = lowhttp.ReplaceHTTPPacketBody([]byte(header), []byte(body), false)
						for _, i := range mutate.MutateQuick(finPacket) {
							feedback([]byte(i))
						}
					}
				}
			}()
			return ch
		},
		MatchBytes: nil,
	}
	chaosMap.Store("suricata-http", httpHandler)
}

func HttpRequestBytesToChaosTraffic(mainRule *ChaosMakerRule, rule *suricata.Rule, req []byte) *ChaosTraffic {
	return &ChaosTraffic{
		ChaosRule:    mainRule,
		SuricataRule: rule,
		HttpRequest:  req,
	}
}

func HttpResponseBytesToChaosTraffic(mainRule *ChaosMakerRule, rule *suricata.Rule, rsp []byte) *ChaosTraffic {
	return &ChaosTraffic{
		ChaosRule:    mainRule,
		SuricataRule: rule,
		HttpResponse: rsp,
	}
}
