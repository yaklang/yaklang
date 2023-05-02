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

func WithSpiderDepth(depth int) ConfigOpt {
	return func(c *Config) {
		c.spider_depth = depth
	}
}

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

func WithStrictUrlDetect(status bool) ConfigOpt {
	return func(c *Config) {
		c.strict_url = status
	}
}

func WithHeader(s string) ConfigOpt {
	headerData, err := character.AnalysisHeaders(s)
	if err != nil {
		return func(c *Config) {}
	}
	return func(c *Config) {
		c.headers = headerData
	}
}

func WithUrlCount(count int) ConfigOpt {
	return func(c *Config) {
		c.url_count = count
	}
}

func WithWhiteDomain(matchStr string) ConfigOpt {
	g, err := glob.Compile(matchStr)
	if err != nil {

		return func(c *Config) {}
	}
	return func(c *Config) { c.white_subdomain = append(c.white_subdomain, g) }
}

func WithBlackDomain(matchStr string) ConfigOpt {
	g, err := glob.Compile(matchStr)
	if err != nil {

		return func(c *Config) {}
	}
	return func(c *Config) { c.black_subdomain = append(c.black_subdomain, g) }
}

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
