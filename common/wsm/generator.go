package wsm

import (
	"fmt"
	"github.com/yaklang/yaklang/common/wsm/payloads/templates"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// WsGenerate 生成

type GenerateConfig func(generate *ypb.ShellGenerate)
type ConfuseFunc func(code string) (string, error)

func NewGenerate(opt ...GenerateConfig) *Generate {
	y := new(ypb.ShellGenerate)
	for _, config := range opt {
		config(y)
	}
	switch y.Script {
	case ypb.ShellScript_PHP:
		return newPhpGenerate(y)
	case ypb.ShellScript_JSP:
		return newJspGenerate(y)
	case ypb.ShellScript_ASPX:
		return newAspxGenerate(y)
	}
	return nil
}

/*
WithEncMode 加密模式，和ypb里面对应

	EncMode_Raw       EncMode = 0
	EncMode_Base64    EncMode = 1
	EncMode_AesRaw    EncMode = 2
	EncMode_AesBase64 EncMode = 3
	EncMode_XorRaw    EncMode = 4
	EncMode_XorBase64 EncMode = 5
*/

func WithXorBase64() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.EncMode = ypb.EncMode_XorBase64
	}
}
func WithXorRaw() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.EncMode = ypb.EncMode_XorRaw
	}
}
func WithPhpScript() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.Script = ypb.ShellScript_PHP
	}
}
func WithJspScript() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.Script = ypb.ShellScript_JSP
	}
}
func WithAspxScript() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.Script = ypb.ShellScript_ASPX
	}
}
func WithAesBase64() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.EncMode = ypb.EncMode_AesBase64
	}
}
func WithBase64() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.EncMode = ypb.EncMode_Base64
	}
}
func WithPass(pass string) GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.Pass = pass
	}
}
func WithConfuse() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.Confuse = true
	}
}
func WithSessionMode() GenerateConfig {
	return func(generate *ypb.ShellGenerate) {
		generate.IsSession = true
	}
}

type Generate struct {
	header string
	decode string

	memberLabelLeft   string
	memberLabelRight  string
	serviceLabelLeft  string
	serviceLabelRight string

	memberDecl  string
	serviceDecl string
	pass        string
	confuseFunc ConfuseFunc
}

func (j *Generate) Generate() (string, error) {
	var code string
	service, err := j.confuseFunc(fmt.Sprintf(j.serviceDecl, j.pass))
	if err != nil {
		return "", err
	}
	code = j.header + "\n" + j.memberLabelLeft + "\n" + j.memberDecl + "\n" + j.decode + fmt.Sprintf("\n%s\n", j.memberLabelRight) + fmt.Sprintf("%s\n", j.serviceLabelLeft) + service + fmt.Sprintf("\n%s", j.serviceLabelRight)
	return code, nil
}

func newJspGenerate(generate *ypb.ShellGenerate) *Generate {
	jspGenerate := getDefaultCustomJspGenerate()
	if generate.Confuse {
		jspGenerate.confuseFunc = confuseFuncWithUnicode()
	}
	jspGenerate.pass = generate.Pass
	if generate.IsSession {
		jspGenerate.serviceDecl = templates.GetTemplates().JspSessionServiceDecl
	}
	return jspGenerate
}
func newPhpGenerate(generate *ypb.ShellGenerate) *Generate {
	phpGenerate := getDefaultPhpCustomGenerate()
	if generate.Confuse {
		//todo
		//phpGenerate.confuseFunc = confuseFuncWithUnicode()
	}
	phpGenerate.pass = generate.Pass
	if generate.IsSession {
		phpGenerate.memberDecl = templates.GetTemplates().PhpSessionMemberDecl
		phpGenerate.serviceDecl = templates.GetTemplates().PhpSessionServiceDecl
	}
	return phpGenerate
}
func newAspxGenerate(generate *ypb.ShellGenerate) *Generate {
	aspxWebShellGenerate := getDefaultAspxWebShellGenerate()
	if generate.Confuse {
	}
	aspxWebShellGenerate.pass = generate.Pass
	if generate.IsSession {
		aspxWebShellGenerate.serviceDecl = templates.GetTemplates().AspxSessionServiceDecl
	}
	return aspxWebShellGenerate
}

func confuseFuncWithUnicode() ConfuseFunc {
	return func(code string) (string, error) {
		return codec.JsonUnicodeEncode(code), nil
	}
}

func getDefaultCustomJspGenerate() *Generate {
	t := templates.GetTemplates()
	return &Generate{
		memberLabelLeft:   "<%!",
		memberLabelRight:  "%>",
		serviceLabelLeft:  "<%",
		serviceLabelRight: "%>",
		header:            t.JspDefaultHeader,
		decode:            t.JspDefaultDecode,
		memberDecl:        t.JspDefaultMemberDecl,
		serviceDecl:       t.JspDefaultServiceDecl,
		confuseFunc: func(code string) (string, error) {
			return code, nil
		},
	}
}
func getDefaultPhpCustomGenerate() *Generate {
	t := templates.GetTemplates()
	return &Generate{
		header:            t.PhpDefaultHeader,
		decode:            t.PhpDefaultDecode,
		memberLabelLeft:   "",
		memberLabelRight:  "",
		serviceLabelLeft:  "",
		serviceLabelRight: "",
		memberDecl:        t.PhpDefaultMemberDecl,
		serviceDecl:       t.PhpDefaultServiceDecl,
		confuseFunc: func(code string) (string, error) {
			return code, nil
		},
	}
}

func getDefaultAspxWebShellGenerate() *Generate {
	t := templates.GetTemplates()
	return &Generate{
		header:            t.AspxDefaultHeader,
		serviceLabelLeft:  "<%",
		serviceLabelRight: "%>",
		serviceDecl:       t.AspxDefaultServiceDecl,
		confuseFunc: func(code string) (string, error) {
			return code, nil
		},
	}
}
