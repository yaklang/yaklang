package webfingerprint

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/regen"
	"github.com/yaklang/yaklang/embed"
	"math/rand"
	"path"
	"strconv"
	"strings"
	"time"
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
	// debug
	//resp, _ := ioutil.ReadFile("./webfingerprint/fingerprint-rules.yml")
	//
	//rules, _ := ParseWebFingerprintRules(resp)
	names := strings.Split(name, ",")
	rules, _ := LoadDefaultDataSource()
	var err error
	headerStr := "HTTP/1.1 200 OK" + utils.CRLF
	bodyStr := ""
	nameMap := make(map[string]struct{})
	for _, name := range names {
		nameMap[name] = struct{}{}
	}
	for _, rule := range rules {
		for _, m := range rule.Methods {
			if m.MD5s != nil {
				continue
			}
			if m.Keywords != nil {
				for _, keyword := range m.Keywords {
					if _, exists := nameMap[keyword.Product]; exists {
						fakeBody := keyword.Regexp
						log.Debugf("[%s] fakeBody: %s", keyword.Product, fakeBody)

						generates, err := regen.GenerateOne(fakeBody)
						if err != nil {
							continue
						}
						log.Debugf("[%s] generates: %s", keyword.Product, generates)

						if strings.HasSuffix(keyword.Regexp, " )") || strings.HasSuffix(keyword.Regexp, " ") {
							bodyStr += utils.CRLF + generates[0] + "filling"
						} else {
							bodyStr += utils.CRLF + generates[0]
						}
					}
				}
			}
			if m.HTTPHeaders != nil {
				for _, header := range m.HTTPHeaders {
					if _, exists := nameMap[header.HeaderValue.Product]; exists {
						if header.HeaderValue.Regexp != "" {
							if header.HeaderName == "" {
								header.HeaderName = utils.RandSecret(5)
							}
							fakeHeader := header.HeaderValue.Regexp
							log.Debugf("[%s] fakeHeader: %s", header.HeaderValue.Product, fakeHeader)

							generates, err := regen.GenerateVisibleOne(fakeHeader)
							if err != nil {
								continue
							}
							log.Debugf("[%s] generates: %s", header.HeaderValue.Product, generates)

							if generates == " " {
								generates = "filling"
							}
							if strings.HasSuffix(header.HeaderValue.Regexp, " ") {
								headerStr += header.HeaderName + ": " + generates + "filling" + utils.CRLF
							} else {
								headerStr += header.HeaderName + ": " + generates + utils.CRLF

							}
						} else {
							headerStr += header.HeaderName + ": EMPTY" + utils.CRLF

						}
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
	fmt.Println(string(response))
	return utils.DebugMockHTTP([]byte(response))
}

func MockRandomWebFingerPrints() ([]string, string, int) {
	// debug
	//resp, _ := ioutil.ReadFile("./webfingerprint/fingerprint-rules.yml")
	//
	//rules, _ := ParseWebFingerprintRules(resp)

	rules, _ := LoadDefaultDataSource()
	src := rand.NewSource(time.Now().UnixNano())
	r := rand.New(src)

	// Generate a list of 10 random rules from the rules slice
	randomRules := make([]*WebRule, 100)
	for i := range randomRules {
		randomRules[i] = rules[r.Intn(len(rules))]
	}
	// debug
	//randomRules = rules
	var ruleNames []string
	//var err error
	headerStr := "HTTP/1.1 200 OK" + utils.CRLF
	bodyStr := ""
	var serverCount = 1
	for _, rule := range randomRules {
		for _, m := range rule.Methods {
			if m.MD5s != nil {
				continue
			}
			if m.Keywords != nil {
				for _, keyword := range m.Keywords {
					ruleNames = append(ruleNames, keyword.Product)
					fakeBody := keyword.Regexp
					log.Debugf("[%s] fakeBody: %s", keyword.Product, fakeBody)
					generates, err := regen.GenerateOne(fakeBody)
					if err != nil {
						continue
					}
					log.Debugf("[%s] generates: %s", keyword.Product, generates)
					if strings.HasSuffix(keyword.Regexp, " )") || strings.HasSuffix(keyword.Regexp, " ") {
						bodyStr += utils.CRLF + generates[0] + "filling"
					} else {
						bodyStr += utils.CRLF + generates[0]
					}
				}
			}
			if m.HTTPHeaders != nil {
				for _, header := range m.HTTPHeaders {
					if header.HeaderName == "Server" {
						header.HeaderName = "Server_" + strconv.Itoa(serverCount)
						serverCount++
					}
					ruleNames = append(ruleNames, header.HeaderValue.Product)
					if header.HeaderName == "" {
						header.HeaderName = utils.RandSecret(5)
					}
					if header.HeaderValue.Regexp != "" {
						fakeHeader := header.HeaderValue.Regexp
						log.Debugf("[%s] fakeHeader: %s", header.HeaderValue.Product, fakeHeader)
						generates, err := regen.GenerateVisibleOne(fakeHeader)
						if err != nil {
							continue
						}
						log.Debugf("[%s] generates: %s", header.HeaderValue.Product, generates)

						if generates == " " {
							generates = "filling"
						}
						if strings.HasSuffix(header.HeaderValue.Regexp, " ") {
							headerStr += header.HeaderName + ": " + generates + "filling" + utils.CRLF
						} else {
							headerStr += header.HeaderName + ": " + generates + utils.CRLF
						}
					} else {
						headerStr += header.HeaderName + ": EMPTY" + utils.CRLF

					}
				}
			}
		}
	}
	rsp := headerStr + utils.CRLF + bodyStr
	response, _, err := lowhttp.FixHTTPResponse([]byte(rsp))
	if err != nil {
		return nil, "", 0
	}
	//log.Infof("response: %s", string(response))
	//fmt.Println(string(response))

	host, port := utils.DebugMockHTTP(response)
	return ruleNames, host, port
}
