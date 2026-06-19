package bruteforce

import "github.com/go-rod/rod/lib/input"

type Config struct {
	usernamePath string
	passwordPath string

	username []string
	password []string

	usernameElement    string
	passwordElement    string
	captchaElement     string
	captchaIMGElement  string
	clickButtonElement string

	webNavigateWait int
	clickInterval   int
	responseWait    int

	//
	doBefore ConfigMethods

	captchaUrl string
}

type ConfigOpt func(c *Config)

// WithUserPassPath 从文件加载用户名/密码字典（导出名为 rpa.bruteUserPassPath）
// 参数:
//   - filepath: 一个或两个文件路径，传一个时用户名密码使用同一文件
//
// 返回值:
//   - 爆破配置可选项
//
// Example:
// ```
// opt = rpa.bruteUserPassPath("/tmp/users.txt", "/tmp/pass.txt")
// println(opt)
// ```
func WithUserPassPath(filepath ...string) ConfigOpt {
	var userpath, passpath string
	if len(filepath) > 1 {
		userpath = filepath[0]
		passpath = filepath[1]
	} else if len(filepath) == 1 {
		userpath = filepath[0]
		passpath = filepath[0]
	}
	return func(c *Config) {
		c.usernamePath = userpath
		c.passwordPath = passpath
	}
}

// WithUsername 设置爆破用户名（导出名为 rpa.bruteUsername）
// 参数:
//   - username: 一个或多个用户名
//
// 返回值:
//   - 爆破配置可选项
//
// Example:
// ```
// opt = rpa.bruteUsername("admin", "root")
// println(opt)
// ```
func WithUsername(username ...string) ConfigOpt {
	return func(c *Config) {
		c.username = append(c.username, username...)
	}
}

// WithPassword 设置爆破密码（导出名为 rpa.brutePassword）
// 参数:
//   - password: 一个或多个密码
//
// 返回值:
//   - 爆破配置可选项
//
// Example:
// ```
// opt = rpa.brutePassword("admin", "123456")
// println(opt)
// ```
func WithPassword(password ...string) ConfigOpt {
	return func(c *Config) {
		c.password = append(c.password, password...)
	}
}

// WithUsernameElement 设置用户名输入框的元素选择器（导出名为 rpa.bruteUserElement）
// 参数:
//   - element: 用户名输入框的 CSS selector
//
// 返回值:
//   - 爆破配置可选项
//
// Example:
// ```
// opt = rpa.bruteUserElement("#username")
// println(opt)
// ```
func WithUsernameElement(element string) ConfigOpt {
	return func(c *Config) {
		c.usernameElement = element
	}
}

// WithPasswordElement 设置密码输入框的元素选择器（导出名为 rpa.brutePassElement）
// 参数:
//   - element: 密码输入框的 CSS selector
//
// 返回值:
//   - 爆破配置可选项
//
// Example:
// ```
// opt = rpa.brutePassElement("#password")
// println(opt)
// ```
func WithPasswordElement(element string) ConfigOpt {
	return func(c *Config) {
		c.passwordElement = element
	}
}

// WithCaptchaElement 设置验证码输入框与验证码图片的元素选择器（导出名为 rpa.bruteCaptchaElement）
// 参数:
//   - element: 验证码输入框的 CSS selector
//   - pic: 验证码图片的 CSS selector
//
// 返回值:
//   - 爆破配置可选项
//
// Example:
// ```
// opt = rpa.bruteCaptchaElement("#captcha", "#captcha-img")
// println(opt)
// ```
func WithCaptchaElement(element, pic string) ConfigOpt {
	return func(c *Config) {
		c.captchaElement = element
		c.captchaIMGElement = pic
	}
}

// WithButtonElement 设置登录提交按钮的元素选择器（导出名为 rpa.bruteButtonElement）
// 参数:
//   - element: 登录按钮的 CSS selector
//
// 返回值:
//   - 爆破配置可选项
//
// Example:
// ```
// opt = rpa.bruteButtonElement("#login")
// println(opt)
// ```
func WithButtonElement(element string) ConfigOpt {
	return func(c *Config) {
		c.clickButtonElement = element
	}
}

func WithCaptchaUrl(urlStr string) ConfigOpt {
	return func(c *Config) {
		c.captchaUrl = urlStr
	}
}

type ConfigMethod func(*BruteForce)

type ConfigMethods []ConfigMethod

// ClickMethod 在爆破前对指定元素执行点击操作（导出名为 rpa.click）
// 参数:
//   - selector: 目标元素的 CSS selector
//
// 返回值:
//   - 爆破配置可选项
//
// Example:
// ```
// opt = rpa.click("#agree")
// println(opt)
// ```
func ClickMethod(selector string) ConfigOpt {
	return func(c *Config) {
		c.doBefore = append(c.doBefore, clickMethod(selector))
	}
}

// SelectMethod 在爆破前对指定下拉框选择某个选项（导出名为 rpa.select）
// 参数:
//   - selector: 下拉框的 CSS selector
//   - item: 要选择的选项
//
// 返回值:
//   - 爆破配置可选项
//
// Example:
// ```
// opt = rpa.select("#role", "admin")
// println(opt)
// ```
func SelectMethod(selector, item string) ConfigOpt {
	return func(c *Config) {
		c.doBefore = append(c.doBefore, selectMethod(selector, item))
	}
}

// InputMethod 在爆破前向指定输入框填入文本（导出名为 rpa.input）
// 参数:
//   - selector: 输入框的 CSS selector
//   - inputStr: 要填入的文本
//
// 返回值:
//   - 爆破配置可选项
//
// Example:
// ```
// opt = rpa.input("#extra", "value")
// println(opt)
// ```
func InputMethod(selector, inputStr string) ConfigOpt {
	return func(c *Config) {
		c.doBefore = append(c.doBefore, inputMethod(selector, inputStr))
	}
}

func clickMethod(selector string) ConfigMethod {
	return func(bf *BruteForce) {
		element, _ := bf.GetElement(selector)
		if element == nil {
			return
		}
		bf.Click(element)
	}
}

func selectMethod(selector, item string) ConfigMethod {
	return func(bf *BruteForce) {
		element, _ := bf.GetElement(selector)
		if element == nil {
			return
		}
		element.MustSelect(item)
	}
}

func inputMethod(selector, inputStr string) ConfigMethod {
	return func(bf *BruteForce) {
		element, _ := bf.GetElement(selector)
		if element == nil {
			return
		}
		element.Type([]input.Key(inputStr)...)
	}
}
