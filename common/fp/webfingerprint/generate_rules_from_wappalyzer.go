package webfingerprint

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"net/http"
	log "yaklang/common/log"
	"strconv"
	"strings"
)

func check(e error) {
	if e != nil {
		log.Error(e)
	}
}

func GenerateRulesFromWappalyzer() {
	resp, err := HttpGetWithRetry(3, "https://raw.githubusercontent.com/AliasIO/Wappalyzer/master/src/apps.json")
	f := WappalyzerBase{}
	err = json.Unmarshal(resp, &f)
	check(err)
	var rules []WebRule
	for name, info := range f.Apps {
		name = strings.ReplaceAll(strings.ToLower(name), " ", "_")
		if info.Headers != nil {
			webRule := WebRule{}
			methods := []*WebMatcherMethods{}
			method := WebMatcherMethods{}
			headers := []*HTTPHeaderMatcher{}
			for k, v := range info.Headers {
				if strings.Contains(v, "?!") {
					continue
				}
				header := HTTPHeaderMatcher{}
				vs := strings.Split(v, `\;version:`)
				if strings.HasPrefix(vs[0], "^") {
					vs[0] = vs[0][1:]
				}
				if len(vs) > 1 {
					versionIndex, err := strconv.Atoi(vs[1][1:])
					if err == nil {
						header.HeaderValue.VersionIndex = versionIndex
					} else {
						if !strings.Contains(vs[1], `\`) {
							header.HeaderValue.Version = vs[1]
						}
					}
				}
				header.HeaderValue.Product = name
				header.HeaderName = k
				header.HeaderValue.Regexp = vs[0]
				headers = append(headers, &header)
			}

			method.HTTPHeaders = headers
			methods = append(methods, &method)
			webRule.Methods = methods
			rules = append(rules, webRule)
		}
		if info.Meta != nil {
			webRule := WebRule{}
			methods := []*WebMatcherMethods{}
			method := WebMatcherMethods{}
			keywords := []*KeywordMatcher{}
			for k, v := range info.Meta {
				if strings.Contains(v, "?!") {
					continue
				}
				keyword := KeywordMatcher{}
				vs := strings.Split(v, `\;version:`)
				if strings.HasPrefix(vs[0], "^") {
					vs[0] = vs[0][1:]
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
				keyword.Regexp = fmt.Sprintf(`< *meta[^>]*name *= *['"]%s['"][^>]*content *= *['"]%s`, k, vs[0])
				keyword.Product = name
				keywords = append(keywords, &keyword)
			}
			method.Keywords = keywords
			methods = append(methods, &method)
			webRule.Methods = methods
			rules = append(rules, webRule)
		}
		if info.Cookies != nil {
			webRule := WebRule{}
			methods := []*WebMatcherMethods{}
			method := WebMatcherMethods{}
			headers := []*HTTPHeaderMatcher{}
			for k, v := range info.Cookies {
				if strings.Contains(v, "?!") {
					continue
				}
				header := HTTPHeaderMatcher{}
				header.HeaderName = "Set-Cookie"
				vs := strings.Split(v, `\;version:`)
				if strings.HasPrefix(vs[0], "^") {
					vs[0] = vs[0][1:]
				}
				if len(vs) > 1 {
					version, err := strconv.Atoi(vs[1][1:])
					if err == nil {
						header.HeaderValue.VersionIndex = version
					} else {
						if !strings.Contains(vs[1], `\`) {
							header.HeaderValue.Version = vs[1]
						}
					}
				}
				header.HeaderValue.Regexp = fmt.Sprintf(`%s=%s`, k, vs[0])
				header.HeaderValue.Product = name
				headers = append(headers, &header)
			}
			method.HTTPHeaders = headers
			methods = append(methods, &method)
			webRule.Methods = methods
			rules = append(rules, webRule)
		}

		switch html := info.Html.(type) {
		case string:
			webRule := WebRule{}
			methods := []*WebMatcherMethods{}
			method := WebMatcherMethods{}
			keywords := []*KeywordMatcher{}
			if !strings.Contains(html, "?!") {
				keyword := KeywordMatcher{}
				vs := strings.Split(html, `\;version:`)
				if strings.HasPrefix(vs[0], "^") {
					vs[0] = vs[0][1:]
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
				keywords = append(keywords, &keyword)

				method.Keywords = keywords
				methods = append(methods, &method)
				webRule.Methods = methods
				rules = append(rules, webRule)
			}
		case []interface{}:
			webRule := WebRule{}
			methods := []*WebMatcherMethods{}
			method := WebMatcherMethods{}
			keywords := []*KeywordMatcher{}
			for _, v := range html {
				if strings.Contains(fmt.Sprint(v), "?!") {
					continue
				}
				keyword := KeywordMatcher{}
				vs := strings.Split(fmt.Sprint(v), `\;version:`)
				if strings.HasPrefix(vs[0], "^") {
					vs[0] = vs[0][1:]
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
				keyword.Regexp = vs[0]
				keyword.Product = name
				keywords = append(keywords, &keyword)
			}
			method.Keywords = keywords
			methods = append(methods, &method)
			webRule.Methods = methods
			rules = append(rules, webRule)
		}

		switch script := info.Script.(type) {
		case string:
			webRule := WebRule{}
			methods := []*WebMatcherMethods{}
			method := WebMatcherMethods{}
			keywords := []*KeywordMatcher{}
			vs := strings.Split(script, `\;version:`)
			if !strings.Contains(script, "?!") {
				keyword := KeywordMatcher{}
				if strings.HasPrefix(vs[0], "^") {
					vs[0] = vs[0][1:]
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
				keyword.Regexp = `< *script[^>]*src *= *['"][^'"]*` + vs[0]
				keyword.Product = name
				keywords = append(keywords, &keyword)

				method.Keywords = keywords
				methods = append(methods, &method)
				webRule.Methods = methods
				rules = append(rules, webRule)
			}
		case []interface{}:
			webRule := WebRule{}
			methods := []*WebMatcherMethods{}
			method := WebMatcherMethods{}
			keywords := []*KeywordMatcher{}
			for _, v := range script {
				if strings.Contains(fmt.Sprint(v), "?!") {
					continue
				}
				keyword := KeywordMatcher{}
				vs := strings.Split(fmt.Sprint(v), `\;version:`)
				if strings.HasPrefix(vs[0], "^") {
					vs[0] = vs[0][1:]
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
				keyword.Regexp = `< *script[^>]*src *= *['"][^'"]*` + vs[0]
				keyword.Product = name
				keywords = append(keywords, &keyword)
			}
			method.Keywords = keywords
			methods = append(methods, &method)
			webRule.Methods = methods
			rules = append(rules, webRule)
		}
	}
	output, err := yaml.Marshal(rules)
	check(err)
	output = output
	err = ioutil.WriteFile("data/fingerprint-rules.yml", output, 0644)
	check(err)
}

type ResponseInfo struct {
	http.Response
	Body []byte
}

type WappalyzerBase struct {
	Apps map[string]Info
}

type Info struct {
	//Cats    []int
	Icon    string
	Website string

	Headers map[string]string
	Html    interface{}
	Script  interface{}
	Meta    map[string]string
	Cookies map[string]string
	Url     string
	Js      map[string]string
}
