package rpa

import (
	"yaklang.io/yaklang/common/rpa/core"
	"yaklang.io/yaklang/common/rpa/implement/bruteforce"
)

var Exports = map[string]interface{}{
	"Start":       Start,
	"depth":       core.WithSpiderDepth,     //扫描深度
	"proxy":       core.WithBrowserProxy,    //代理地址 可选输入用户名密码
	"headers":     core.WithHeader,          //指定headers 可以选择headers文件地址或json字符串
	"strictUrl":   core.WithStrictUrlDetect, //检测url是否存在风险
	"maxUrl":      core.WithUrlCount,        //url获得总数限制 超出后不再扫描
	"whiteDomain": core.WithWhiteDomain,     //白名单
	"blackDomain": core.WithBlackDomain,     //黑名单
	"timeout":     core.WithTimeout,         //单链接超时

	// BruteForce
	"Bruteforce":          bruteforce.BruteForceStart,
	"bruteUserPassPath":   bruteforce.WithUserPassPath,
	"bruteUsername":       bruteforce.WithUsername,
	"brutePassword":       bruteforce.WithPassword,
	"bruteUserElement":    bruteforce.WithUsernameElement,
	"brutePassElement":    bruteforce.WithPasswordElement,
	"bruteCaptchaElement": bruteforce.WithCaptchaElement,
	"bruteButtonElement":  bruteforce.WithButtonElement,

	// BruteForce do_before_brute
	"click":  bruteforce.ClickMethod,
	"select": bruteforce.SelectMethod,
	"input":  bruteforce.InputMethod,
}
