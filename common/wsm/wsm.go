package wsm

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/wsm/payloads/behinder"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

type ShellConfig func(info *ypb.WebShell)
type codecFunc func(raw []byte) ([]byte, error)

type BaseShellManager interface {
	PacketCodecI
	PayloadCodecI
	Ping(opts ...behinder.ExecParamsConfig) (bool, error)
	BasicInfo(opts ...behinder.ExecParamsConfig) ([]byte, error)
	CommandExec(cmd string, opts ...behinder.ExecParamsConfig) ([]byte, error)
	String() string
	GenWebShell() string
}

type FileOperation interface {
	Execute(base BaseShellManager) ([]byte, error)
}

func NewWebShellManager(s *ypb.WebShell) (BaseShellManager, error) {
	switch s.GetShellType() {
	case ypb.ShellType_Behinder.String():
		return NewBehinder(s)
	case ypb.ShellType_Godzilla.String():
		return NewGodzilla(s)
	default:
		return nil, utils.Errorf("unsupported shell type %s", s.GetShellType())
	}
}

func NewWebShell(url string, opts ...ShellConfig) (BaseShellManager, error) {
	info := &ypb.WebShell{
		Url: url,
	}
	for _, opt := range opts {
		opt(info)
	}
	switch info.ShellType {
	case ypb.ShellType_Behinder.String():
		return NewBehinder(info)
	case ypb.ShellType_Godzilla.String():
		return NewGodzilla(info)
	default:
		return nil, utils.Errorf("unsupported shell type %s", info.GetShellType())
	}
}

func NewBehinderManager(url string, opts ...ShellConfig) (*Behinder, error) {
	info := &ypb.WebShell{
		Url: url,
	}
	opts = append(opts, SetBeinderTool())
	for _, opt := range opts {
		opt(info)
	}
	return NewBehinder(info)
}

func SaveShell(manager BaseShellManager) {

}

func SetShellType(tools string) ShellConfig {
	key, ok := ypb.ShellType_value[tools]
	if !ok {
		panic("only support [Behinder/Godzilla]")
	}
	return func(info *ypb.WebShell) {
		info.ShellType = ypb.ShellType(key).String()
	}
}

func SetBeinderTool() ShellConfig {
	return func(info *ypb.WebShell) {
		info.ShellType = ypb.ShellType_Behinder.String()
	}
}

func SetGodzillaTool() ShellConfig {
	return func(info *ypb.WebShell) {
		info.ShellType = ypb.ShellType_Godzilla.String()
	}
}

func SetShellScript(script string) ShellConfig {
	script = strings.ToUpper(script)
	return func(info *ypb.WebShell) {
		info.ShellScript = script
	}
}

func SetSecretKey(key string) ShellConfig {
	return func(info *ypb.WebShell) {
		info.SecretKey = key
	}
}

func SetPass(pass string) ShellConfig {
	return func(info *ypb.WebShell) {
		info.Pass = pass
	}
}

func SetBase64Aes() ShellConfig {
	return func(info *ypb.WebShell) {
		info.EncMode = ypb.EncMode_Base64.String()
	}
}

func SetRawAes() ShellConfig {
	return func(info *ypb.WebShell) {
		info.EncMode = ypb.EncMode_Raw.String()
	}
}

// SetHeaders TODO
func SetHeaders() ShellConfig {
	return func(info *ypb.WebShell) {

	}
}

// SetProxy TODO
func SetProxy(p string) ShellConfig {
	return func(info *ypb.WebShell) {
		info.Proxy = p
	}
}
