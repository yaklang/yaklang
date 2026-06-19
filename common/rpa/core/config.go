package core

import (
	"github.com/yaklang/yaklang/common/rpa/character"

	"github.com/go-rod/rod/lib/proto"
	"github.com/gobwas/glob"
)

type Config struct {
	//cookie
	cookie []*proto.NetworkCookieParam

	//设置 浏览器

	//对页面加载特定js
	evalJs map[string]string

	// 设置header 比如BasicAuth

	// headers []*header
	headers map[string]string

	//自动填写表单
	//formFill []*form
	formFill map[string]string
	//
	onRequest func(req *Req)
	// depth
	spider_depth int
	// proxy
	browser_proxy  string
	proxy_username string
	proxy_password string
	// strict url search
	strict_url bool
	// max scan url num
	url_count int

	// white & black subdomain
	white_subdomain []glob.Glob
	black_subdomain []glob.Glob

	// timeout per page
	timeout int

	captchaUrl string
}

type ConfigOpt func(c *Config)

// func GetConfigOpt() configOpt { return func(c *Config) }

func WithCookie(domain, k, v string) ConfigOpt {
	return func(c *Config) {
		c.cookie = append(c.cookie, &proto.NetworkCookieParam{
			Domain: domain,
			Name:   k,
			Value:  v,
		})

	}
}

func WithFrom(k, v string) ConfigOpt {
	return func(c *Config) {
		c.formFill[k] = v
	}
}

func WithEvalJs(url, js string) ConfigOpt {
	return func(c *Config) {
		c.evalJs[url] = js
	}
}

func WithOnRequest(f func(req *Req)) ConfigOpt {
	return func(c *Config) {
		c.onRequest = f
	}
}

// WithSpiderDepth 设置 RPA 爬虫的扫描深度（导出名为 rpa.depth）
// 参数:
//   - depth: 扫描深度
//
// 返回值:
//   - RPA 配置可选项
//
// Example:
// ```
// opt = rpa.depth(3)
// println(opt)
// ```
func WithSpiderDepth(depth int) ConfigOpt {
	return func(c *Config) {
		c.spider_depth = depth
	}
}

// WithBrowserProxy 设置浏览器代理地址，可选附带用户名密码（导出名为 rpa.proxy）
// 参数:
//   - url: 代理地址
//   - userinfo: 可选的代理用户名与密码
//
// 返回值:
//   - RPA 配置可选项
//
// Example:
// ```
// opt = rpa.proxy("http://127.0.0.1:8080")
// println(opt)
// ```
func WithBrowserProxy(url string, userinfo ...string) ConfigOpt {
	var username, password string
	if len(userinfo) < 2 {
		username = ""
		password = ""
	} else {
		username = userinfo[0]
		password = userinfo[1]
	}
	return func(c *Config) {
		c.browser_proxy = url
		c.proxy_username = username
		c.proxy_password = password
	}
}

// WithStrictUrlDetect 设置是否严格检测 URL 是否存在风险（导出名为 rpa.strictUrl）
// 参数:
//   - status: 是否启用严格 URL 检测
//
// 返回值:
//   - RPA 配置可选项
//
// Example:
// ```
// opt = rpa.strictUrl(true)
// println(opt)
// ```
func WithStrictUrlDetect(status bool) ConfigOpt {
	return func(c *Config) {
		c.strict_url = status
	}
}

// WithHeader 设置请求头，可传入 headers 文件路径或 JSON 字符串（导出名为 rpa.headers）
// 参数:
//   - s: headers 文件路径或 JSON 字符串
//
// 返回值:
//   - RPA 配置可选项
//
// Example:
// ```
// opt = rpa.headers(`{"User-Agent": "yak-rpa"}`)
// println(opt)
// ```
func WithHeader(s string) ConfigOpt {
	headerData, err := character.AnalysisHeaders(s)
	if err != nil {
		return func(c *Config) {}
	}
	return func(c *Config) {
		c.headers = headerData
	}
}

// WithUrlCount 设置 URL 总数上限，超出后停止扫描（导出名为 rpa.maxUrl）
// 参数:
//   - count: URL 数量上限
//
// 返回值:
//   - RPA 配置可选项
//
// Example:
// ```
// opt = rpa.maxUrl(100)
// println(opt)
// ```
func WithUrlCount(count int) ConfigOpt {
	return func(c *Config) {
		c.url_count = count
	}
}

// WithWhiteDomain 添加域名白名单（glob 匹配，导出名为 rpa.whiteDomain）
// 参数:
//   - matchStr: 域名 glob 匹配表达式
//
// 返回值:
//   - RPA 配置可选项
//
// Example:
// ```
// opt = rpa.whiteDomain("*.example.com")
// println(opt)
// ```
func WithWhiteDomain(matchStr string) ConfigOpt {
	g, err := glob.Compile(matchStr)
	if err != nil {

		return func(c *Config) {}
	}
	return func(c *Config) { c.white_subdomain = append(c.white_subdomain, g) }
}

// WithBlackDomain 添加域名黑名单（glob 匹配，导出名为 rpa.blackDomain）
// 参数:
//   - matchStr: 域名 glob 匹配表达式
//
// 返回值:
//   - RPA 配置可选项
//
// Example:
// ```
// opt = rpa.blackDomain("*.ad.example.com")
// println(opt)
// ```
func WithBlackDomain(matchStr string) ConfigOpt {
	g, err := glob.Compile(matchStr)
	if err != nil {

		return func(c *Config) {}
	}
	return func(c *Config) { c.black_subdomain = append(c.black_subdomain, g) }
}

// WithTimeout 设置单链接超时时间（导出名为 rpa.timeout）
// 参数:
//   - timeout: 超时时间（秒）
//
// 返回值:
//   - RPA 配置可选项
//
// Example:
// ```
// opt = rpa.timeout(10)
// println(opt)
// ```
func WithTimeout(timeout int) ConfigOpt {
	return func(c *Config) {
		c.timeout = timeout
	}
}

func WithCaptchaUrl(captchaUrl string) ConfigOpt {
	return func(c *Config) {
		c.captchaUrl = captchaUrl
	}
}

//
//func WithHeader(k, v string) configOpt {
//	return func(c *Config) {
//		c.headers = append(c.headers, &header{
//			Key:   k,
//			Value: v,
//		})
//	}
//}
