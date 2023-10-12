package webfingerprint

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/regen"
	"github.com/yaklang/yaklang/embed"
	"math/rand"
	"path"
)

func LoadDefaultDataSource() ([]*WebRule, error) {
	content, err := embed.Asset("data/fingerprint-rules.yml.gz")
	if err != nil {
		return nil, errors.Errorf("get local web fingerprint rules failed: %s", err)
	}

	content, err = utils.GzipDeCompress(content)
	if err != nil {
		return nil, utils.Errorf("web fp rules decompress failed: %s", err)
	}

	rules, err := ParseWebFingerprintRules(content)
	if err != nil {
		return nil, errors.Errorf("parse wappalyzer rules failed: %s", err)
	}
	rules = append(rules, DefaultWebFingerprintRules...)

	// 加载用户自定义的规则库
	userDefinedPath := "data/user-wfp-rules"
	files, err := embed.AssetDir(userDefinedPath)
	if err != nil {
		log.Infof("user defined rules is missed: %s", err)
		return rules, nil
	}

	for _, fileName := range files {
		absFileName := path.Join(userDefinedPath, fileName)
		content, err := embed.Asset(absFileName)
		if err != nil {
			log.Warnf("bindata fetch asset: %s failed: %s", absFileName, err)
			continue
		}

		subRules, err := ParseWebFingerprintRules(content)
		if err != nil {
			log.Warnf("parse FILE:%s failed: %s", absFileName, err)
			continue
		}

		rules = append(rules, subRules...)
	}

	return rules, nil
}

func MockWebFingerPrintByName(name string) (string, int) {
	rules, _ := LoadDefaultDataSource()
	var generates []string
	var err error
	headerStr := "HTTP/1.1 200 OK" + utils.CRLF
	bodyStr := ""

	for _, rule := range rules {
		for _, m := range rule.Methods {
			if m.MD5s != nil {
				continue
			}
			if m.Keywords != nil {
				for _, keyword := range m.Keywords {
					if keyword.Product == name {
						fakeBody := keyword.Regexp
						generates, err = regen.GenerateOne(fakeBody)
						if err != nil {
							continue
						}
						bodyStr += utils.CRLF + generates[0]
					}
				}
			}
			if m.HTTPHeaders != nil {
				for _, header := range m.HTTPHeaders {
					if header.HeaderValue.Product == name {
						fakeHeader := header.HeaderValue.Regexp
						generates, err = regen.GenerateOne(fakeHeader)
						if err != nil {
							continue
						}
						headerStr += header.HeaderName + ": " + generates[0] + utils.CRLF
					}
				}
			}
		}
	}
	rsp := headerStr + utils.CRLF + bodyStr
	response, _, err := lowhttp.FixHTTPResponse([]byte(rsp))
	if err != nil {
		return "", 0
	}
	return utils.DebugMockHTTP([]byte(response))
}

func MockRandomWebFingerPrints() ([]string, string, int) {
	// debug
	//resp, _ := ioutil.ReadFile("./webfingerprint/fingerprint-rules.yml")
	//
	//rules, _ := ParseWebFingerprintRules(resp)

	rules, _ := LoadDefaultDataSource()

	// Generate a list of 10 random rules from the rules slice
	randomRules := make([]*WebRule, 10)
	for i := range randomRules {
		randomRules[i] = rules[rand.Intn(len(rules))]
	}
	// debug
	//randomRules = rules
	var ruleNames []string
	var generates []string
	var err error
	headerStr := "HTTP/1.1 200 OK" + utils.CRLF
	bodyStr := ""
	for _, rule := range randomRules {
		for _, m := range rule.Methods {
			if m.MD5s != nil {
				continue
			}
			if m.Keywords != nil {
				for _, keyword := range m.Keywords {
					ruleNames = append(ruleNames, keyword.Product)
					fakeBody := keyword.Regexp
					generates, err = regen.GenerateOne(fakeBody)
					if err != nil {
						continue
					}
					bodyStr += utils.CRLF + generates[0]
				}
			}
			if m.HTTPHeaders != nil {
				for _, header := range m.HTTPHeaders {
					ruleNames = append(ruleNames, header.HeaderValue.Product)

					fakeHeader := header.HeaderValue.Regexp
					generates, err = regen.GenerateOne(fakeHeader)
					if err != nil {
						continue
					}
					headerStr += header.HeaderName + ": " + generates[0] + utils.CRLF
				}
			}
		}
	}
	rsp := headerStr + utils.CRLF + bodyStr
	response, _, err := lowhttp.FixHTTPResponse([]byte(rsp))
	if err != nil {
		return nil, "", 0
	}
	//fmt.Println(string(response))

	host, port := utils.DebugMockHTTP(response)
	return ruleNames, host, port
}
