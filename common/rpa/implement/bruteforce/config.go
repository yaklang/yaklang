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

func WithUsername(username ...string) ConfigOpt {
	return func(c *Config) {
		c.username = append(c.username, username...)
	}
}

func WithPassword(password ...string) ConfigOpt {
	return func(c *Config) {
		c.password = append(c.password, password...)
	}
}

func WithUsernameElement(element string) ConfigOpt {
	return func(c *Config) {
		c.usernameElement = element
	}
}

func WithPasswordElement(element string) ConfigOpt {
	return func(c *Config) {
		c.passwordElement = element
	}
}

func WithCaptchaElement(element, pic string) ConfigOpt {
	return func(c *Config) {
		c.captchaElement = element
		c.captchaIMGElement = pic
	}
}

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

func ClickMethod(selector string) ConfigOpt {
	return func(c *Config) {
		c.doBefore = append(c.doBefore, clickMethod(selector))
	}
}

func SelectMethod(selector, item string) ConfigOpt {
	return func(c *Config) {
		c.doBefore = append(c.doBefore, selectMethod(selector, item))
	}
}

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
