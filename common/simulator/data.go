// Package simulator
// @Author bcy2007  2023/8/17 16:19
package simulator

var ElementAttribute = []string{
	"placeholder", "id", "name", "value", "alt",
}

var ElementProperty = []string{
	"innerHTML",
}

var ElementKeyword = append(ElementAttribute, ElementProperty...)

var usernameKeyword = []string{
	"username", "admin", "account",
	"用户名", "账户名", "账号",
	"telephone", "email",
	"手机", "电话", "邮箱",
}

var simpleUsernameKeyword = []string{
	"user", "admin", "tele", "email", "account",
	"用户", "账户", "账号", "手机", "电话", "邮箱",
}

var passwordKeyword = []string{
	"password", "pwd", "密码",
}

var simplePasswordKeyword = []string{
	"pass", "pwd", "密码",
}

var captchaKeyword = []string{
	"captcha", "register", "check", "validate",
	"验证码", "校验码", "注册码", "verifica", "verify", "image",
}

var simpleCaptchaKeyword = []string{
	"capt", "reg", "验证", "校验", "注册", "validate", "verif", "image",
}

var loginKeyword = []string{
	"登录", "login", "submit",
}

var KeywordDict = map[string][]string{
	"Username": usernameKeyword,
	"Password": passwordKeyword,
	"Captcha":  captchaKeyword,
	"Login":    loginKeyword,
}

var SimpleKeywordDict = map[string][]string{
	"Username": simpleUsernameKeyword,
	"Password": simplePasswordKeyword,
	"Captcha":  simpleCaptchaKeyword,
	"Login":    loginKeyword,
}

const (
	OtherOcr = iota + 1
	OldDDDDOcr
	NewDDDDOcr
)
