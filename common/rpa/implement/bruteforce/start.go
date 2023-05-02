package bruteforce

import "yaklang/common/rpa/captcha"

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
