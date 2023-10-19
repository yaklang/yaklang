package webfingerprint

import (
	"encoding/json"
	"fmt"
	log "github.com/yaklang/yaklang/common/log"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
)

func check(e error) {
	if e != nil {
		log.Error(e)
	}
}

func processVs(vs []string, name string) (*string, *int, error) {
	var version *string
	var versionIndex *int

	if len(vs) > 1 {
		if strings.Contains(vs[1], "?") {
			parts := strings.Split(vs[1], "?")
			if len(parts) > 0 {
				vIndex, err := strconv.Atoi(parts[0][1:])
				if err == nil {
					versionIndex = &vIndex
				}
			}
			if len(parts) > 1 {
				v := strings.TrimSuffix(parts[1], ":")
				version = &v
			}
		} else {
			vIndex, err := strconv.Atoi(strings.Trim(vs[1], "\\"))
			if err == nil {
				versionIndex = &vIndex
			}
			if err != nil && !strings.Contains(vs[1], "\\") {
				v := strings.TrimSuffix(vs[1], ":")
				version = &v
			}
		}
	}

	return version, versionIndex, nil
}

func GenerateRulesFromWappalyzer() {
	//resp, err := HttpGetWithRetry(3, "https://raw.githubusercontent.com/AliasIO/Wappalyzer/master/src/apps.json")
	//resp, err := HttpGetWithRetry(3, "https://github.com/r0eXpeR/fingerprint/blob/main/wappalyzer-%E6%8C%87%E7%BA%B9/a.json")
	resp, err := HttpGetWithRetry(3, "https://raw.githubusercontent.com/Lissy93/wapalyzer/main/apps.json")
	//resp, err := ioutil.ReadFile("./apps.json")
	if err != nil {
		log.Errorf("HTTP GET error: %s", err)
	}
	f := WappalyzerBase{}
	err = json.Unmarshal(resp, &f)

	var rules []WebRule
	productRuleMap := make(map[string]*WebRule)

	for name, info := range f.Apps {
		name = strings.ReplaceAll(strings.ToLower(name), " ", "_")
		if info.Headers != nil {
			//webRule := WebRule{}
			//var methods []*WebMatcherMethods
			//method := WebMatcherMethods{}
			//var headers []*HTTPHeaderMatcher
			for k, v := range info.Headers {
				if strings.Contains(v, "?!") {
					continue
				}
				header := HTTPHeaderMatcher{}
				vc := strings.Split(fmt.Sprint(v), `\;confidence:`)
				if len(vc) > 1 {
					v = vc[0]
				}
				vs := strings.Split(v, `\;version:`)
				if strings.HasPrefix(vs[0], "^") {
					vs[0] = vs[0][1:]
				}
				if strings.HasSuffix(vs[0], "$") {
					vs[0] = vs[0][:len(vs[0])-1]
				}
				version, versionIndex, err := processVs(vs, name)
				if err != nil {
					log.Errorf("processVs error: %s", err)
					continue
				}
				if versionIndex != nil {
					header.HeaderValue.VersionIndex = *versionIndex
				}
				if version != nil {
					header.HeaderValue.Version = *version
				}

				header.HeaderValue.Product = name
				header.HeaderName = k
				header.HeaderValue.Regexp = vs[0]
				// Check if a rule for this product already exists
				if rule, exists := productRuleMap[name]; exists {
					rule.Methods[0].HTTPHeaders = append(rule.Methods[0].HTTPHeaders, &header)
				} else {
					method := WebMatcherMethods{
						HTTPHeaders: []*HTTPHeaderMatcher{&header},
					}
					webRule := WebRule{
						Methods: []*WebMatcherMethods{&method},
					}
					productRuleMap[name] = &webRule
				}
			}

			//method.HTTPHeaders = headers
			//methods = append(methods, &method)
			//webRule.Methods = methods
			//rules = append(rules, webRule)
		}
		if info.Cookies != nil {
			//webRule := WebRule{}
			//var methods []*WebMatcherMethods
			//method := WebMatcherMethods{}
			//var headers []*HTTPHeaderMatcher
			for k, v := range info.Cookies {
				if strings.Contains(v, "?!") {
					continue
				}
				header := HTTPHeaderMatcher{}
				header.HeaderName = "Set-Cookie"
				vc := strings.Split(fmt.Sprint(v), `\;confidence:`)
				if len(vc) > 1 {
					v = vc[0]
				}
				vs := strings.Split(v, `\;version:`)
				if strings.HasPrefix(vs[0], "^") {
					vs[0] = vs[0][1:]
				}
				if strings.HasSuffix(vs[0], "$") {
					vs[0] = vs[0][:len(vs[0])-1]
				}
				version, versionIndex, err := processVs(vs, name)
				if err != nil {
					log.Errorf("processVs error: %s", err)
					continue
				}
				if versionIndex != nil {
					header.HeaderValue.VersionIndex = *versionIndex
				}
				if version != nil {
					header.HeaderValue.Version = *version
				}
				header.HeaderValue.Regexp = fmt.Sprintf(`%s=%s`, k, vs[0])
				header.HeaderValue.Product = name
				//headers = append(headers, &header)

				// Check if a rule for this product already exists
				if rule, exists := productRuleMap[name]; exists {
					rule.Methods[0].HTTPHeaders = append(rule.Methods[0].HTTPHeaders, &header)
				} else {
					method := WebMatcherMethods{
						HTTPHeaders: []*HTTPHeaderMatcher{&header},
					}
					webRule := WebRule{
						Methods: []*WebMatcherMethods{&method},
					}
					productRuleMap[name] = &webRule
				}
			}
			//method.HTTPHeaders = headers
			//methods = append(methods, &method)
			//webRule.Methods = methods
			//rules = append(rules, webRule)
		}

		switch meta := info.Meta.(type) {
		case map[string]interface{}:

			for k, v := range meta {
				keyword := ProcessMetaValue(v, k, name) // 假设这是处理逻辑
				if keyword == nil {
					continue
				}
				if rule, exists := productRuleMap[name]; exists {
					rule.Methods[0].Keywords = append(rule.Methods[0].Keywords, keyword)
				} else {
					method := WebMatcherMethods{
						Keywords: []*KeywordMatcher{keyword},
					}
					webRule := WebRule{
						Methods: []*WebMatcherMethods{&method},
					}
					productRuleMap[name] = &webRule
				}
			}
		}

		switch html := info.Html.(type) {
		case string:
			keyword := processHtml(html, name)
			if keyword != nil {
				if rule, exists := productRuleMap[name]; exists {
					rule.Methods[0].Keywords = append(rule.Methods[0].Keywords, keyword)
				} else {
					method := WebMatcherMethods{
						Keywords: []*KeywordMatcher{keyword},
					}
					webRule := WebRule{
						Methods: []*WebMatcherMethods{&method},
					}
					productRuleMap[name] = &webRule
				}
			}
		case []interface{}:
			for _, v := range html {
				keyword := processHtml(v, name)
				if keyword != nil {
					if rule, exists := productRuleMap[name]; exists {
						rule.Methods[0].Keywords = append(rule.Methods[0].Keywords, keyword)
					} else {
						method := WebMatcherMethods{
							Keywords: []*KeywordMatcher{keyword},
						}
						webRule := WebRule{
							Methods: []*WebMatcherMethods{&method},
						}
						productRuleMap[name] = &webRule
					}
				}
			}
		}

		switch script := info.Scripts.(type) {
		case string:
			keyword := processScript(script, name)
			if keyword != nil {
				if rule, exists := productRuleMap[name]; exists {
					rule.Methods[0].Keywords = append(rule.Methods[0].Keywords, keyword)
				} else {
					method := WebMatcherMethods{
						Keywords: []*KeywordMatcher{keyword},
					}
					webRule := WebRule{
						Methods: []*WebMatcherMethods{&method},
					}
					productRuleMap[name] = &webRule
				}
			}
		case []interface{}:
			for _, v := range script {
				keyword := processScript(v, name)
				if keyword != nil {
					if rule, exists := productRuleMap[name]; exists {
						rule.Methods[0].Keywords = append(rule.Methods[0].Keywords, keyword)
					} else {
						method := WebMatcherMethods{
							Keywords: []*KeywordMatcher{keyword},
						}
						webRule := WebRule{
							Methods: []*WebMatcherMethods{&method},
						}
						productRuleMap[name] = &webRule
					}
				}
			}
		}
	}
	for _, rule := range productRuleMap {
		rules = append(rules, *rule)
	}
	output, err := yaml.Marshal(rules)
	if err != nil {
		log.Errorf("Marshal error: %s", err)
	}
	err = ioutil.WriteFile("./fingerprint-rules.yml", output, 0644)
	if err != nil {
		log.Errorf("WriteFile error: %s", err)
	}
}

// ProcessMetaValue processes and returns a keyword matcher
func ProcessMetaValue(v interface{}, key, name string) *KeywordMatcher {
	vv, ok := v.(string)
	if !ok {
		arr, arrOk := v.([]interface{})
		if !arrOk || len(arr) == 0 {
			return nil
		}
		vv, ok = arr[0].(string)
		if !ok {
			return nil
		}
	}
	if strings.Contains(vv, "?!") {
		return nil
	}
	keyword := &KeywordMatcher{}
	vc := strings.Split(vv, `\;confidence:`)
	if len(vc) > 1 {
		vv = vc[0]
	}
	vs := strings.Split(vv, `\;version:`)
	if strings.HasPrefix(vs[0], "^") {
		vs[0] = vs[0][1:]
	}
	if strings.HasSuffix(vs[0], "$") {
		vs[0] = vs[0][:len(vs[0])-1]
	}
	if len(vs) > 1 {
		version, err := strconv.Atoi(vs[1][1:])
		if err == nil {
			keyword.VersionIndex = version
		} else {
			if !strings.Contains(vs[1], `\`) {
				keyword.CPE.Version = vs[1]
			}
		}
	}
	keyword.Regexp = fmt.Sprintf(`< *meta[^>]*name *= *['"]%s['"][^>]*content *= *['"]%s`, key, vs[0])
	keyword.Product = name
	return keyword
}

func processHtml(value interface{}, name string) *KeywordMatcher {
	// 具体处理逻辑
	vStr := fmt.Sprint(value)
	if strings.Contains(vStr, "?!") {
		return nil
	}

	keyword := KeywordMatcher{}
	vc := strings.Split(vStr, `\;confidence:`)
	if len(vc) > 1 {
		vStr = vc[0]
	}
	vs := strings.Split(vStr, `\;version:`)
	if strings.HasPrefix(vs[0], "^") {
		vs[0] = vs[0][1:]
	}
	if strings.HasSuffix(vs[0], "$") {
		vs[0] = vs[0][:len(vs[0])-1]
	}
	if len(vs) > 1 {
		version, err := strconv.Atoi(vs[1][1:])
		if err == nil {
			keyword.VersionIndex = version
		} else {
			if !strings.Contains(vs[1], `\`) {
				keyword.Version = vs[1]
			}
		}
	}
	keyword.Regexp = vs[0]
	keyword.Product = name
	return &keyword
}

func processScript(value interface{}, name string) *KeywordMatcher {
	// 具体处理逻辑
	vStr := fmt.Sprint(value)
	if strings.Contains(vStr, "?!") {
		return nil
	}

	keyword := KeywordMatcher{}
	vc := strings.Split(vStr, `\;confidence:`)
	if len(vc) > 1 {
		vStr = vc[0]
	}
	vs := strings.Split(vStr, `\;version:`)
	if strings.HasPrefix(vs[0], "^") {
		vs[0] = vs[0][1:]
	}
	if strings.HasSuffix(vs[0], "$") {
		vs[0] = vs[0][:len(vs[0])-1]
	}
	version, versionIndex, err := processVs(vs, name)
	if err != nil {
		log.Errorf("processVs error: %s", err)
		return nil
	}
	if versionIndex != nil {
		keyword.VersionIndex = *versionIndex
	}
	if version != nil {
		keyword.CPE.Version = *version
	}
	keyword.Regexp = `< *script[^>]*src *= *['"][^'"]*` + vs[0]
	keyword.Product = name
	return &keyword
}

type ResponseInfo struct {
	http.Response
	Body []byte
}

type WappalyzerBase struct {
	Apps map[string]Info
}

type Info struct {
	Cats    []int
	Icon    string
	Website string

	Headers map[string]string
	Html    interface{}
	Scripts interface{}
	Meta    interface{}
	Cookies map[string]string
	Url     string
	Js      map[string]string
	Implies []string
}
