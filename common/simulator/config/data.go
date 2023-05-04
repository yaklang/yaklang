package config

var usernameKeyword = []string{
	"username", "admin",
	"用户名", "账户名", "账号",
	"telephone", "email",
	"手机", "电话", "邮箱",
}

var simpleUsernameKeyword = []string{
	"user", "admin", "tele", "email",
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
	"验证码", "校验码", "注册码", "verifica", "verify",
}

var simpleCaptchaKeyword = []string{
	"capt", "reg", "验证", "校验", "注册", "validate", "verif",
}

var loginKeyword = []string{
	"登录", "login", "submit",
}

var simpleLoginKeyword = []string{
	"登录", "login", "submit",
}

var KeywordDict = map[string][]string{
	"username": usernameKeyword,
	"password": passwordKeyword,
	"captcha":  captchaKeyword,
	"login":    loginKeyword,
}

var SimpleKeywordDict = map[string][]string{
	"username": simpleUsernameKeyword,
	"password": simplePasswordKeyword,
	"captcha":  simpleCaptchaKeyword,
	"login":    simpleLoginKeyword,
}

var KeywordAttribute = append(ElementAttribute, ElementProperty...)

var ElementAttribute = []string{
	"placeholder", "id", "name", "value", "alt",
}

var ElementProperty = []string{
	"innerHTML",
}
