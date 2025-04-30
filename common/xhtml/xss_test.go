package xhtml

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const reqRaw = `GET /xss/example7.php?name=hacker HTTP/1.1
Host: 192.168.101.211:8990
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9
Accept-Encoding: gzip, deflate
Accept-Language: zh-CN,zh;q=0.9
Cache-Control: max-age=0
Cookie: sidebarStatus=1; vue_admin_template_token=eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJ1c2VyX2lkIjoxLCJ1c2VybmFtZSI6ImFkbWluIiwiZXhwIjoxNjUzMTI0NDg4LCJlbWFpbCI6IiJ9.hivEryBUofHU0AxMoaaorzv5M3N0Z4Ghagy3_3hhU4k; PHPSESSID=4165314a839af32486261f961485b3fc; security=low; ADMINCONSOLESESSION=BVRdvT7LJzCS0YrkrFHV7psMLG0LKWcvXvhDyYGygmb19vkcc6Gn!-737481101; JSESSIONID=60F1BFC343BC97C8D0E15EFA336F3FE7
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/102.0.5005.61 Safari/537.36

`
const testBody = `<html>
    <body>
        <div>
Hello 
hacker1


    </div> <!-- /container -->
<div id = hacker123></div>

  </body>
</html>
`
const testBody2 = `<html>
    <body>
        <div>
Hello 
hacker2


    </div> <!-- /container -->
<div id = "hacker123"></div>

  </body>
</html>
`

type XssResult struct {
	diffInfo *DiffInfo
	Payload  string
}
type Environment struct {
	position int
	quote    string
}

//func TestXss(t *testing.T) {
//	//var xssFuzzRes []XssResult
//	//testParam := map[string]string{"name": testStr}
//	request := NewXssFuzz(reqRaw)
//	params, err := request.GetParams()
//	if err != nil {
//		return
//	}
//	for i := 0; i < len(params); i++ {
//		params_copy := params
//		randStart := html.RandSafeString(5)
//		randEnd := html.RandSafeString(5)
//		randstr := randStart + randEnd
//		resp1, err := request.Request(params_copy[i].Key, randstr)
//		if err != nil {
//			fmt.Printf("fuzz param error: %v", err)
//			return
//		}
//		rspFIndex := resp1
//		indexN := []int{}
//		randstrL := len(randstr)
//		for {
//			n := strings.Index(rspFIndex, randstr)
//			if n == -1 {
//				break
//			}
//			indexN = append(indexN, n)
//			rspFIndex = rspFIndex[n+randstrL:]
//		}
//
//		//reflectionsNum := strings.Count(resp1, testParam)
//		payload := []string{}
//		checkDisableChar := []string{}
//		ends := []string{}
//		//checkType := []string{}
//		//environment_details := []Environment{}
//
//		//根据响应包，判断对哪些字符进行过滤检测
//		html.Walker(resp1, func(node *html.Node) {
//
//			if utils.MatchAllOfGlob(node.Data, fmt.Sprintf("*%s*", randstr)) {
//				if !(node.Type == html.TextNode && node.Parent.Type == html.ElementNode) {
//					return
//				}
//				parentNodeTag := node.Parent.Data
//				if utils.StringArrayContains([]string{"style", "template", "textarea", "title", "noembed", "noscript"}, parentNodeTag) {
//					return
//				}
//				if parentNodeTag == "script" {
//					log.Info("script标签发现回显")
//					ends = append(ends, "//")
//					AddElement2Set(&checkDisableChar, "</scRipT/>")
//					re, err := regexp.Compile(fmt.Sprintf("%s.*", randstr))
//					if err != nil {
//						return
//					}
//					subs := re.FindAllStringSubmatch(node.Data, -1)
//
//					dangerChar1 := []string{"/", "'", "`", "\""}
//					dangerChar2 := []string{")", "]", "}"}
//					for _, sub := range subs {
//						substr := sub[0]
//						for i := 0; i < len(substr); i++ {
//							c := string(substr[i])
//							if utils.StringArrayContains(dangerChar1, c) && !html.IsEscaped(substr[:i]) {
//								//n := strings.Index(node.Data, substr)
//								//environment_details = append(environment_details, Environment{position: n, quote: c})
//								html.AddElement2Set(&checkDisableChar, c)
//							} else if utils.StringArrayContains(dangerChar2, c) && !html.IsEscaped(substr[:i]) {
//								break
//							}
//						}
//
//					}
//
//				} else if parentNodeTag == "comment" {
//					log.Info("注释发现回显")
//					html.AddElement2Set(&checkDisableChar, "-->")
//
//				} else {
//					log.Info("文字标签发现回显")
//					html.AddElement2Set(&checkDisableChar, "<")
//					html.AddElement2Set(&checkDisableChar, ">")
//				}
//
//			} else if node.Type == html.ElementNode {
//				for _, attr := range node.Attr {
//					if utils.MatchAllOfGlob(attr.Val, fmt.Sprintf("*%s*", randstr)) {
//						log.Infof("%s标签的%s属性发现回显", node.Data, attr.Key)
//						html.AddElement2Set(&checkDisableChar, "\"")
//						key := attr.Key
//						if strings.HasPrefix(strings.ToLower(key), "On") {
//							//addFuzzType(AttrOnxxx)
//						} else if strings.ToLower(key) == "href" {
//							//addFuzzType(AttrHref)
//						} else if strings.ToLower(key) == "srcdoc" {
//							html.AddElement2Set(&checkDisableChar, "&lt;")
//							html.AddElement2Set(&checkDisableChar, "&gt;")
//						}
//					}
//				}
//			}
//		})
//		payloads := []string{}
//		okDisableChar := []string{}
//		onoDisableChar := []string{}
//		for _, ch := range checkDisableChar {
//			checkParamValue := randStart + ch + randEnd
//			//params_copy[i].TestValude = []string{checkParamValue}
//			rsp, err := request.Request(params_copy[i].Key, checkParamValue)
//			if err != nil {
//				log.Errorf("fuzz param error: %v", err)
//				return
//			}
//			rspFIndex := rsp
//			isOk := false
//			for {
//				n, btStr := html.MatchBetween(rspFIndex, randStart, randEnd, 50)
//				rspFIndex = rspFIndex[n+len(randStart+btStr+randEnd):]
//				if n == -1 {
//					break
//				}
//				if btStr == ch {
//					isOk = true
//					switch ch {
//					case ">":
//						ends = append(ends, ">")
//					case "</scRipT/>":
//						ends = append(ends, html.RandomUpperAndLower("</script/>"))
//					case "'":
//						for _, jFilling := range html.jFillings {
//							for _, function := range html.functions {
//								if strings.Index(function, "=") != -1 {
//									function = "(" + function + ")"
//								}
//								payload := "'" + jFilling + function + "//"
//								payloads = append(payloads, payload)
//							}
//						}
//					}
//				} else {
//					log.Infof("字符%s被过滤为%s", ch, btStr)
//				}
//
//			}
//			if isOk {
//				okDisableChar = append(okDisableChar, ch)
//
//			} else {
//				onoDisableChar = append(onoDisableChar, ch)
//			}
//		}
//		paramFilter := []string{"submit"}
//		//payloads = GenPayload(randstr, ends)
//		for _, payload := range payloads {
//			for i := 0; i < len(params); i++ {
//				if utils.StringArrayContains(paramFilter, params[i].Key) {
//				}
//				params_copy := params
//				rsp, err := request.Request(params_copy[i].Key, payload)
//				if err != nil {
//					return
//				}
//				result, err := CompareHtml(resp1, rsp)
//				for _, r := range result {
//					fmt.Printf("Payload: %s\nXpath: %s\nReason: %s\nOrigin: %s\nFuzz: %s\n", payload, r.XpathPos, r.Reason, r.OriginRaw, r.FuzzRaw)
//				}
//			}
//
//		}
//		println(strings.Join(payload, "\n"))
//	}
//	//
//	//resp2, err := request.Request(params_copy)
//	//if err != nil {
//	//	fmt.Printf("fuzz param error: %v", err)
//	//	return
//	//}
//	//expectDiff, err := CompareHtml(resp1, resp2)
//
//	//fuzzedText := false
//	//fuzzedAttr := false
//	//fuzzText := func(node *html.Node, nodeType NodeType) {
//	//	pnode := node.Parent
//	//	var payload string
//	//	switch nodeType {
//	//	case Text:
//	//		var parentTag string
//	//		if pnode.Type != html.ElementNode {
//	//			return
//	//		}
//	//		parentTag = pnode.Data
//	//		if strings.ToLower(parentTag) == "script" {
//	//			payload = fmt.Sprintf("hacke1</%s>123<%s>", parentTag, parentTag)
//	//		} else {
//	//			payload = fmt.Sprintf("hacke1</%s>123<%s>", parentTag, parentTag)
//	//		}
//	//	case Attr:
//	//		var tag string
//	//		if node.Type != html.ElementNode {
//	//			return
//	//		}
//	//		tag = node.Data
//	//		payload = fmt.Sprintf("\">123</%s><%s id=\"", tag, tag)
//	//	}
//	//	rsp, err := fuzzParam(reqRaw, payload)
//	//	if err != nil {
//	//		fmt.Printf("fuzz param error: %v", err)
//	//		return
//	//	}
//	//	result, err := CompareHtml(resp1, rsp)
//	//	for _, res := range result {
//	//		xssFuzzRes = append(xssFuzzRes, XssResult{diffInfo: res, Payload: payload})
//	//	}
//	//}
//	//fuzzTypes := []string{}
//	//addFuzzType := func(t string) {
//	//	if !utils.StringArrayContains(fuzzTypes, t) {
//	//		fuzzTypes = append(fuzzTypes, t)
//	//	}
//	//
//	//}
//	//matchParam := fmt.Sprintf("*%s*", "param1")
//	//
//	//for _, xss := range xssFuzzRes {
//	//	r := xss.diffInfo
//	//	isIn := false
//	//	for _, diff := range expectDiff {
//	//		if r.XpathPos == diff.XpathPos {
//	//			isIn = true
//	//		}
//	//	}
//	//	if !isIn {
//	//		fmt.Printf("Payload: %s\nXpath: %s\nReason: %s\nOrigin: %s\nFuzz: %s\n", xss.Payload, r.XpathPos, r.Reason, r.OriginRaw, r.FuzzRaw)
//	//	}
//
//	//}
//}

func fuzzParam(raw interface{}, param string) ([]byte, error) {
	reqRaw := utils.InterfaceToString(raw)
	freq, err := mutate.NewFuzzHTTPRequest(reqRaw)
	if err != nil {
		return nil, err
	}
	param = ("name=" + param + "&submit=%E6%8F%90%E4%BA%A4")
	resps, err := freq.FuzzPostRaw("name", param).Exec()
	//resps, err := freq.FuzzGetParams("name", param).Exec()
	defaultReq := <-resps
	defaultBody, err := lowhttp.ExtractBodyFromHTTPResponseRaw(defaultReq.ResponseRaw)
	if err != nil {
		return nil, err
	}
	return defaultBody, nil
}
func TestWalk(t *testing.T) {
	result := FindNodeFromHtml(testBody, "*hacker*")
	for _, path := range result {
		println(path)
	}
}
func TestCompareHtml(t *testing.T) {

	defaultParam := "hacker1"
	fuzzXssPayload := "123"
	freq, err := mutate.NewFuzzHTTPRequest(reqRaw)
	if err != nil {
		return
	}

	resps, err := freq.FuzzGetParams("name", defaultParam).Exec()
	defaultReq := <-resps
	resps, err = freq.FuzzGetParams("name", fuzzXssPayload).Exec()
	fuzzReq := <-resps
	if err != nil {
		return
	}
	defaultBody, err := lowhttp.ExtractBodyFromHTTPResponseRaw(defaultReq.ResponseRaw)
	fuzzBody, err := lowhttp.ExtractBodyFromHTTPResponseRaw(fuzzReq.ResponseRaw)
	res, err := CompareHtml(defaultBody, fuzzBody)
	if err != nil {
		return
	}
	for _, r := range res {
		fmt.Printf("Xpath: %s\nReason: %s\nOrigin: %s\nFuzz: %s\n", r.XpathPos, r.Reason, r.OriginRaw, r.FuzzRaw)
		if r.Type == Tag || r.Type == Attr {

		}
	}
}

//func TestXssDetect(t *testing.T) {
//	req := "<raw request>"
//	freq, err := mutate.NewFuzzHTTPRequest(req)
//	if err != nil {
//		println("new HTTPRequest error: %v", err)
//		return
//	}
//	params := freq.GetCommonParams() // 包含post json、post form、get参数、cookie参数（会自动过滤PHPSESSID、_ga、_gid等参数）
//	for _, param := range params {
//		fmt.Printf("key: %s, value: %s, postion: %s\n", param.Name(), param.Value(), param.PositionVerbose())
//		randStr := utils.RandStringBytes(5)
//		resp, err := param.Fuzz(randStr).Exec()
//		if err != nil {
//			println("Fuzz param %s error: %v", param.Name(), err)
//			return
//		}
//		rspo := <-resp
//		body, err := lowhttp.ExtractBodyFromHTTPResponseRaw(rspo.ResponseRaw)
//		if err != nil {
//			println("Get response body error: %v", err)
//			return
//		}
//		matchNodes := FindNodeFromHtml(body, randStr)
//		payloads := []string{}
//		for _, matchNode := range matchNodes {
//			fmt.Printf("Echo xpath postion: %s", matchNode.Xpath)
//			if matchNode.IsText() {
//				if matchNode.MatchText == "" && matchNode.TagName == "script" {
//
//				}
//				if matchNode.TagName == "script" {
//					//例：<script>a = '<参数>';</script>
//					//payload := fmt.Sprintf("';alert('Hello');'", matchNode.TagName)
//					payload := ""
//					payloads = append(payloads, payload)
//				} else {
//					//例：<div><参数></div>
//					payload := fmt.Sprintf("</%s>Hello<%s>", matchNode.TagName, matchNode.TagName)
//					payloads = append(payloads, payload)
//				}
//			} else if matchNode.IsAttr() {
//				//例：<div id="<参数>"></div>
//				payload := fmt.Sprintf("\"></%s>Hello<%s %s=\"%s", matchNode.TagName, matchNode.TagName, matchNode.Key, matchNode.Value)
//				payloads = append(payloads, payload)
//			} else if matchNode.IsCOMMENT() {
//				//例：<!-- <参数> -->
//				payload := fmt.Sprintf("-->Hello<!--")
//				payloads = append(payloads, payload)
//			}
//		}
//		//diffs, err := CompareHtml(rawHtml, body)
//		//for _, diff := range diffs {
//		//	if diff.Type == Tag {
//		//		fmt.Sprintf("Found xss, XpathPos: %s, Reason: %s", diff.XpathPos, diff.Reason)
//		//	}
//		//}
//		filterPayloadByChar := func(payloads []string, chars []string) []string {
//			detectChars := []string{}
//			for _, payload := range payloads {
//				for _, dangerousChar := range chars {
//					if utils.MatchAllOfGlob(payload, fmt.Sprintf("*%s*", dangerousChar)) {
//						detectChars = append(detectChars, dangerousChar)
//					}
//				}
//			}
//			return detectChars
//		}
//
//		dangerousChars := []string{"<", ">", "/", "\\", "'", "\""}
//		detectChars := filterPayloadByChar(payloads, dangerousChars)
//		for _, payload := range payloads {
//			for _, dangerousChar := range dangerousChars {
//				if utils.MatchAllOfGlob(payload, fmt.Sprintf("*%s*", dangerousChar)) {
//					detectChars = append(detectChars, dangerousChar)
//				}
//			}
//		}
//		detectStr := randStr + strings.Join(detectChars, randStr) + randStr
//		resp, err = param.Fuzz(detectStr).Exec()
//		if err != nil {
//			println("Fuzz param %s error: %v", param.Name(), err)
//			return
//		}
//		rspo = <-resp
//		body, err = lowhttp.ExtractBodyFromHTTPResponseRaw(rspo.ResponseRaw)
//		if err != nil {
//			println("Get response body error: %v", err)
//			return
//		}
//		randStrFromIndex := body
//		passChars := []string{}
//		i := 0
//		for {
//			n, btChar := MatchBetween(randStrFromIndex, randStr, randStr, 50)
//			if n == -1 {
//				break
//			}
//			if i >= len(dangerousChars) {
//				break
//			}
//			if dangerousChars[i] == btChar {
//				passChars = append(passChars, btChar)
//			} else {
//				fmt.Printf("Found characters to be filtered: %s->%s", dangerousChars[i], btChar)
//			}
//			randStrFromIndex = randStrFromIndex[n+len(randStr):]
//			i += 1
//		}
//		newPayloads := []string{}
//		for _, payload := range payloads {
//			for _, filterChar := range passChars {
//				if utils.MatchAllOfGlob(payload, fmt.Sprintf("*%s*", filterChar)) {
//					newPayloads = append(newPayloads, payload)
//				}
//			}
//		}
//	}
//
//}
