package aispec

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
)

func ShrinkAndSafeToFile(i any) string {
	var buf bytes.Buffer
	if utils.IsMap(i) {
		for k, v := range utils.InterfaceToGeneralMap(i) {
			buf.WriteString("# parameter: " + fmt.Sprint(k) + "\n")
			valString := utils.InterfaceToString(v)
			buf.WriteString(valString + "\n\n")
		}
	} else if funk.IsIteratee(i) {
		idx := 0
		funk.ForEach(i, func(element any) {
			idx++
			buf.WriteString("# parameter: " + fmt.Sprint(idx) + "\n")
			valString := utils.InterfaceToString(element)
			buf.WriteString(valString + "\n\n")
		})
	} else {
		buf.WriteString("# raw input " + "\n")
		buf.WriteString(utils.InterfaceToString(i))
	}
	results := strings.TrimRight(buf.String(), "\n")
	var promptString string
	if buf.Len() > 1024 {
		filename := consts.TempAIFileFast("huge-params-*.md", buf.String())
		promptString = utils.ShrinkString(results, 1000) + fmt.Sprintf(" [saved in %v]", filename)
	} else {
		promptString = results
	}
	return promptString
}

var EnableNewLoadOption = true

func GetBaseURLFromConfig(config *AIConfig, defaultRootUrl, defaultUri string) string {
	return GetBaseURLFromConfigEx(config, defaultRootUrl, defaultUri, true)
}

func GetBaseURLFromConfigEx(config *AIConfig, defaultRootUrl, defaultUri string, openaiMode bool) string {
	fixDomain(config)

	keepChatCompletionsSuffix := func(s string) string {
		if !strings.HasSuffix(s, "/chat/completions") {
			trimSlash := strings.TrimRight(s, "/")
			s = trimSlash + "/chat/completions"
		}
		return s
	}

	if config.BaseURL != "" {
		if openaiMode {
			config.BaseURL = keepChatCompletionsSuffix(config.BaseURL)
		}
		return config.BaseURL
	}
	// 按照NoHttps修改defaultRootUrl的scheme
	if config.NoHttps && strings.HasPrefix(defaultRootUrl, "https://") {
		defaultRootUrl = "http://" + strings.TrimPrefix(defaultRootUrl, "https://")
	} else if !config.NoHttps && strings.HasPrefix(defaultRootUrl, "http://") {
		defaultRootUrl = "https://" + strings.TrimPrefix(defaultRootUrl, "http://")
	}
	rootUrl := defaultRootUrl
	if config.Domain != "" {
		if config.NoHttps {
			rootUrl = "http://" + config.Domain
		} else {
			rootUrl = "https://" + config.Domain
		}
	}
	urlPath, err := url.JoinPath(rootUrl, defaultUri)
	if err != nil {
		result := rootUrl + defaultUri
		if openaiMode {
			result = keepChatCompletionsSuffix(result)
		}
		return result
	}
	if openaiMode {
		urlPath = keepChatCompletionsSuffix(urlPath)
	}
	return urlPath
}

// fixDomain 修复不规范的domain配置
func fixDomain(c *AIConfig) {
	// 修复domain配置
	fixedDomain := c.Domain
	//originDomain := c.Domain
	if fixedDomain != "" {
		// 检查domain是否包含协议前缀
		if strings.HasPrefix(fixedDomain, "http://") || strings.HasPrefix(fixedDomain, "https://") {
			// 解析URL
			if strings.HasPrefix(fixedDomain, "http://") {
				c.NoHttps = true
				fixedDomain = strings.TrimPrefix(fixedDomain, "http://")
			} else {
				c.NoHttps = false
				fixedDomain = strings.TrimPrefix(fixedDomain, "https://")
			}

			// 检查是否包含路径
			if strings.Contains(fixedDomain, "/") {
				parts := strings.SplitN(fixedDomain, "/", 2)
				c.Domain = parts[0]
				if c.BaseURL == "" {
					// 构造BaseURL
					if c.NoHttps {
						c.BaseURL = "http://" + parts[0] + "/" + parts[1]
					} else {
						c.BaseURL = "https://" + parts[0] + "/" + parts[1]
					}
				}
			} else {
				c.Domain = fixedDomain
			}

			//log.Debugf("检测到不标准的domain配置: %s，已自动解析为 Domain: %s, NoHttps: %v, BaseURL: %s",
			//	originDomain, c.Domain, c.NoHttps, c.BaseURL)
		} else {
			// 标准的domain配置，不包含协议
			c.Domain = fixedDomain
		}
	}
}
