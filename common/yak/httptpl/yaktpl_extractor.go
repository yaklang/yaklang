package httptpl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/antchfx/htmlquery"
	"github.com/antchfx/xmlquery"
	"github.com/asaskevich/govalidator"
	"github.com/bcicen/jstream"
	"github.com/gobwas/httphead"
	"github.com/itchyny/gojq"
	"mime"
	"net/url"
	"regexp"
	"strings"
	"yaklang/common/jsonextractor"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/lowhttp"
)

type YakExtractor struct {
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
	Scope            string // header body all
	Groups           []string
	RegexpMatchGroup []int
	XPathAttribute   string
}

// group1 for key
// group2 for value
var kvExtractorRegexp = regexp.MustCompile(`([^\s=:,]+)\s*((:)|(=))\s*?(^\s[^\n\r]*)`)

func (y *YakExtractor) Execute(rsp []byte) (map[string]interface{}, error) {
	var material string
	switch strings.TrimSpace(strings.ToLower(y.Scope)) {
	case "body":
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp)
		material = string(body)
	case "header":
		header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp)
		material = header
	default:
		material = string(rsp)
	}

	var results = make(map[string]interface{})
	t := strings.TrimSpace(strings.ToLower(y.Type))
	switch t {
	case "regex":
		for index, group := range y.Groups {
			if group != "" {
				var tag string
				if y.Name == "" {
					tag = fmt.Sprintf("data%v", index)
				} else {
					tag = y.Name
				}
				r, err := regexp.Compile(group)
				if err != nil {
					log.Errorf("compile[%v] failed: %v", group, err)
					continue
				}
				var count = 0
				if len(y.RegexpMatchGroup) > 0 {
					for _, i := range y.RegexpMatchGroup {
						for _, res := range r.FindAllStringSubmatch(material, -1) {
							if len(res) > i {
								if count == 0 {
									results[tag] = res[i]
									count++
									continue
								}
								results[fmt.Sprintf("%v_%v", tag, count)] = res[i]
								count++
							}
						}
					}
					continue
				}

				for _, res := range r.FindAllStringSubmatch(material, -1) {
					if len(res) > 0 {
						if count == 0 {
							results[tag] = res[0]
							count++
							continue
						}
						results[tag] = res[0]
						count++
					}
				}
			}
		}
	case "kv", "key-value", "kval":
		for _, result := range kvExtractorRegexp.FindAllStringSubmatch(material, -1) {
			if len(result) > 5 {

				var key = strings.Trim(result[1], `"{}'`+"`,")
				var value = strings.Trim(result[5], `"{}'`+"`,")
				if len(y.Groups) == 0 || utils.StringSliceContain(y.Groups, key) {
					results[key] = value
				}
			}
		}
	case "json", "jq":
		for index, group := range y.Groups {
			if group != "" {
				var tag string
				if y.Name == "" {
					tag = fmt.Sprintf("data%v", index)
				} else {
					tag = y.Name
				}
				query, err := gojq.Parse(group)
				if err != nil {
					log.Errorf("parse jq query[%v] failed: %s", group, err)
					continue
				}
				var obj interface{}
				if err := json.Unmarshal([]byte(material), &obj); err != nil {
					log.Errorf("parse json failed: %s", err)
					continue
				}
				iter := query.Run(obj)
				count := 0
				for {

					v, ok := iter.Next()
					if !ok {
						break
					}
					if err, ok := v.(error); ok {
						log.Warnf("jq query[%v] failed: %s", group, err)
						continue
					}
					if count != 0 {
						results[fmt.Sprintf("%v_%v", tag, count)] = v
					} else {
						results[tag] = v
					}
					count++
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
			for index, group := range y.Groups {
				if group != "" {
					var tag string
					if y.Name == "" {
						tag = fmt.Sprintf("data%v", index)
					} else {
						tag = y.Name
					}
					nodes, err := xmlquery.QueryAll(doc, group)
					if err != nil {
						log.Errorf("xpath[%v] failed: %s", group, err)
						continue
					}
					for index, node := range nodes {
						if index != 0 {
							results[fmt.Sprintf("%v_%v", tag, index)] = node.InnerText()
						} else {
							results[tag] = node.InnerText()
						}
					}
				}
			}
		} else {
			doc, err := htmlquery.Parse(strings.NewReader(material))
			if err != nil {
				log.Warnf("parse html failed: %s", err)
				return nil, utils.Errorf("htmlquery.Parse failed: %s", err)
			}
			count := 0
			for index, group := range y.Groups {
				if group != "" {
					var tag string
					if y.Name == "" {
						tag = fmt.Sprintf("data%v", index)
					} else {
						tag = y.Name
					}
					nodes, err := htmlquery.QueryAll(doc, group)
					if err != nil {
						log.Errorf("xpath[%v] failed: %s", group, err)
						continue
					}
					for index, node := range nodes {
						count++
						var tagName string
						if index != 0 {
							tagName = fmt.Sprintf("%v_%v", tag, index)
						} else {
							tagName = tag
						}
						if y.XPathAttribute != "" {
							for _, attr := range node.Attr {
								if attr.Key == y.XPathAttribute {
									results[tagName] = attr.Val
								}
							}
						} else {
							results[tagName] = htmlquery.InnerText(node)
						}
					}
				}
			}
			if count <= 0 {
				isXml = true
				goto TRYXML
			}
		}
	case "nuclei-dsl", "nuclei":
		box := NewNucleiDSLYakSandbox()
		header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp)
		for index, group := range y.Groups {
			if group != "" {
				var tag string
				if y.Name == "" {
					tag = fmt.Sprintf("data%v", index)
				} else {
					tag = y.Name
				}
				data, err := box.Execute(group, map[string]interface{}{
					"body":     body,
					"header":   header,
					"raw":      string(rsp),
					"response": string(rsp),
				})
				if err != nil {
					continue
				}
				results[tag] = data
			}
		}
	default:
		return nil, utils.Errorf("unknown extractor type: %s", t)
	}
	return results, nil
}

var (
	extractFromJSONValue = regexp.MustCompile(`("[^":]+")\s*?:\s*(("([^"\n])+")|([^"]\S+[^"\s,]))`)
	extractFromEqValue   = regexp.MustCompile(`(?i)([_a-z][^=\s]*)=(([^=\s&,"]+)|("[^"]+"))`)
)

func ExtractKValFromResponse(rsp []byte) map[string]interface{} {
	results := make(map[string]interface{})
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacketEx2(rsp, nil, func(proto string, code int, codeMsg string) error {
		results["proto"] = proto
		results["status_code"] = code
		return nil
	}, func(line string) {
		k, v := lowhttp.SplitHTTPHeader(line)
		originKey := k
		k = strings.ReplaceAll(strings.ToLower(k), "-", "_")
		results[k] = v
		results[originKey] = v

		if k == `content_type` {
			ct, params, err := mime.ParseMediaType(v)
			if err != nil {
				return
			}
			results[`content_type`] = ct
			for k, v := range params {
				results[k] = v
			}
		} else {
			httphead.ScanOptions([]byte("__yaktpl_placeholder__; "+v+"; "), func(index int, option, attribute, value []byte) httphead.Control {
				decoded, err := url.QueryUnescape(string(value))
				if err != nil {
					results[string(attribute)] = string(value)
				} else {
					results[string(attribute)] = decoded
				}
				return httphead.ControlContinue
			})
		}

	})
	// 特殊处理 JSON
	var skipJson = false
	for _, bodyRaw := range jsonextractor.ExtractStandardJSON(string(body)) {
		skipJson = true
		if govalidator.IsJSON(bodyRaw) {
			for i := 0; i < strings.Count(bodyRaw, "{")+2; i++ {
				for k := range jstream.NewDecoder(bytes.NewBufferString(bodyRaw), i).Stream() {
					switch k.ValueType {
					case jstream.Object:
						data := utils.InterfaceToMapInterface(k.Value)
						if data == nil {
							continue
						}
						for k, v := range data {
							switch v.(type) {
							case string, int64, float64, []int8, []byte, bool, float32:
								results[k] = v
							default:
							}
						}
					}
				}
			}
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
				results[key] = value
			}
		}
	}

	for _, result := range extractFromEqValue.FindAllStringSubmatch(string(body), -1) {
		if len(result) > 2 {
			key, value := strings.TrimSpace(result[1]), strings.TrimSpace(result[2])
			if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
				value = value[1 : len(value)-1]
			}
			results[key] = value
		}
	}

	return results
}
