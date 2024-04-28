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
	ExecutePluginOrCache(param map[string]string) ([]byte, error)
	String() string
	GenWebShell() string
	SetCustomEncFunc(func(data, key []byte) ([]byte, error))
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
	case ypb.ShellType_YakShell.String():
		return NewYakShell(s)
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
	case ypb.ShellType_YakShell.String():
		return NewYakShell(info)
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

func NewGodzillaManager(url string, opts ...ShellConfig) (*Godzilla, error) {
	info := &ypb.WebShell{
		Url: url,
	}
	opts = append(opts, SetGodzillaTool())
	for _, opt := range opts {
		opt(info)
	}
	return NewGodzilla(info)
}

func NewYakShellManager(url string, opts ...ShellConfig) (*YakShell, error) {
	shell := getDefaultYpbShell(url)
	opts = append(opts, SetYakShellTool())
	for _, opt := range opts {
		opt(shell)
	}
	return NewYakShell(shell)
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
func SetYakShellTool() ShellConfig {
	return func(info *ypb.WebShell) {
		info.ShellType = ypb.ShellType_YakShell.String()
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
func SetSession() ShellConfig {
	return func(info *ypb.WebShell) {
		info.ShellOptions.IsSession = true
	}
}

func SetTimeout(timeout int64) ShellConfig {
	return func(info *ypb.WebShell) {
		info.ShellOptions.Timeout = timeout
	}
}
func SetBlockSize(size int64) ShellConfig {
	return func(info *ypb.WebShell) {
		info.ShellOptions.BlockSize = size
	}
}

func SetBase64Aes() ShellConfig {
	return func(info *ypb.WebShell) {
		info.EncMode = ypb.EncMode_AesBase64.String()
	}
}

func SetBase64Dec() ShellConfig {
	return func(info *ypb.WebShell) {
		info.ResDecMOde = ypb.EncMode_Base64.String()
	}
}

// SetBase64AesDec 当为Jsp的时候，需要满足Key为16或者32，todo：
func SetBase64AesDec() ShellConfig {
	return func(info *ypb.WebShell) {
		info.ResDecMOde = ypb.EncMode_AesBase64.String()
	}
}
func SetBase64xorDec() ShellConfig {
	return func(info *ypb.WebShell) {
		info.ResDecMOde = ypb.EncMode_XorBase64.String()
	}
}

func SetBase64() ShellConfig {
	return func(info *ypb.WebShell) {
		info.EncMode = ypb.EncMode_Base64.String()
	}
}

func SetBase64Xor() ShellConfig {
	return func(info *ypb.WebShell) {
		info.EncMode = ypb.EncMode_XorBase64.String()
	}
}

func SetRawAes() ShellConfig {
	return func(info *ypb.WebShell) {
		info.EncMode = ypb.EncMode_Raw.String()
	}
}

// SetHeaders TODO
func SetHeaders(headers map[string]string) ShellConfig {
	return func(info *ypb.WebShell) {
		info.Headers = headers
	}
}

func SetPosts(posts map[string]string) ShellConfig {
	return func(info *ypb.WebShell) {
		info.Posts = posts
	}
}

func SetProxy(p string) ShellConfig {
	return func(info *ypb.WebShell) {
		info.Proxy = p
	}
}

func getDefaultYpbShell(url string) *ypb.WebShell {
	return &ypb.WebShell{
		Url:     url,
		Charset: "utf-8",
		ShellOptions: &ypb.ShellOptions{
			RetryCount: 3,
			Timeout:    10,
			BlockSize:  1024 * 8,
			MaxSize:    0,
			UpdateTime: 20,
			IsSession:  false,
		},
	}
}
