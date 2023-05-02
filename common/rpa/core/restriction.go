package core

import (
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var defaultExcludedSuffix = []string{
	".js",
	".css",
	".jpg", ".jpeg", ".png",
	".mp3", ".mp4", ".ico", ".bmp",
	".flv", ".aac", ".ogg", ".avi",
	".svg", ".gif", ".woff", ".woff2",
	".doc", ".docx", ".pptx",
	".ppt", ".pdf",
}

var defaultExcludedFileName = []string{
	"logout", "del",
}

var defaultFillForm = map[string]string{
	"username":   "admin",
	"password":   "password",
	"ip":         "127.0.0.1",
	"mtxMessage": "aaaaa",
	// "captcha":    "aaaa",
}

var defaultUsername = []string{
	"user", "admin", "tele", "email",
	"用户", "账户", "账号", "手机", "电话", "邮箱",
}

var strictUsername = []string{
	"username", "admin", "telephone", "email",
	"用户", "账户", "账号", "手机", "电话", "邮箱",
}

var defaultPassword = []string{
	"pass",
	"密码",
}

var strictPassword = []string{
	"password", "密码",
}

var defaultCaptcha = []string{
	"captcha",
	"验证码",
	"code",
	"verify",
}

var strictCaptcha = []string{
	"captcha",
	"验证码",
	"verifycode",
}

var getDefault = map[string][]string{
	"username": defaultUsername,
	"password": defaultPassword,
	"captcha":  defaultCaptcha,
}

var getStrict = map[string][]string{
	"username": strictUsername,
	"password": strictPassword,
	"captcha":  strictCaptcha,
}

var DefaultKeyword = getDefault

var StrictKeyword = getStrict

var sensitiveWords = []string{
	"add", "set", "clean", "edit", "delete",
	"register", "install", "modify", "upload",
	"upgrade",
}

var sensitiveWordsCN = []string{
	"添加", "删除", "修改", "清除", "上传",
	"注册", "安装", "升级", "编辑", "设置",
}

func (m *Manager) checkFileSuffixValid(u string) bool {
	uins, err := url.Parse(u)
	if err != nil {
		return false
	}

	// 获得带 . 的前缀
	ext := strings.ToLower(filepath.Ext(uins.EscapedPath()))
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	for _, suf := range m.excludedSuffix {
		if ext == suf {
			return false
		}
	}
	_, fileName := filepath.Split(uins.EscapedPath())
	for _, file := range m.excludedFileName {
		if strings.HasPrefix(fileName, file) {
			return false
		}
	}

	if len(m.includedSuffix) > 0 {
		for _, pre := range m.includedSuffix {
			if pre == ext {
				return true
			}
		}
		return false
	}

	return true
}

func (m *Manager) checkHostIsValid(url string) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(url)), "javascript:") {
		return false
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(url)), "data:image") {
		return false
	}
	host, _, err := utils.ParseStringToHostPort(url)
	if err != nil {
		log.Errorf("parse url %s failed: %s", url, err)
		return false
	}

	if utils.IsIPv4(host) {
		ipIns := net.ParseIP(host)
		if ipIns == nil {
			log.Errorf("parse %v to ip failed: %s", host, err)
			return false
		}

		// 黑名单优先级更高
		for _, n := range m.blackNetwork {
			if n.Contains(ipIns) {
				return false
			}
		}

		// 白名单兜底
		if len(m.whiteNetwork) > 0 {
			for _, i := range m.whiteNetwork {
				if i.Contains(ipIns) {
					return true
				}
			}
			return false
		}

		return true
	}

	for _, g := range m.blackSubdomainGlob {
		if g.Match(host) {
			return false
		}
	}

	if len(m.whiteSubdomainGlob) > 0 {
		for _, g := range m.whiteSubdomainGlob {
			if g.Match(host) {
				return true
			}
		}
		return false
	}

	return true
}

func (m *Manager) RemoveParamValue(urlStr string) string {
	reg := regexp.MustCompile(`[\w_\-%]+\?([\w_\-%]+\=[\w_\-%]+&)*[\w_\-%]+\=[\w_\-%]+`)
	// fmt.Println(reg.FindAllString(urlStr, -1))
	allstring := reg.FindAllString(urlStr, -1)
	if len(allstring) <= 0 {
		return urlStr
	}
	sub_reg := regexp.MustCompile(`\=[\w\-_%]+`)
	result := sub_reg.ReplaceAllLiteralString(urlStr, "=")
	return result
}
