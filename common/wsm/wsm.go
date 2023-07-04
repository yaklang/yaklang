package wsm

import (
	"fmt"
	"github.com/yaklang/yaklang/common/wsm/payloads/behinder"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

type ShellConfig func(info *ypb.WebShell)
type EncoderFunc func(raw []byte) ([]byte, error)

type BaseShellManager interface {
	//Encoder(EncoderFunc) ([]byte, error)
	Encoder(func(raw []byte) ([]byte, error))
	//Encoder([]byte) ([]byte, error)
	Ping(opts ...behinder.ParamsConfig) (bool, error)
	BasicInfo(opts ...behinder.ParamsConfig) ([]byte, error)
	CommandExec(cmd string, opts ...behinder.ParamsConfig) ([]byte, error)
}

type Wsm struct {
}

func (w Wsm) Ping() (bool, error) {
	//TODO implement me
	panic("implement me")
}

func NewWsm(s *ypb.WebShell) (BaseShellManager, error) {
	var m BaseShellManager
	var err error
	switch s.GetShellType() {
	case ypb.ShellType_Behinder.String():
		m = NewBehinder(s)
	case ypb.ShellType_Godzilla.String():
		m = NewGodzilla(s)
	default:
		panic(fmt.Sprintf("unsupported option %s", s.GetShellType()))
	}
	return m, err
}

func NewWebShell(url string, opts ...ShellConfig) BaseShellManager {
	info := &ypb.WebShell{
		Url: url,
	}
	var bm BaseShellManager
	for _, opt := range opts {
		opt(info)
	}
	switch info.ShellType {
	case ypb.ShellType_Behinder.String():
		bm = NewBehinder(info)
	case ypb.ShellType_Godzilla.String():
		bm = NewGodzilla(info)
	default:
		panic(fmt.Sprintf("unsupported option %s", info.GetShellType()))
	}
	return bm
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
