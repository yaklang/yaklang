package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/wsm"
	"github.com/yaklang/yaklang/common/wsm/payloads/behinder"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type stubWebShellManager struct{}

func (s *stubWebShellManager) ClientRequestEncode(raw []byte) ([]byte, error) { return raw, nil }
func (s *stubWebShellManager) ServerResponseDecode(raw []byte) ([]byte, error) { return raw, nil }
func (s *stubWebShellManager) SetPacketScriptContent(string)                   {}
func (s *stubWebShellManager) EchoResultEncodeFormYak(raw []byte) ([]byte, error) {
	return raw, nil
}
func (s *stubWebShellManager) EchoResultDecodeFormYak(raw []byte) ([]byte, error) {
	return raw, nil
}
func (s *stubWebShellManager) SetPayloadScriptContent(string) {}
func (s *stubWebShellManager) Ping(...behinder.ExecParamsConfig) (bool, error) {
	return true, nil
}
func (s *stubWebShellManager) BasicInfo(...behinder.ExecParamsConfig) ([]byte, error) {
	return nil, nil
}
func (s *stubWebShellManager) CommandExec(string, ...behinder.ExecParamsConfig) ([]byte, error) {
	return nil, nil
}
func (s *stubWebShellManager) ExecutePluginOrCache(map[string]string) ([]byte, error) {
	return nil, nil
}
func (s *stubWebShellManager) String() string { return "stub" }
func (s *stubWebShellManager) GenWebShell() string {
	return ""
}
func (s *stubWebShellManager) SetCustomEncFunc(func(data, key []byte) ([]byte, error)) {}

func TestCreateWebShellInvalidatesManagerCache(t *testing.T) {
	server, err := newLiteYakServerForHotPatchTests(t)
	require.NoError(t, err)
	require.NoError(t, server.GetProjectDatabase().AutoMigrate(&schema.WebShell{}).Error)
	require.NoError(t, server.GetProfileDatabase().AutoMigrate(&schema.YakScript{}).Error)

	originCache := webShellManagerCache
	webShellManagerCache = make(map[int64]wsm.BaseShellManager)
	t.Cleanup(func() {
		webShellManagerCache = originCache
	})

	req := &ypb.WebShell{
		Url:         "http://example.com/a.jsp",
		SecretKey:   "rebeyond",
		Charset:     "utf-8",
		ShellType:   ypb.ShellType_Behinder.String(),
		ShellScript: ypb.ShellScript_JSP.String(),
		ShellOptions: &ypb.ShellOptions{
			Timeout:    10,
			RetryCount: 1,
			BlockSize:  1024,
		},
	}
	shell, err := server.CreateWebShell(context.Background(), req)
	require.NoError(t, err)

	webShellManagerCache[shell.GetId()] = &stubWebShellManager{}

	req.Remark = "updated"
	shell, err = server.CreateWebShell(context.Background(), req)
	require.NoError(t, err)

	_, ok := webShellManagerCache[shell.GetId()]
	require.False(t, ok)
}

func TestPingReturnsManagerLoadError(t *testing.T) {
	server, err := newLiteYakServerForHotPatchTests(t)
	require.NoError(t, err)
	require.NoError(t, server.GetProjectDatabase().AutoMigrate(&schema.WebShell{}).Error)
	require.NoError(t, server.GetProfileDatabase().AutoMigrate(&schema.YakScript{}).Error)

	originCache := webShellManagerCache
	webShellManagerCache = make(map[int64]wsm.BaseShellManager)
	t.Cleanup(func() {
		webShellManagerCache = originCache
	})

	shell, err := server.CreateWebShell(context.Background(), &ypb.WebShell{
		Url:              "http://example.com/b.jsp",
		SecretKey:        "rebeyond",
		Charset:          "utf-8",
		ShellType:        ypb.ShellType_Behinder.String(),
		ShellScript:      ypb.ShellScript_JSP.String(),
		PacketCodecName:  "not-exists-packet-codec",
		PayloadCodecName: "not-exists-payload-codec",
		ShellOptions: &ypb.ShellOptions{
			Timeout:    10,
			RetryCount: 1,
			BlockSize:  1024,
		},
	})
	require.NoError(t, err)

	_, err = server.Ping(context.Background(), &ypb.WebShellRequest{Id: shell.GetId()})
	require.Error(t, err)
}
