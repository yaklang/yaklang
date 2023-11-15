package webfingerprint

import (
	"math/rand"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/regen"
	"github.com/yaklang/yaklang/embed"
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
	defRules := rules
	rules = append(rules, DefaultWebFingerprintRules...)

	// 加载用户自定义的规则库
	userDefinedPath := "data/user-wfp-rules"
	files, err := embed.AssetDir(userDefinedPath)
	if err != nil {
		log.Infof("user defined rules is missed: %s", err)
		return rules, nil
	}
	userRules := make([]*WebRule, 0)
	for _, fileName := range files {
		var (
			content     []byte
			err         error
			absFileName string
		)

		if strings.HasSuffix(fileName, ".gz") || strings.HasSuffix(fileName, ".gzip") {
			absFileName = path.Join(userDefinedPath, fileName)
			content, err = embed.Asset(absFileName)
			if err != nil {
				log.Warnf("bindata fetch asset: %s failed: %s", absFileName, err)
				continue
			}
			content, err = utils.GzipDeCompress(content)
			if err != nil {
				log.Warnf("web fp rules[%v] decompress failed: %s", absFileName, err)
				continue
			}
		} else {
			absFileName = path.Join(userDefinedPath, fileName)
			content, err = embed.Asset(absFileName)
			if err != nil {
				log.Warnf("bindata fetch asset: %s failed: %s", absFileName, err)
				continue
			}
		}

		subRules, err := ParseWebFingerprintRules(content)
		if err != nil {
			log.Warnf("parse FILE:%s failed: %s", absFileName, err)
			continue
		}
		userRules = append(userRules, subRules...)
		rules = append(rules, subRules...)
	}

	defProductOccurrences := getProductOccurrences(defRules)
	defaultProductOccurrences := getProductOccurrences(DefaultWebFingerprintRules)
	userProductOccurrences := getProductOccurrences(userRules)

	allProducts := make(map[string]struct{})
	for product := range defProductOccurrences {
		allProducts[product] = struct{}{}
	}
	for product := range defaultProductOccurrences {
		allProducts[product] = struct{}{}
	}
	for product := range userProductOccurrences {
		allProducts[product] = struct{}{}
	}

	var duplicateProducts []string
	for product := range allProducts {
		count := 0
		if _, exists := defProductOccurrences[product]; exists {
			count++
		}
		if _, exists := defaultProductOccurrences[product]; exists {
			count++
		}
		if _, exists := userProductOccurrences[product]; exists {
			count++
		}
		if count > 1 {
			duplicateProducts = append(duplicateProducts, product)
		}
	}

	if len(duplicateProducts) > 0 {
		log.Debugf("Found duplicate product names:[%d] %v", len(duplicateProducts), duplicateProducts)
	} else {
		log.Infof("No duplicate product names found.")
	}

	return rules, nil
}

func getProductOccurrences(rules []*WebRule) map[string]int {
	productOccurrences := make(map[string]int)
	for _, rule := range rules {
		for _, method := range rule.Methods {
			// Extract from Keywords
			for _, keyword := range method.Keywords {
				productOccurrences[keyword.Product]++
			}
			// Extract from HTTPHeaders
			for _, header := range method.HTTPHeaders {
				productOccurrences[header.HeaderValue.Product]++
			}
			// Extract from MD5s
			for _, md5 := range method.MD5s {
				productOccurrences[md5.Product]++
			}
		}
	}
	return productOccurrences
}

func MockWebFingerPrintByName(name string) (string, int) {
	// debug
	//resp, _ := ioutil.ReadFile("./webfingerprint/fingerprint-rules.yml")
	//
	//rules, _ := ParseWebFingerprintRules(resp)
	names := strings.Split(name, ",")
	rules, _ := LoadDefaultDataSource()
	headerStr := "HTTP/1.1 200 OK" + utils.CRLF
	bodyStr := ""
	serverCount := 1
	nameMap := make(map[string]struct{})
	for _, name := range names {
		nameMap[name] = struct{}{}
	}
	log.Infof("nameMap: %v", len(nameMap))
	for _, rule := range rules {
		for _, m := range rule.Methods {
			if m.Keywords != nil {
				for _, keyword := range m.Keywords {
					if (strings.Contains(strings.ToLower(keyword.Regexp), "meta") && strings.Contains(strings.ToLower(keyword.Regexp), "url=")) || strings.Contains(strings.ToLower(keyword.Regexp), "window.location") {
						continue
					}
					if _, exists := nameMap[keyword.Product]; exists {
						fakeBody := keyword.Regexp
						log.Debugf("[%s] fakeBody: %s", keyword.Product, fakeBody)

						generated, err := regen.GenerateVisibleOne(fakeBody)
						if err != nil {
							continue
						}
						log.Debugf("[%s] generates: %s", keyword.Product, generated)

						if strings.HasSuffix(keyword.Regexp, " )") || strings.HasSuffix(keyword.Regexp, " ") {
							bodyStr += utils.CRLF + generated + "filling"
						} else {
							bodyStr += utils.CRLF + generated
						}
					}
				}
			}
			if m.HTTPHeaders != nil {
				for _, header := range m.HTTPHeaders {
					if _, exists := nameMap[header.HeaderValue.Product]; exists {
						if strings.ToLower(header.HeaderName) == "server" {
							header.HeaderName = "Server_" + strconv.Itoa(serverCount)
							serverCount++
						}
						if header.HeaderValue.Regexp != "" {
							if header.HeaderName == "" {
								header.HeaderName = utils.RandSample(5)
							}
							fakeHeader := header.HeaderValue.Regexp
							log.Debugf("[%s] fakeHeader: %s", header.HeaderValue.Product, fakeHeader)

							generated, err := regen.GenerateVisibleOne(fakeHeader)
							if err != nil {
								continue
							}
							log.Debugf("[%s] generates: %s", header.HeaderValue.Product, generated)

							if generated == " " || generated == "" {
								generated = "filling"
							}

							if strings.HasSuffix(header.HeaderValue.Regexp, " ") || strings.HasSuffix(generated, " ") {
								headerStr += header.HeaderName + ": " + generated + "filling" + utils.CRLF
							} else {
								headerStr += header.HeaderName + ": " + generated + utils.CRLF
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
	// fmt.Println(rsp)
	response, _, err := lowhttp.FixHTTPResponse([]byte(rsp))
	if err != nil {
		return "", 0
	}
	return utils.DebugMockHTTP(response)
	//return utils.DebugMockHTTPEx(func(req []byte) []byte {
	//	response, _, err := lowhttp.FixHTTPResponse([]byte(rsp))
	//	if err != nil {
	//		return nil
	//	}
	//	return response
	//})
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
	// randomRules = rules
	var ruleNames []string
	for _, rule := range randomRules {
		for _, m := range rule.Methods {
			if m.Keywords != nil {
				for _, keyword := range m.Keywords {
					if (strings.Contains(strings.ToLower(keyword.Regexp), "meta") && strings.Contains(strings.ToLower(keyword.Regexp), "url=")) || strings.Contains(strings.ToLower(keyword.Regexp), "window.location") {
						continue
					}
					ruleNames = append(ruleNames, keyword.Product)
				}
			}
			if m.HTTPHeaders != nil {
				for _, header := range m.HTTPHeaders {
					ruleNames = append(ruleNames, header.HeaderValue.Product)
				}
			}
		}
	}

	// 去重
	ruleNames = utils.RemoveRepeatStringSlice(ruleNames)
	log.Infof("Product count : %d", len(ruleNames))
	host, port := MockWebFingerPrintByName(strings.Join(ruleNames, ","))
	return ruleNames, host, port
}
