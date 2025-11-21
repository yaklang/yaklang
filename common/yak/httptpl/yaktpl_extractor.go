package httptpl

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/url"
	"reflect"
	"regexp"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/itchyny/gojq"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/htmlquery"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func NewExtractorFromGRPCModel(m *ypb.HTTPResponseExtractor) *YakExtractor {
	return &YakExtractor{
		Name:             m.GetName(),
		Type:             m.GetType(),
		Scope:            m.GetScope(),
		Groups:           m.GetGroups(),
		RegexpMatchGroup: utils.Int64SliceToIntSlice(m.GetRegexpMatchGroup()),
		XPathAttribute:   m.GetXPathAttribute(),
	}
}

type YakExtractor struct {
	Id   int
	Name string // name or index

	// regexp
	// json
	// kval
	// xpath
	// nuclei-dsl
	Type string

	// body
	// header
	// all
	Scope                string // header body all
	Groups               []string
	RegexpMatchGroup     []int
	RegexpMatchGroupName []string
	XPathAttribute       string
}

// group1 for key
// group2 for value

func (y *YakExtractor) Execute(rsp []byte, previous ...map[string]any) (map[string]any, error) {
	return y.ExecuteWithRequest(rsp, nil, false, previous...)
}

func (y *YakExtractor) ExecuteWithRequest(rsp []byte, req []byte, isHttps bool, previous ...map[string]any) (map[string]any, error) {
	tag := y.Name
	if tag == "" {
		tag = "data"
	}
	results := []string{}
	addResult := func(result any) {
		results = append(results, utils.InterfaceToString(result))
	}
	resultsMap := make(map[string]any)

	var material string
	switch strings.TrimSpace(strings.ToLower(y.Scope)) {
	case "body":
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp)
		material = string(body)
	case "header":
		header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp)
		material = header
	case "request_header":
		if len(req) > 0 {
			header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req)
			material = header
		} else {
			material = ""
		}
	case "request_body":
		if len(req) > 0 {
			_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req)
			material = string(body)
		} else {
			material = ""
		}
	case "request_raw":
		if len(req) > 0 {
			material = string(req)
		} else {
			material = ""
		}
	case "request_url":
		if len(req) > 0 {
			if reqUrl, err := lowhttp.ExtractURLFromHTTPRequestRaw(req, isHttps); err == nil {
				material = reqUrl.String()
			} else {
				material = ""
			}
		} else {
			material = ""
		}
	default:
		material = string(rsp)
	}

	t := strings.TrimSpace(strings.ToLower(y.Type))
	switch t {
	case "regex":
		for _, group := range y.Groups {
			if group != "" {
				r, err := regexp.Compile(group)
				if err != nil {
					log.Errorf("compile[%v] failed: %v", group, err)
					continue
				}

				var regexpMatchGroup []int
				if len(y.RegexpMatchGroupName) > 0 {
					for _, groupName := range y.RegexpMatchGroupName {
						regexpMatchGroup = append(regexpMatchGroup, r.SubexpIndex(groupName))
					}
				}
				regexpMatchGroup = lo.Uniq(append(regexpMatchGroup, y.RegexpMatchGroup...))
				if len(regexpMatchGroup) == 0 {
					regexpMatchGroup = []int{0}
				}

				// just append result which in match group
				for _, res := range r.FindAllStringSubmatch(material, -1) {
					for _, i := range regexpMatchGroup {
						if i < len(res) {
							addResult(res[i])
						}
					}
				}
			}
		}
	case "kv", "key-value", "kval":
		var kvResult map[string]any
		scope := strings.TrimSpace(strings.ToLower(y.Scope))
		if scope == "body" || scope == "request_body" {
			kvResult = ExtractKValFromBody(material)
		} else {
			kvResult = ExtractKValFromResponse([]byte(material))
		}
		for _, group := range y.Groups {
			if v, ok := kvResult[group]; ok {
				v1 := v.([]interface{})
				for _, v2 := range v1 {
					addResult(v2)
				}
			}
		}
	case "json", "jq":
		for _, group := range y.Groups {
			if group != "" {
				query, err := gojq.Parse(group)
				if err != nil {
					log.Errorf("parse jq query[%v] failed: %s", group, err)
					continue
				}
				var obj interface{}
				if err := json.Unmarshal([]byte(material), &obj); err != nil {
					log.Debugf("parse json failed: %s", err)
					continue
				}
				iter := query.Run(obj)
				for {
					v, ok := iter.Next()
					if !ok {
						break
					}
					if err, ok := v.(error); ok {
						log.Warnf("jq query[%v] failed: %s", group, err)
						continue
					}
					addResult(v)
				}
			}
		}
	case "xpath":
		isXml := strings.HasPrefix(strings.ToLower(strings.TrimSpace(material)), "<?xml")
	TRYXML:
		if isXml {
			doc, err := xmlquery.Parse(strings.NewReader(material))
			if err != nil {
				log.Warnf("parse xml failed: %s", err)
				return nil, utils.Errorf("xmlquery.Parse failed: %s", err)
			}
			for _, group := range y.Groups {
				if group != "" {
					nodes, err := xmlquery.QueryAll(doc, group)
					if err != nil {
						log.Errorf("xpath[%v] failed: %s", group, err)
						continue
					}
					for _, node := range nodes {
						addResult(node.InnerText())
					}
				}
			}
		} else {
			isXml = true
			doc, err := htmlquery.Parse(strings.NewReader(material))
			if err != nil {
				log.Warnf("parse html failed: %s", err)
				return nil, utils.Errorf("htmlquery.Parse failed: %s", err)
			}
			for _, group := range y.Groups {
				if group != "" {
					nodes, err := htmlquery.QueryAll(doc, group)
					if err != nil {
						log.Errorf("xpath[%v] failed: %s", group, err)
						continue
					}
					for _, node := range nodes {
						if y.XPathAttribute != "" {
							for _, attr := range node.Attr {
								if attr.Key == y.XPathAttribute {
									addResult(attr.Val)
									isXml = false
								}
							}
						} else {
							addResult(htmlquery.InnerText(node))
							isXml = false
						}
					}
				}
			}
			if isXml {
				goto TRYXML
			}
		}
	case "nuclei-dsl", "nuclei", "dsl":
		box := NewNucleiDSLYakSandbox()
		// Load all built-in variables from response (including status_code, headers, etc.)
		previousMap := LoadVarFromRawResponseWithRequest(rsp, req, 0, isHttps)

		// Merge previous extraction results
		for _, p := range previous {
			for k, v := range p {
				switch reflect.TypeOf(v).Kind() {
				case reflect.Slice, reflect.Array:
					previousMap[k] = strings.Join(utils.InterfaceToStringSlice(v), ",")
				default:
					previousMap[k] = v
				}
			}
		}

		// Also keep the old variable names for backward compatibility
		header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp)
		previousMap["body_1"] = body
		previousMap["header_1"] = header
		previousMap["raw_1"] = string(rsp)
		previousMap["response"] = string(rsp)
		previousMap["response_1"] = string(rsp)

		for _, group := range y.Groups {
			if group != "" {
				data, err := box.Execute(group, previousMap)
				if err != nil {
					continue
				}
				addResult(data)
			}
		}
	default:
		return nil, utils.Errorf("unknown extractor type: %s", t)
	}

	if len(results) == 0 {
		resultsMap[tag] = nil // extract empty string are different from not being extracted
	} else if len(results) == 1 {
		resultsMap[tag] = results[0]
	} else {
		resultsMap[tag] = results
	}

	return resultsMap, nil
}

var (
	extractFromJSONValue = regexp.MustCompile(`("[^":]+")\s*?:\s*(("([^"\n])+")|([^"]\S+[^"\s,]))`)
	extractFromEqValue   = regexp.MustCompile(`(?i)([_a-z][^=\s]*)=(([^=\s&,"]+)|("[^"]+"))`)
)

func ExtractKValFromBody(body string) map[string]interface{} {
	return extractKVal([]byte(body), false)
}

func ExtractKValFromResponse(rsp []byte) map[string]interface{} {
	return extractKVal(rsp, true)
}

func extractKVal(rsp []byte, shouldSplit bool) map[string]interface{} {
	results := make(map[string]interface{})
	addResult := func(k, v string) {
		if _, ok := results[k]; !ok {
			results[k] = make([]interface{}, 0)
		}
		results[k] = append(results[k].([]interface{}), v)
	}
	var body []byte
	if shouldSplit {
		_, body = lowhttp.SplitHTTPPacket(rsp, nil, func(proto string, code int, codeMsg string) error {
			addResult("proto", proto)
			addResult("status_code", fmt.Sprintf("%d", code))
			return nil
		}, func(line string) string {
			k, v := lowhttp.SplitHTTPHeader(line)
			originKey := k
			k = strings.ReplaceAll(strings.ToLower(k), "-", "_")
			addResult(k, v)
			addResult(originKey, v)
			if k == `content_type` {
				ct, params, err := mime.ParseMediaType(v)
				if err != nil {
					return line
				}
				addResult(`content_type`, ct)
				for k, v := range params {
					addResult(k, v)
				}
			} else {
				kvs := strings.Split(v, ";")
				for _, kv := range kvs {
					kv = strings.TrimSpace(kv)
					key, value, ok := strings.Cut(kv, "=")
					if ok {
						decoded, err := url.QueryUnescape(value)
						if err != nil {
							addResult(key, value)
						} else {
							addResult(key, decoded)
						}
					}
				}
			}
			return line
		})
	} else {
		body = rsp
	}

	var processJSON func(jsonString string, depth int)
	processJSON = func(jsonString string, depth int) {
		if depth > 3 {
			return
		}
		var ok bool
		if jsonString, ok = utils.IsJSON(jsonString); ok {
			result := gjson.Parse(jsonString)
			result.ForEach(func(key, value gjson.Result) bool {
				addResult(key.String(), value.String())
				if value.Type == gjson.JSON {
					processJSON(value.String(), depth+1)
				}
				return true
			})
		}
	}
	// 特殊处理 JSON
	skipJson := false
	for _, bodyRaw := range jsonextractor.ExtractStandardJSON(string(body)) {
		skipJson = true
		var ok bool
		if bodyRaw, ok = utils.IsJSON(bodyRaw); ok {
			processJSON(bodyRaw, 1)
		}
	}

	if !skipJson {
		for _, result := range extractFromJSONValue.FindAllStringSubmatch(string(body), -1) {
			if len(result) > 2 {
				key, value := strings.TrimSpace(result[1]), strings.TrimSpace(result[2])
				if strings.HasPrefix(key, `"`) && strings.HasSuffix(key, `"`) {
					key = key[1 : len(key)-1]
				}
				if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
					value = value[1 : len(value)-1]
				}
				addResult(key, value)
			}
		}
	}

	for _, result := range extractFromEqValue.FindAllStringSubmatch(string(body), -1) {
		if len(result) > 2 {
			key, value := strings.TrimSpace(result[1]), strings.TrimSpace(result[2])
			if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
				value = value[1 : len(value)-1]
			}
			addResult(key, value)
		}
	}

	return results
}
