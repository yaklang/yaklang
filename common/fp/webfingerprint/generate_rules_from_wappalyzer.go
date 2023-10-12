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
	for name, info := range f.Apps {
		name = strings.ReplaceAll(strings.ToLower(name), " ", "_")
		if info.Headers != nil {
			webRule := WebRule{}
			var methods []*WebMatcherMethods
			method := WebMatcherMethods{}
			var headers []*HTTPHeaderMatcher
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
				headers = append(headers, &header)
			}

			method.HTTPHeaders = headers
			methods = append(methods, &method)
			webRule.Methods = methods
			rules = append(rules, webRule)
		}
		if info.Cookies != nil {
			webRule := WebRule{}
			var methods []*WebMatcherMethods
			method := WebMatcherMethods{}
			var headers []*HTTPHeaderMatcher
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
				headers = append(headers, &header)
			}
			method.HTTPHeaders = headers
			methods = append(methods, &method)
			webRule.Methods = methods
			rules = append(rules, webRule)
		}

		switch meta := info.Meta.(type) {
		case map[string]interface{}:
			webRule := WebRule{}
			var methods []*WebMatcherMethods
			method := WebMatcherMethods{}
			var keywords []*KeywordMatcher
			for k, v := range meta {
				log.Infof("k: %s, v: %s", k, v)
				switch vv := v.(type) {
				case string:
					if strings.Contains(fmt.Sprint(vv), "?!") {
						continue
					}
					keyword := KeywordMatcher{}
					vc := strings.Split(fmt.Sprint(vv), `\;confidence:`)
					if len(vc) > 1 {
						vv = vc[0]
					}
					vs := strings.Split(fmt.Sprint(vv), `\;version:`)
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
					keyword.Regexp = fmt.Sprintf(`< *meta[^>]*name *= *['"]%s['"][^>]*content *= *['"]%s`, k, vs[0])
					keyword.Product = name
					keywords = append(keywords, &keyword)
				case []interface{}:
					if strings.Contains(fmt.Sprint(vv[0]), "?!") {
						continue
					}
					keyword := KeywordMatcher{}
					vc := strings.Split(fmt.Sprint(vv), `\;confidence:`)
					if len(vc) > 1 {
						vv[0] = vc[0]
					}
					vs := strings.Split(fmt.Sprint(vv[0]), `\;version:`)
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
					keyword.Regexp = fmt.Sprintf(`< *meta[^>]*name *= *['"]%s['"][^>]*content *= *['"]%s`, k, vs[0])
					keyword.Product = name
					keywords = append(keywords, &keyword)
				}

			}
			method.Keywords = keywords
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
				vc := strings.Split(html, `\;confidence:`)
				if len(vc) > 1 {
					html = vc[0]
				}
				vs := strings.Split(html, `\;version:`)
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
				vc := strings.Split(fmt.Sprint(v), `\;confidence:`)
				if len(vc) > 1 {
					v = vc[0]
				}
				vs := strings.Split(fmt.Sprint(v), `\;version:`)
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
					keyword.VersionIndex = *versionIndex
				}
				if version != nil {
					keyword.CPE.Version = *version
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

		switch script := info.Scripts.(type) {
		case string:
			webRule := WebRule{}
			methods := []*WebMatcherMethods{}
			method := WebMatcherMethods{}
			keywords := []*KeywordMatcher{}
			vc := strings.Split(script, `\;confidence:`)
			if len(vc) > 1 {
				script = vc[0]
			}
			vs := strings.Split(script, `\;version:`)
			if !strings.Contains(script, "?!") {
				keyword := KeywordMatcher{}
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
					keyword.VersionIndex = *versionIndex
				}
				if version != nil {
					keyword.CPE.Version = *version
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
				vc := strings.Split(fmt.Sprint(v), `\;confidence:`)
				if len(vc) > 1 {
					v = vc[0]
				}
				vs := strings.Split(fmt.Sprint(v), `\;version:`)
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
					keyword.VersionIndex = *versionIndex
				}
				if version != nil {
					keyword.CPE.Version = *version
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
	if err != nil {
		log.Errorf("Marshal error: %s", err)
	}
	err = ioutil.WriteFile("./fingerprint-rules.yml", output, 0644)
	if err != nil {
		log.Errorf("WriteFile error: %s", err)
	}
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
