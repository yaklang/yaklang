package core

var InvalidSuffix = []string{
	".js",
	".css",
	".jpg", ".jpeg", ".png",
	".mp3", ".mp4", ".ico", ".bmp",
	".flv", ".aac", ".ogg", ".avi",
	".svg", ".gif", ".woff", ".woff2",
	".doc", ".docx", ".pptx",
	".ppt", ".pdf",
}

var DefaultFormFill = map[string]string{
	"admin":    "admin",
	"password": "admin",
	"captcha":  "captcha",
}

var ElementAttribute = []string{
	"placeholder", "id", "name", "value", "alt",
}

var SensitiveWords = []string{
	"add", "set", "clean", "edit", "delete",
	"register", "install", "modify", "upload",
	"upgrade",
}

var SensitiveWordsCN = []string{
	"添加", "删除", "修改", "清除", "上传",
	"注册", "安装", "升级", "编辑", "设置",
}
