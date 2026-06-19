package bruteforce

import "github.com/yaklang/yaklang/common/rpa/captcha"

// BruteForceStart 对目标登录页面进行基于浏览器的自动化登录爆破（导出名为 rpa.Bruteforce）
// 参数:
//   - url: 目标登录页面 URL
//   - opts: 可选项，如 rpa.bruteUsername、rpa.brutePassword、rpa.bruteUserElement 等
//
// 返回值:
//   - 爆破成功的用户名
//   - 爆破成功的密码
//
// Example:
// ```
// // 对登录页面进行爆破（示意性示例，需要本地已安装浏览器）
// username, password = rpa.Bruteforce("http://example.com/login",
//
//	rpa.bruteUsername("admin"),
//	rpa.brutePassword("admin", "123456"),
//	rpa.bruteUserElement("#username"),
//	rpa.brutePassElement("#password"),
//	rpa.bruteButtonElement("#login"),
//
// )
// println(username, password)
// ```
func BruteForceStart(url string, opts ...ConfigOpt) (string, string) {
	config := &Config{
		webNavigateWait: 1,
		clickInterval:   1,
		responseWait:    1,

		doBefore: make(ConfigMethods, 0),

		// to be deleted
		captchaUrl: captcha.CAPTCHA_URL,
	}
	for _, opt := range opts {
		opt(config)
	}
	bf := BruteForce{
		web_navigate_load: config.webNavigateWait,
		click_interval:    config.clickInterval,
		response_wait:     config.responseWait,

		singleCaptchaErrorNum: 3,
		CaptchaErrorNumCount:  20,

		usernameStr:   config.usernameElement,
		passwordStr:   config.passwordElement,
		captchaStr:    config.captchaElement,
		captchaIMGStr: config.captchaIMGElement,
		buttonStr:     config.clickButtonElement,

		//CaptchaUrl: config.captchaUrl,
	}

	bf.CaptchaUrl = config.captchaUrl

	if config.usernamePath != "" && config.passwordPath != "" {
		bf.ReadUserPassList(config.usernamePath, config.passwordPath)
	}
	if len(config.username) > 0 {
		bf.usernames = append(bf.usernames, config.username...)
	}
	if len(config.password) > 0 {
		bf.passwords = append(bf.passwords, config.password...)
	}
	// fmt.Printf("start brute: %s\n", url)
	return bf.Start(url, config.doBefore...)
}
